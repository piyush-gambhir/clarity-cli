package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type entry struct {
	name     string
	typeflag byte // tar type; for zip, TypeSymlink marks a symlink
	body     string
}

func tarGz(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	tw := tar.NewWriter(z)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0o755, Typeflag: e.typeflag, Size: int64(len(e.body))}
		if e.typeflag != tar.TypeReg {
			h.Size, h.Linkname = 0, "/etc/passwd"
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil && e.typeflag == tar.TypeReg {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func zipArchive(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(0o755)
		if e.typeflag == tar.TypeSymlink {
			h.SetMode(fs.ModeSymlink | 0o777)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func checksum(name string, data []byte) []byte {
	return []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name))
}

func TestChecksumAndExtraction(t *testing.T) {
	archive := tarGz(t, entry{"LICENSE", tar.TypeReg, "MIT"}, entry{"clarity", tar.TypeReg, "binary"})
	asset := "clarity-cli_darwin_arm64.tar.gz"
	if err := VerifyChecksum(archive, checksum(asset, archive), asset); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChecksum([]byte("tampered"), checksum(asset, archive), asset); err == nil {
		t.Fatal("accepted tampered archive")
	}
	if err := VerifyChecksum(archive, checksum("clarity-cli_linux_amd64.tar.gz", archive), asset); err == nil {
		t.Fatal("accepted archive without a checksum entry")
	}
	if got, err := ExtractBinary(archive, "darwin"); err != nil || string(got) != "binary" {
		t.Fatalf("extract %q %v", got, err)
	}
	z := zipArchive(t, entry{"README.md", tar.TypeReg, "readme"}, entry{"clarity.exe", tar.TypeReg, "exe"})
	if got, err := ExtractBinary(z, "windows"); err != nil || string(got) != "exe" {
		t.Fatalf("extract zip %q %v", got, err)
	}
}

func TestExtractRefusesUnsafeArchives(t *testing.T) {
	good := entry{"clarity", tar.TypeReg, "binary"}
	for name, archive := range map[string][]byte{
		"traversal":      tarGz(t, entry{"../clarity", tar.TypeReg, "evil"}, good),
		"absolute":       tarGz(t, entry{"/tmp/clarity", tar.TypeReg, "evil"}, good),
		"backslash":      tarGz(t, entry{`..\clarity`, tar.TypeReg, "evil"}, good),
		"symlink":        tarGz(t, entry{"clarity", tar.TypeSymlink, ""}),
		"hardlink":       tarGz(t, entry{"clarity", tar.TypeLink, ""}),
		"directory":      tarGz(t, entry{"clarity/", tar.TypeDir, ""}),
		"duplicate":      tarGz(t, good, good),
		"missing":        tarGz(t, entry{"README.md", tar.TypeReg, "readme"}),
		"zip as tar.gz":  zipArchive(t, entry{"clarity", tar.TypeReg, "binary"}),
		"empty binary":   tarGz(t, entry{"clarity", tar.TypeReg, ""}),
		"nested binary":  tarGz(t, entry{"bin/clarity", tar.TypeReg, "binary"}),
		"dot-dot inside": tarGz(t, entry{"a/../../clarity", tar.TypeReg, "evil"}, good),
	} {
		if got, err := ExtractBinary(archive, "linux"); err == nil {
			t.Errorf("%s: extracted %q", name, got)
		}
	}
	goodExe := entry{"clarity.exe", tar.TypeReg, "exe"}
	for name, archive := range map[string][]byte{
		"zip traversal": zipArchive(t, entry{"../clarity.exe", tar.TypeReg, "evil"}, goodExe),
		"zip drive":     zipArchive(t, entry{"C:/clarity.exe", tar.TypeReg, "evil"}, goodExe),
		"zip symlink":   zipArchive(t, entry{"clarity.exe", tar.TypeSymlink, "C:/Windows/notepad.exe"}),
		"zip duplicate": zipArchive(t, goodExe, goodExe),
		"zip missing":   zipArchive(t, entry{"clarity", tar.TypeReg, "unix binary"}),
	} {
		if got, err := ExtractBinary(archive, "windows"); err == nil {
			t.Errorf("%s: extracted %q", name, got)
		}
	}
}

func TestExtractBoundsSkippedEntries(t *testing.T) {
	defer func(n int64) { maxExtracted = n }(maxExtracted)
	maxExtracted = 1 << 20
	bomb := tarGz(t, entry{"clarity", tar.TypeReg, "binary"}, entry{"README.md", tar.TypeReg, strings.Repeat("0", 2<<20)})
	if _, err := ExtractBinary(bomb, "linux"); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("err = %v", err)
	}
}

func writeExe(t *testing.T, content string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "clarity")
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
	}
}

// assertOnly fails if dir holds anything besides the named files (such as a leftover temp file).
func assertOnly(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(names, ",") {
		t.Fatalf("%s has %v, want %v", dir, got, names)
	}
}

