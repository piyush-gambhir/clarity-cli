package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	Repo = "piyush-gambhir/clarity-cli"
	SourceUpdate  = "git pull && make install (in your clarity-cli checkout)"
	installScript = "curl -fsSL https://raw.githubusercontent.com/" + Repo + "/main/install.sh | sh"
	maxArchive    = 64 << 20
	maxBinary     = 64 << 20
	latestTimeout = 15 * time.Second
)

var (
	tagPattern     = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	releasePattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?$`)
	errNotFound    = errors.New("HTTP 404")
	maxExtracted int64 = 256 << 20
	rename = os.Rename
)

type Source struct {
	LatestURL   string
	DownloadURL string
}

var GitHub = Source{
	LatestURL:   "https://github.com/" + Repo + "/releases/latest",
	DownloadURL: "https://github.com/" + Repo + "/releases/download/",
}

type Release struct {
	Tag string
	URL string
}

func ReleaseURL(tag string) string { return "https://github.com/" + Repo + "/releases/tag/" + tag }

func IsRelease(v string) bool { return releasePattern.MatchString(v) }

func Display(v string) string {
	if !IsRelease(v) {
		return v
	}
	return "v" + strings.TrimPrefix(v, "v")
}

func Newer(latest, current string) bool {
	l, c := releasePattern.FindStringSubmatch(latest), releasePattern.FindStringSubmatch(current)
	if l == nil || c == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(l[i])
		y, _ := strconv.Atoi(c[i])
		if x != y {
			return x > y
		}
	}
	return l[4] == "" && c[4] != ""
}

func Notice(current, latest string, goBin bool) string {
	how := "clarity update"
	if goBin {
		how = SourceUpdate
	}
	return fmt.Sprintf("A new version of clarity is available: %s -> %s\nUpdate with: %s\nRelease notes: %s\n",
		Display(current), Display(latest), how, ReleaseURL(Display(latest)))
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
	if res.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("response exceeds size limit")
	}
	return b, nil
}

func (s Source) Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", s.LatestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	req.Header.Set("User-Agent", "clarity-cli")
	h := &http.Client{Timeout: latestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no published release found for %s; install from source with make install", Repo)
	}
	if res.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("check latest release: expected a redirect to the release, got HTTP %d", res.StatusCode)
	}
	tag, err := redirectTag(req.URL, res.Header.Get("Location"))
	if err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	return &Release{Tag: tag, URL: ReleaseURL(tag)}, nil
}

func redirectTag(latest *url.URL, location string) (string, error) {
	if location == "" {
		return "", errors.New("the response did not name a release")
	}
	u, err := latest.Parse(location)
	if err != nil {
		return "", errors.New("unexpected release redirect")
	}
	tag, ok := strings.CutPrefix(u.Path, strings.TrimSuffix(latest.Path, "latest")+"tag/")
	if !ok || u.Scheme != latest.Scheme || u.Host != latest.Host {
		return "", errors.New("unexpected release redirect")
	}
	if !tagPattern.MatchString(tag) {
		return "", errors.New("the latest release tag is not a version")
	}
	return tag, nil
}

func AssetName(goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "clarity-cli_" + goos + "_" + goarch + ext
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

func ExtractBinary(archive []byte, goos string) ([]byte, error) {
	if goos == "windows" {
		return extractZip(archive, "clarity.exe")
	}
	return extractTarGz(archive, "clarity")
}

func extractTarGz(archive []byte, want string) ([]byte, error) {
	z, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	r := tar.NewReader(&capReader{r: z, left: maxExtracted})
	var bin []byte
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if match, err := isEntry(h.Name, want); err != nil || !match {
			if err != nil {
				return nil, err
			}
			continue
		}
		if bin != nil {
			return nil, fmt.Errorf("release archive has more than one %s", want)
		}
		if h.Typeflag != tar.TypeReg || h.Size <= 0 || h.Size > maxBinary {
			return nil, fmt.Errorf("invalid %s binary in release", want)
		}
		if bin, err = readBinary(r, want); err != nil {
			return nil, err
		}
	}
	if bin == nil {
		return nil, fmt.Errorf("release does not contain %s binary", want)
	}
	return bin, nil
}

func extractZip(archive []byte, want string) ([]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	var bin []byte
	for _, f := range z.File {
		if match, err := isEntry(f.Name, want); err != nil || !match {
			if err != nil {
				return nil, err
			}
			continue
		}
		if bin != nil {
			return nil, fmt.Errorf("release archive has more than one %s", want)
		}
		if !f.Mode().IsRegular() || f.UncompressedSize64 == 0 || f.UncompressedSize64 > maxBinary {
			return nil, fmt.Errorf("invalid %s binary in release", want)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		bin, err = readBinary(rc, want)
		rc.Close()
		if err != nil {
			return nil, err
		}
	}
	if bin == nil {
		return nil, fmt.Errorf("release does not contain %s binary", want)
	}
	return bin, nil
}

type capReader struct {
	r    io.Reader
	left int64
}

func (c *capReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if c.left -= int64(n); c.left < 0 {
		return n, fmt.Errorf("release archive expands beyond the size limit")
	}
	return n, err
}

func isEntry(name, want string) (bool, error) {
	if name == "" || path.IsAbs(name) || strings.ContainsAny(name, `\:`) || slices.Contains(strings.Split(name, "/"), "..") {
		return false, fmt.Errorf("release archive has an unsafe entry %q", name)
	}
	return path.Clean(name) == want, nil
}

func readBinary(r io.Reader, name string) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxBinary+1))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b) > maxBinary {
		return nil, fmt.Errorf("invalid %s binary in release", name)
	}
	return b, nil
}

type Target struct {
	GOOS, GOARCH string
	Exe          string
}

func (s Source) Install(ctx context.Context, tag string, t Target) error {
	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("invalid release version")
	}
	asset := AssetName(t.GOOS, t.GOARCH)
	base := s.DownloadURL + tag + "/"
	checksums, err := get(ctx, base+"checksums.txt", 1<<20)
	if err != nil {
		return fmt.Errorf("download checksums.txt for %s: %w", tag, err)
	}
	archive, err := get(ctx, base+asset, maxArchive)
	if errors.Is(err, errNotFound) {
		return fmt.Errorf("release %s has no %s asset for this platform", tag, asset)
	}
	if err != nil {
		return fmt.Errorf("download %s: %w", asset, err)
	}
	if err := VerifyChecksum(archive, checksums, asset); err != nil {
		return err
	}
	bin, err := ExtractBinary(archive, t.GOOS)
	if err != nil {
		return err
	}
	return Replace(t.GOOS, t.Exe, bin)
}

func Replace(goos, exe string, binary []byte) error {
	dir := filepath.Dir(exe)
	f, err := os.CreateTemp(dir, ".clarity-update-*")
	if err != nil {
		return notWritable(goos, dir, err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, err = f.Write(binary)
	if err == nil {
		err = f.Chmod(0o755)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write new executable: %w", err)
	}
	if goos != "windows" {
		if err := rename(tmp, exe); err != nil {
			return fmt.Errorf("replace %s: %w", exe, err)
		}
		return nil
	}
	old := exe + ".old"
	_ = os.Remove(old)
	if err := rename(exe, old); err != nil {
		return fmt.Errorf("move %s aside: %w", exe, err)
	}
	if err := rename(tmp, exe); err != nil {
		if rerr := rename(old, exe); rerr != nil {
			return fmt.Errorf("install new executable: %v; restoring the previous one also failed (%v), it is at %s", err, rerr, old)
		}
		return fmt.Errorf("install new executable: %w", err)
	}
	return nil
}

func notWritable(goos, dir string, err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	hint := "re-run with sudo, or reinstall with the install script into a writable directory: " + installScript
	if goos == "windows" {
		hint = "re-run from an Administrator terminal, or extract the release ZIP into a writable directory"
	}
	return fmt.Errorf("cannot write to %s (%v); %s. The installed clarity was not changed", dir, err, hint)
}

func RemoveOld(exe string) { _ = os.Remove(exe + ".old") }

func InGoBin(exe string) bool {
	exeDir, err := os.Stat(filepath.Dir(exe))
	if err != nil {
		return false
	}
	dirs := []string{os.Getenv("GOBIN")}
	for _, p := range filepath.SplitList(os.Getenv("GOPATH")) {
		if p != "" {
			dirs = append(dirs, filepath.Join(p, "bin"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "go", "bin"))
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if fi, err := os.Stat(d); err == nil && os.SameFile(fi, exeDir) {
			return true
		}
	}
	return false
}
