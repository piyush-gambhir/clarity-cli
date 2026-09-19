package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the real installer using local release fixtures and a fake curl.
// No external requests or system installation are performed.
func TestInstaller(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer; Windows uses ZIP downloads")
	}
	script, err := filepath.Abs("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, version       string
		tampered, wantError bool
	}{
		{"latest", "", false, false}, {"pinned", "v0.1.0", false, false}, {"unprefixed", "0.1.0", false, false}, {"prerelease", "v0.1.0-rc.1", false, false}, {"tampered", "v0.1.0", true, true}, {"invalid", "../oops", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			fixture := filepath.Join(tmp, "fixture")
			bin := filepath.Join(tmp, "fake-bin")
			install := filepath.Join(tmp, "installed")
			for _, dir := range []string{fixture, bin} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			content := []byte("#!/bin/sh\necho clarity-test\n")
			if err := tw.WriteHeader(&tar.Header{Name: "clarity", Mode: 0755, Size: int64(len(content))}); err != nil {
				t.Fatal(err)
			}
			tw.Write(content)
			tw.Close()
			gz.Close()
			asset := fmt.Sprintf("clarity-cli_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
			if err := os.WriteFile(filepath.Join(fixture, asset), buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(buf.Bytes())
			if tc.tampered {
				hash = sha256.Sum256([]byte("wrong"))
			}
			if err := os.WriteFile(filepath.Join(fixture, "checksums.txt"), []byte(fmt.Sprintf("%x  %s\n", hash, asset)), 0600); err != nil {
				t.Fatal(err)
			}
			fake := `#!/bin/sh
set -eu
url=''
out=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    https://*) url="$1" ;;
    -o) shift; out="$1" ;;
  esac
  shift
done
cp "$FIXTURE_DIR/${url##*/}" "$out"
`
			if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(fake), 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "FIXTURE_DIR="+fixture, "VERSION="+tc.version, "INSTALL_DIR="+install)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v output=%s", err, out)
			}
			if tc.wantError {
				if _, err := os.Stat(filepath.Join(install, "clarity")); !os.IsNotExist(err) {
					t.Fatal("installed invalid release")
				}
				return
			}
			got, err := exec.Command(filepath.Join(install, "clarity")).Output()
			if err != nil || strings.TrimSpace(string(got)) != "clarity-test" {
				t.Fatalf("installed binary: %s %v", got, err)
			}
		})
	}
}
