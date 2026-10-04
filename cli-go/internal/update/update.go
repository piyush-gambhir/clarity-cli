// Package update finds, verifies, and installs clarity releases from GitHub, and
// caches the latest release for the update notice.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
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
	// SourceUpdate is how a build in a Go bin directory updates. The main package
	// lives in cli-go/, so `go install .../cli-go@latest` would install a binary
	// named cli-go from an untagged commit; the documented source path is make install.
	SourceUpdate  = "git pull && make install (in your clarity-cli checkout)"
	installScript = "curl -fsSL https://raw.githubusercontent.com/" + Repo + "/main/install.sh | sh"
	maxArchive    = 64 << 20
	maxBinary     = 64 << 20
)

var (
	tagPattern     = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	releasePattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?$`)
	errNotFound    = errors.New("HTTP 404")
	// maxExtracted caps the bytes decompressed from a tar.gz, skipped entries
	// included, so a small archive cannot expand without bound. Tests lower it.
	maxExtracted int64 = 256 << 20
	// rename is os.Rename; tests replace it to simulate a failed swap.
	rename = os.Rename
)

// Source holds the release endpoints. Tests point it at local servers.
type Source struct {
	LatestURL   string // JSON for the latest published release
	DownloadURL string // prefix for <tag>/<asset> downloads
}

// GitHub is the published release source.
var GitHub = Source{
	LatestURL:   "https://api.github.com/repos/" + Repo + "/releases/latest",
	DownloadURL: "https://github.com/" + Repo + "/releases/download/",
}

type Release struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

// ReleaseURL is the release notes page for a tag.
func ReleaseURL(tag string) string { return "https://github.com/" + Repo + "/releases/tag/" + tag }

// IsRelease reports whether v is a release version rather than a dev or unknown build.
func IsRelease(v string) bool { return releasePattern.MatchString(v) }

// Display prefixes release versions with "v"; other builds (such as dev) are shown as is.
func Display(v string) string {
	if !IsRelease(v) {
		return v
	}
	return "v" + strings.TrimPrefix(v, "v")
}

// Newer reports whether latest is a higher release version than current.
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
	// A release is newer than its own prereleases.
	return l[4] == "" && c[4] != ""
}

// Notice is the update notice printed on stderr after a command's output.
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

// Latest looks up the latest published release.
func (s Source) Latest(ctx context.Context) (*Release, error) {
	b, err := get(ctx, s.LatestURL, 1<<20)
	if errors.Is(err, errNotFound) {
		return nil, fmt.Errorf("no published release found for %s; install from source with make install", Repo)
	}
	if err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	var r Release
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	if !tagPattern.MatchString(r.Tag) {
		return nil, fmt.Errorf("invalid release version")
	}
	r.URL = ReleaseURL(r.Tag)
	return &r, nil
}

// AssetName is the GoReleaser archive for a platform (see cli-go/.goreleaser.yaml).
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

// ExtractBinary returns the clarity executable from a release archive (zip on
// Windows, tar.gz elsewhere). It refuses archives with unsafe entry names and a
// binary entry that is missing, duplicated, oversized, or not a regular file.
// Nothing is written to disk; the caller chooses the output path.
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

// capReader fails once more than left bytes have been read.
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

// isEntry reports whether an archive entry is the wanted binary. A release
// archive never contains absolute, parent-relative, or drive paths, so any such
// entry rejects the whole archive.
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

// Target is the platform to download for and the executable to replace.
type Target struct {
	GOOS, GOARCH string
	Exe          string // resolved path of the installed executable
}

// Install downloads the release archive for t, verifies it against the
// release's checksums.txt, and replaces t.Exe. Any failure leaves t.Exe as it was.
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

// Replace swaps exe for binary without leaving exe missing or partial. The new
// file is written next to exe first. On Unix it is renamed over exe. Windows
// cannot overwrite a running executable but can rename it, so exe moves aside to
// exe+".old" (removed on a later start by RemoveOld) and the new file takes its
// place. goos is a parameter so both paths are testable on any OS.
func Replace(goos, exe string, binary []byte) error {
	dir := filepath.Dir(exe)
	f, err := os.CreateTemp(dir, ".clarity-update-*")
	if err != nil {
		return notWritable(goos, dir, err)
	}
	tmp := f.Name()
	defer os.Remove(tmp) // a no-op once the file is renamed into place
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
	_ = os.Remove(old) // a leftover from an earlier update
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

// RemoveOld deletes the executable a Windows update moved aside. It is best
// effort: the file stays while a process started from it is still running.
func RemoveOld(exe string) { _ = os.Remove(exe + ".old") }

// InGoBin reports whether exe is in a directory where Go tooling installs
// binaries ($GOBIN, each $GOPATH/bin, or ~/go/bin). Such builds come from
// source, so they are not replaced by a release binary.
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
