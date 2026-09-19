package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const Repo = "piyush-gambhir/clarity-cli"

var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type Release struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

func get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "clarity-cli")
	h := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != "https" {
			return fmt.Errorf("unsafe or excessive release redirect")
		}
		return nil
	}}
	res, err := h.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return nil, fmt.Errorf("no published release found for %s; install from source with make install", Repo)
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("release download returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("release response exceeds size limit")
	}
	return b, nil
}

func Latest(ctx context.Context) (*Release, error) {
	b, err := get(ctx, "https://api.github.com/repos/"+Repo+"/releases/latest", 1<<20)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if !versionPattern.MatchString(r.Tag) {
		return nil, fmt.Errorf("invalid release version")
	}
	r.URL = "https://github.com/" + Repo + "/releases/tag/" + r.Tag
	return &r, nil
}

func VerifyChecksum(archive []byte, checksums []byte, name string) error {
	wanted := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			if wanted != "" {
				return fmt.Errorf("duplicate archive checksum")
			}
			wanted = fields[0]
		}
	}
	hash := sha256.Sum256(archive)
	if wanted == "" || !strings.EqualFold(wanted, hex.EncodeToString(hash[:])) {
		return fmt.Errorf("SHA-256 checksum verification failed for %s", name)
	}
	return nil
}

func ExtractBinary(archive []byte) ([]byte, error) {
	z, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	r := tar.NewReader(z)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Name != "clarity" && h.Name != "./clarity" {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > 64<<20 {
			return nil, fmt.Errorf("invalid clarity binary in release")
		}
		return io.ReadAll(io.LimitReader(r, 64<<20))
	}
	return nil, fmt.Errorf("release does not contain clarity binary")
}

func Install(ctx context.Context, r *Release) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("on Windows, download and replace clarity.exe from %s", r.URL)
	}
	if !versionPattern.MatchString(r.Tag) {
		return fmt.Errorf("invalid release version")
	}
	asset := "clarity-cli_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	base := "https://github.com/" + Repo + "/releases/download/" + r.Tag + "/"
	checksums, err := get(ctx, base+"checksums.txt", 1<<20)
	if err != nil {
		return err
	}
	archive, err := get(ctx, base+asset, 64<<20)
	if err != nil {
		return err
	}
	if err := VerifyChecksum(archive, checksums, asset); err != nil {
		return err
	}
	b, err := ExtractBinary(archive)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".clarity-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0755); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), exe)
}
