package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/build"
	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/update"
)

const notice = "\nA new version of clarity is available: v0.1.2 -> v0.1.3\nUpdate with: clarity update\nRelease notes: https://github.com/piyush-gambhir/clarity-cli/releases/tag/v0.1.3\n"

type updateEnv struct {
	exe      string // stands in for the running executable
	cache    string
	src      update.Source
	hits     *atomic.Int32 // requests for the latest release
	atLookup atomic.Pointer[update.Cache]
	terminal bool
}

// newUpdateEnv isolates config and env, sets the build version, and serves
// latest-release v0.1.3 (plus this platform's archive) from a local server.
func newUpdateEnv(t *testing.T, version string) *updateEnv {
	t.Helper()
	isolate(t)
	for _, name := range []string{"CI", "CLARITY_NO_UPDATE_NOTIFIER", "NO_UPDATE_NOTIFIER", "GOBIN", "GOPATH"} {
		t.Setenv(name, "")
	}
	old := build.Version
	build.Version = version
	t.Cleanup(func() { build.Version = old })

	asset := update.AssetName(runtime.GOOS, runtime.GOARCH)
	archive := releaseArchive(t, runtime.GOOS, "new build")
	sums := fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), asset)
	e := &updateEnv{hits: new(atomic.Int32), terminal: true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			e.hits.Add(1)
			c := update.LoadCache(e.cache)
			e.atLookup.Store(&c)
			fmt.Fprint(w, `{"tag_name":"v0.1.3"}`)
		case "/download/v0.1.3/checksums.txt":
			fmt.Fprint(w, sums)
		case "/download/v0.1.3/" + asset:
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	e.src = update.Source{LatestURL: srv.URL + "/latest", DownloadURL: srv.URL + "/download/"}
	e.exe = filepath.Join(t.TempDir(), "clarity")
	if err := os.WriteFile(e.exe, []byte("old build"), 0o755); err != nil {
		t.Fatal(err)
	}
	var err error
	if e.cache, err = updateCachePath(); err != nil {
		t.Fatal(err)
	}
	return e
}