func TestReplaceUnix(t *testing.T) {
	exe := writeExe(t, "old")
	if err := Replace("linux", exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	assertFile(t, exe, "new")
	assertOnly(t, filepath.Dir(exe), "clarity")
	if fi, _ := os.Stat(exe); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v, want 0755", fi.Mode().Perm())
	}
}

func TestReplaceWindowsRenamesRunningExeAside(t *testing.T) {
	exe := writeExe(t, "old") + ".exe"
	if err := os.Rename(strings.TrimSuffix(exe, ".exe"), exe); err != nil {
		t.Fatal(err)
	}
	if err := Replace("windows", exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	assertFile(t, exe, "new")
	assertFile(t, exe+".old", "old")
	// The next start removes the leftover.
	RemoveOld(exe)
	assertOnly(t, filepath.Dir(exe), "clarity.exe")

	// A failed move into place restores the previous executable.
	defer func() { rename = os.Rename }()
	rename = func(from, to string) error {
		if strings.Contains(filepath.Base(from), ".clarity-update-") {
			return errors.New("simulated failure")
		}
		return os.Rename(from, to)
	}
	if err := Replace("windows", exe, []byte("newer")); err == nil {
		t.Fatal("expected failure")
	}
	assertFile(t, exe, "new")
	assertOnly(t, filepath.Dir(exe), "clarity.exe")
}

func TestReplaceUnwritableDirectoryKeepsOldBinary(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not restrict this user")
	}
	exe := writeExe(t, "old")
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	err := Replace("linux", exe, []byte("new"))
	if err == nil || !strings.Contains(err.Error(), "sudo") || !strings.Contains(err.Error(), "install.sh") || !strings.Contains(err.Error(), "not changed") {
		t.Fatalf("err = %v", err)
	}
	assertFile(t, exe, "old")
}

func releaseServer(t *testing.T, assets map[string][]byte) Source {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, srv.URL+"/releases/tag/v0.1.3", http.StatusFound)
			return
		}
		b, ok := assets[strings.TrimPrefix(r.URL.Path, "/download/v0.1.3/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return Source{LatestURL: srv.URL + "/releases/latest", DownloadURL: srv.URL + "/download/"}
}

func TestInstall(t *testing.T) {
	linux := tarGz(t, entry{"clarity", tar.TypeReg, "linux build"})
	windows := zipArchive(t, entry{"clarity.exe", tar.TypeReg, "windows build"})
	sums := append(checksum("clarity-cli_linux_amd64.tar.gz", linux), checksum("clarity-cli_windows_arm64.zip", windows)...)
	src := releaseServer(t, map[string][]byte{"checksums.txt": sums, "clarity-cli_linux_amd64.tar.gz": linux, "clarity-cli_windows_arm64.zip": windows})
	ctx := context.Background()

	r, err := src.Latest(ctx)
	if err != nil || r.Tag != "v0.1.3" || r.URL != "https://github.com/piyush-gambhir/clarity-cli/releases/tag/v0.1.3" {
		t.Fatalf("latest %+v %v", r, err)
	}
	exe := writeExe(t, "old")
	if err := src.Install(ctx, "v0.1.3", Target{GOOS: "linux", GOARCH: "amd64", Exe: exe}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, exe, "linux build")
	winExe := writeExe(t, "old")
	if err := src.Install(ctx, "v0.1.3", Target{GOOS: "windows", GOARCH: "arm64", Exe: winExe}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, winExe, "windows build")
	assertFile(t, winExe+".old", "old")

	// Missing assets and checksum mismatches leave the installed binary alone.
	keep := writeExe(t, "old")
	if err := src.Install(ctx, "v0.1.3", Target{GOOS: "darwin", GOARCH: "arm64", Exe: keep}); err == nil || !strings.Contains(err.Error(), "no clarity-cli_darwin_arm64.tar.gz asset") {
		t.Fatalf("missing asset: %v", err)
	}
	bad := releaseServer(t, map[string][]byte{"checksums.txt": checksum("clarity-cli_linux_amd64.tar.gz", []byte("other")), "clarity-cli_linux_amd64.tar.gz": linux})
	if err := bad.Install(ctx, "v0.1.3", Target{GOOS: "linux", GOARCH: "amd64", Exe: keep}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("mismatch: %v", err)
	}
	assertFile(t, keep, "old")
	assertOnly(t, filepath.Dir(keep), "clarity")
}

func TestLatestReadsRedirectWithoutFollowing(t *testing.T) {
	var followed atomic.Bool
	for _, tc := range []struct {
		name, location string // SRV in location is the test server's origin
		status         int
		want           string // tag, or a substring of the error
	}{
		{"good tag", "SRV/o/r/releases/tag/v0.1.3", http.StatusFound, "v0.1.3"},
		{"relative location", "/o/r/releases/tag/v1.2.0-rc.1", http.StatusFound, "v1.2.0-rc.1"},
		{"missing location", "", http.StatusFound, "did not name a release"},
		{"foreign host", "https://example.com/o/r/releases/tag/v0.1.3", http.StatusFound, "unexpected release redirect"},
		{"other repo", "SRV/x/r/releases/tag/v0.1.3", http.StatusFound, "unexpected release redirect"},
		{"non-semver tag", "SRV/o/r/releases/tag/nightly", http.StatusFound, "not a version"},
		{"escape in tag", "SRV/o/r/releases/tag/v1.0.0%1B%5B31m", http.StatusFound, "not a version"},
		{"nested path", "SRV/o/r/releases/tag/v0.1.3/extra", http.StatusFound, "not a version"},
		{"no redirect", "", http.StatusOK, "got HTTP 200"},
		{"rate limited", "", http.StatusForbidden, "got HTTP 403"},
		{"no release", "", http.StatusNotFound, "no published release"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/o/r/releases/latest" {
				followed.Store(true)
				return
			}
			if tc.location != "" {
				w.Header().Set("Location", strings.ReplaceAll(tc.location, "SRV", "http://"+r.Host))
			}
			w.WriteHeader(tc.status)
		}))
		r, err := Source{LatestURL: srv.URL + "/o/r/releases/latest"}.Latest(context.Background())
		srv.Close()
		switch {
		case strings.HasPrefix(tc.want, "v"):
			if err != nil || r.Tag != tc.want || r.URL != ReleaseURL(tc.want) {
				t.Errorf("%s: %+v %v", tc.name, r, err)
			}
		case err == nil || !strings.Contains(err.Error(), tc.want):
			t.Errorf("%s: %+v %v, want error containing %q", tc.name, r, err, tc.want)
		}
	}
	if followed.Load() {
		t.Fatal("followed the release redirect")
	}
}

func TestGitHubSourceAvoidsAPI(t *testing.T) {
	if GitHub.LatestURL != "https://github.com/"+Repo+"/releases/latest" || strings.Contains(GitHub.LatestURL+GitHub.DownloadURL, "api.github.com") {
		t.Fatalf("GitHub = %+v", GitHub)
	}
}

func TestNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.3", "0.1.2", true}, {"v0.2.0", "v0.1.9", true}, {"v0.1.10", "0.1.9", true}, {"v1.0.0", "1.0.0-rc.1", true},
		{"v0.1.2", "0.1.2", false}, {"v0.1.2", "0.1.3", false}, {"v0.1.3", "dev", false}, {"", "0.1.2", false},
	} {
		if got := Newer(tc.latest, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v", tc.latest, tc.current, got)
		}
	}
}

func TestNoticeOncePerVersionPerDay(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	c := Cache{CheckedAt: now.Add(-time.Hour), LatestVersion: "v0.1.3"}
	if !c.Fresh(now) || c.Fresh(now.Add(CheckInterval)) || c.Fresh(now.Add(-2*time.Hour)) {
		t.Fatal("fresh window is wrong")
	}
	if !c.ShouldNotify("0.1.2", now) || c.ShouldNotify("0.1.3", now) || c.ShouldNotify("dev", now) {
		t.Fatal("notify depends on a newer release")
	}
	c.NotifiedVersion, c.NotifiedAt = "v0.1.3", now
	if c.ShouldNotify("0.1.2", now.Add(23*time.Hour)) {
		t.Fatal("notified twice within a day")
	}
	if !c.ShouldNotify("0.1.2", now.Add(25*time.Hour)) {
		t.Fatal("no notice after a day")
	}
	c.LatestVersion = "v0.1.4"
	if !c.ShouldNotify("0.1.2", now.Add(time.Minute)) {
		t.Fatal("no notice for a newer release")
	}

	path := filepath.Join(t.TempDir(), "sub", CacheFile)
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(path); got != c {
		t.Fatalf("round trip %+v != %+v", got, c)
	}
	if err := os.WriteFile(path, []byte(`{"latest_version":"v9.9.9\u001b]0;x\u0007"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadCache(path); got.LatestVersion != "" {
		t.Fatalf("kept unsafe version %q", got.LatestVersion)
	}
}

func TestNoticeText(t *testing.T) {
	want := "A new version of clarity is available: v0.1.2 -> v0.1.3\nUpdate with: clarity update\nRelease notes: https://github.com/piyush-gambhir/clarity-cli/releases/tag/v0.1.3\n"
	if got := Notice("0.1.2", "v0.1.3", false); got != want {
		t.Fatalf("got %q", got)
	}
	if got := Notice("0.1.2", "v0.1.3", true); !strings.Contains(got, "Update with: "+SourceUpdate+"\n") {
		t.Fatalf("go bin notice %q", got)
	}
}

func TestInGoBin(t *testing.T) {
	gobin, gopath, home := t.TempDir(), t.TempDir(), t.TempDir()
	for _, d := range []string{filepath.Join(gopath, "bin"), filepath.Join(home, "go", "bin")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GOBIN", gobin)
	t.Setenv("GOPATH", gopath)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for dir, want := range map[string]bool{gobin: true, filepath.Join(gopath, "bin"): true, filepath.Join(home, "go", "bin"): true, t.TempDir(): false, gopath: false} {
		if got := InGoBin(filepath.Join(dir, "clarity")); got != want {
			t.Errorf("InGoBin(%s) = %v", dir, got)
		}
	}
}