func releaseArchive(t *testing.T, goos, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	if goos == "windows" {
		zw := zip.NewWriter(&b)
		w, _ := zw.Create("clarity.exe")
		w.Write([]byte(content))
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	z := gzip.NewWriter(&b)
	tw := tar.NewWriter(z)
	if err := tw.WriteHeader(&tar.Header{Name: "clarity", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte(content))
	tw.Close()
	z.Close()
	return b.Bytes()
}

// run executes args and waits for any background check so tests are deterministic.
func (e *updateEnv) run(t *testing.T, input string, args ...string) (*app, string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	a := &app{in: strings.NewReader(input), out: &out, errOut: &errOut, releases: e.src,
		terminal: func(any) bool { return e.terminal }, executable: func() (string, error) { return e.exe, nil }}
	c := newRoot(a)
	c.SetArgs(args)
	err := c.Execute()
	if a.check != nil {
		<-a.check.done
	}
	return a, out.String(), errOut.String(), err
}

func (e *updateEnv) seed(t *testing.T, c update.Cache) {
	t.Helper()
	if err := c.Save(e.cache); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateNoticeSuppressed(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		env           map[string]string
		notTerminal   bool
		args          []string
	}{
		{name: "stderr not a terminal", notTerminal: true},
		{name: "CI", env: map[string]string{"CI": "true"}},
		{name: "CLARITY_NO_UPDATE_NOTIFIER", env: map[string]string{"CLARITY_NO_UPDATE_NOTIFIER": "1"}},
		{name: "NO_UPDATE_NOTIFIER", env: map[string]string{"NO_UPDATE_NOTIFIER": "1"}},
		{name: "quiet flag", args: []string{"export", "dimensions", "-q"}},
		{name: "quiet env", env: map[string]string{"CLARITY_QUIET": "1"}},
		{name: "dev build", version: "dev"},
		{name: "empty version", version: " "},
		{name: "non-semver version", version: "abc123"},
		{name: "version command", args: []string{"version"}},
		{name: "completion command", args: []string{"completion", "zsh"}},
		{name: "help command", args: []string{"help", "export"}},
		{name: "shell completion", args: []string{"__complete", "exp"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := "0.1.2"
			if tc.version != "" {
				version = strings.TrimSpace(tc.version)
			}
			e := newUpdateEnv(t, version)
			e.terminal = !tc.notTerminal
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			args := tc.args
			if args == nil {
				args = []string{"export", "dimensions"}
			}
			a, _, errOut, err := e.run(t, "", args...)
			if err != nil {
				t.Fatal(err)
			}
			if a.check != nil || e.hits.Load() != 0 || strings.Contains(errOut, "new version") {
				t.Fatalf("check started=%v hits=%d stderr=%q", a.check != nil, e.hits.Load(), errOut)
			}
			if _, err := os.Stat(e.cache); !os.IsNotExist(err) {
				t.Fatalf("cache written: %v", err)
			}
		})
	}
}

func TestUpdateNoticeOncePerDay(t *testing.T) {
	e := newUpdateEnv(t, "0.1.2")
	// No cache: one background lookup, recorded for the next command.
	if _, _, _, err := e.run(t, "", "export", "dimensions"); err != nil {
		t.Fatal(err)
	}
	if got := update.LoadCache(e.cache); e.hits.Load() != 1 || got.LatestVersion != "v0.1.3" || !got.Fresh(time.Now()) {
		t.Fatalf("hits=%d cache=%+v", e.hits.Load(), got)
	}
	// The attempt was saved before the request, so a command that exits before
	// GitHub answers still counts as the day's check.
	if c := e.atLookup.Load(); c == nil || !c.Fresh(time.Now()) {
		t.Fatalf("attempt not recorded before the lookup: %+v", c)
	}
	// Fresh cache: no network; the notice prints after stdout, once.
	e.seed(t, update.Cache{CheckedAt: time.Now(), LatestVersion: "v0.1.3"})
	_, out, errOut, err := e.run(t, "", "export", "dimensions")
	if err != nil || out == "" || errOut != notice {
		t.Fatalf("err=%v stdout=%q stderr=%q", err, out, errOut)
	}
	if _, _, errOut, _ := e.run(t, "", "export", "dimensions"); errOut != "" {
		t.Fatalf("notice repeated: %q", errOut)
	}
	if e.hits.Load() != 1 {
		t.Fatalf("fresh cache hit the network: %d", e.hits.Load())
	}
	// A day later it shows again.
	c := update.LoadCache(e.cache)
	c.NotifiedAt = time.Now().Add(-25 * time.Hour)
	e.seed(t, c)
	if _, _, errOut, _ := e.run(t, "", "export", "dimensions"); errOut != notice {
		t.Fatalf("no notice after a day: %q", errOut)
	}
	// In a Go bin directory the update line points at the source install.
	t.Setenv("GOBIN", filepath.Dir(e.exe))
	e.seed(t, update.Cache{CheckedAt: time.Now(), LatestVersion: "v0.1.3"})
	if _, _, errOut, _ := e.run(t, "", "export", "dimensions"); !strings.Contains(errOut, "Update with: "+update.SourceUpdate+"\n") {
		t.Fatalf("go bin notice: %q", errOut)
	}
}

func TestUpdateNoticeFailedCheckIsCached(t *testing.T) {
	e := newUpdateEnv(t, "0.1.2")
	e.src.LatestURL += "-missing"
	for range 2 {
		if _, _, errOut, err := e.run(t, "", "export", "dimensions"); err != nil || errOut != "" {
			t.Fatalf("err=%v stderr=%q", err, errOut)
		}
	}
	if got := update.LoadCache(e.cache); !got.Fresh(time.Now()) || got.LatestVersion != "" {
		t.Fatalf("failed check not cached: %+v", got)
	}
}

func TestUpdateCheckJSON(t *testing.T) {
	e := newUpdateEnv(t, "0.1.2")
	// --check bypasses a fresh cache that says nothing is newer.
	e.seed(t, update.Cache{CheckedAt: time.Now(), LatestVersion: "v0.1.2"})
	check := func() updateStatus {
		t.Helper()
		_, out, _, err := e.run(t, "", "update", "--check", "--read-only", "-o", "json")
		if err != nil {
			t.Fatal(err)
		}
		var raw map[string]any
		var s updateStatus
		if json.Unmarshal([]byte(out), &raw) != nil || json.Unmarshal([]byte(out), &s) != nil || len(raw) != 5 {
			t.Fatalf("output %s", out)
		}
		return s
	}
	want := updateStatus{CurrentVersion: "v0.1.2", LatestVersion: "v0.1.3", UpdateAvailable: true, ReleaseURL: "https://github.com/piyush-gambhir/clarity-cli/releases/tag/v0.1.3", InstallMethod: "self"}
	if got := check(); got != want || e.hits.Load() != 1 {
		t.Fatalf("got %+v hits=%d", got, e.hits.Load())
	}
	t.Setenv("GOBIN", filepath.Dir(e.exe))
	want.InstallMethod = "go"
	if got := check(); got != want {
		t.Fatalf("go install: %+v", got)
	}
}

func TestUpdateAlreadyLatest(t *testing.T) {
	e := newUpdateEnv(t, "0.1.3")
	_, out, _, err := e.run(t, "", "update", "--yes")
	if err != nil || out != "clarity v0.1.3 is up to date (latest release: v0.1.3).\n" {
		t.Fatalf("err=%v out=%q", err, out)
	}
	assertExe(t, e.exe, "old build")
}

func TestUpdateConfirmation(t *testing.T) {
	e := newUpdateEnv(t, "0.1.2")
	if _, _, _, err := e.run(t, "", "update", "--no-input"); err == nil || !strings.Contains(err.Error(), "pass --yes") {
		t.Fatalf("no-input without --yes: %v", err)
	}
	_, out, errOut, err := e.run(t, "n\n", "update")
	if err != nil || out != "Update cancelled.\n" || errOut != "clarity v0.1.2 -> v0.1.3\nUpdate now? [Y/n] " {
		t.Fatalf("decline: err=%v out=%q stderr=%q", err, out, errOut)
	}
	if _, _, _, err := e.run(t, "", "update", "--read-only", "--yes"); err == nil {
		t.Fatal("read-only allowed an install")
	}
	assertExe(t, e.exe, "old build")
}

func TestUpdateInstalls(t *testing.T) {
	e := newUpdateEnv(t, "0.1.2")
	e.seed(t, update.Cache{CheckedAt: time.Now(), LatestVersion: "v0.1.3"})
	// Enter accepts the default.
	_, out, _, err := e.run(t, "\n", "update")
	if err != nil || out != "Updated clarity v0.1.2 -> v0.1.3\nRelease notes: https://github.com/piyush-gambhir/clarity-cli/releases/tag/v0.1.3\n" {
		t.Fatalf("err=%v out=%q", err, out)
	}
	assertExe(t, e.exe, "new build")
	if _, err := os.Stat(e.cache); !os.IsNotExist(err) {
		t.Fatalf("update cache not cleared: %v", err)
	}
}

func TestUpdateGoInstallDoesNotReplace(t *testing.T) {
	e := newUpdateEnv(t, "0.1.2")
	t.Setenv("GOBIN", filepath.Dir(e.exe))
	_, out, _, err := e.run(t, "", "update", "--yes")
	if err != nil || !strings.Contains(out, "Update with: "+update.SourceUpdate+"\n") {
		t.Fatalf("err=%v out=%q", err, out)
	}
	assertExe(t, e.exe, "old build")
}

func TestVersionShowsCachedLatest(t *testing.T) {
	e := newUpdateEnv(t, "0.1.2")
	version := func() map[string]any {
		t.Helper()
		_, out, _, err := e.run(t, "", "version", "-o", "json")
		var got map[string]any
		if err != nil || json.Unmarshal([]byte(out), &got) != nil {
			t.Fatalf("err=%v out=%s", err, out)
		}
		return got
	}
	if got := version(); got["latest"] != nil || got["update_available"] != nil {
		t.Fatalf("unknown latest shown: %v", got)
	}
	e.seed(t, update.Cache{CheckedAt: time.Now().Add(-48 * time.Hour), LatestVersion: "v0.1.3"})
	if got := version(); got["latest"] != "v0.1.3" || got["update_available"] != true || got["version"] != "0.1.2" {
		t.Fatalf("cached latest missing: %v", got)
	}
	if e.hits.Load() != 0 {
		t.Fatal("version contacted GitHub")
	}
}

func assertExe(t *testing.T, path, want string) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("executable = %q, %v; want %q", got, err, want)
	}
}
