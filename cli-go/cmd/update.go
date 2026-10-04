package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/build"
	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/config"
	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/update"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type updateStatus struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseURL      string `json:"release_url"`
	InstallMethod   string `json:"install_method"` // "self" (release binary) or "go" (Go bin directory)
	Updated         bool   `json:"updated,omitempty"`
}

func (a *app) update() *cobra.Command {
	var check, yes bool
	c := &cobra.Command{Use: "update", Short: "Check for or install the latest release", Args: cobra.NoArgs,
		Long: `Check for or install the latest clarity release from GitHub.

update downloads this platform's release archive (tar.gz on macOS and Linux, zip
on Windows), verifies its SHA-256 checksum against the release's checksums.txt,
and replaces the running executable. On Windows the running clarity.exe is renamed
to clarity.exe.old and the leftover is removed on a later start. In a terminal it
asks before installing; --yes skips the prompt and is required with --no-input.
If the executable's directory is not writable, nothing changes: re-run with sudo
or reinstall with the install script into a writable directory. A build in a Go
bin directory ($GOBIN, $GOPATH/bin, ~/go/bin) is not replaced; update prints the
source install command instead. --check only reports and works with --read-only.

Update notice: at most once a day, in an interactive terminal, clarity checks
GitHub for a newer release in the background and prints a short notice on stderr
after the command's output. It never runs when stderr is not a terminal, when CI
is set, with --quiet, for development builds, or for update, version, completion,
and help. Turn it off with CLARITY_NO_UPDATE_NOTIFIER=1 or NO_UPDATE_NOTIFIER=1.`,
		Example: "  clarity update --check\n  clarity update --check -o json\n  clarity update --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.readOnly && !check {
				return fmt.Errorf("self-update is blocked by --read-only; use update --check")
			}
			r, err := a.releases.Latest(cmd.Context())
			if err != nil {
				return err
			}
			current := update.Display(build.Version)
			s := updateStatus{
				CurrentVersion: current, LatestVersion: r.Tag, ReleaseURL: r.URL, InstallMethod: "self",
				// A dev or unknown build can always move to the published release.
				UpdateAvailable: update.Newer(r.Tag, build.Version) || !update.IsRelease(build.Version),
			}
			exe, exeErr := a.executable()
			if exeErr == nil && update.InGoBin(exe) {
				s.InstallMethod = "go"
			}
			if check {
				text := fmt.Sprintf("Current version: %s\nLatest version: %s\nUpdate available: no\nRelease notes: %s", current, r.Tag, r.URL)
				if s.UpdateAvailable {
					text = fmt.Sprintf("Current version: %s\nLatest version: %s\nUpdate available: yes\nUpdate with: %s\nRelease notes: %s", current, r.Tag, updateCommand(s), r.URL)
				}
				return a.result(s, text)
			}
			if !s.UpdateAvailable {
				return a.result(s, fmt.Sprintf("clarity %s is up to date (latest release: %s).", current, r.Tag))
			}
			if s.InstallMethod == "go" {
				return a.result(s, strings.TrimSuffix(update.Notice(build.Version, r.Tag, true), "\n"))
			}
			if !yes {
				if a.noInput {
					return fmt.Errorf("updating clarity %s -> %s needs confirmation; pass --yes", current, r.Tag)
				}
				if a.terminal(a.in) {
					fmt.Fprintf(a.errOut, "clarity %s -> %s\nUpdate now? [Y/n] ", current, r.Tag)
					line, err := bufio.NewReader(a.in).ReadString('\n')
					answer := strings.ToLower(strings.TrimSpace(line))
					if (err != nil && line == "") || (answer != "" && answer != "y" && answer != "yes") {
						return a.result(s, "Update cancelled.")
					}
				}
			}
			if exeErr != nil {
				return fmt.Errorf("locate the running executable: %w", exeErr)
			}
			a.info("Downloading clarity %s...", r.Tag)
			if err := a.releases.Install(cmd.Context(), r.Tag, update.Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Exe: exe}); err != nil {
				return err
			}
			if path, err := updateCachePath(); err == nil {
				_ = os.Remove(path)
			}
			s.Updated = true
			return a.result(s, fmt.Sprintf("Updated clarity %s -> %s\nRelease notes: %s", current, r.Tag, r.URL))
		}}
	c.Flags().BoolVar(&check, "check", false, "Only report the latest release (always queries GitHub)")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Install without asking for confirmation")
	return c
}

func updateCommand(s updateStatus) string {
	if s.InstallMethod == "go" {
		return update.SourceUpdate
	}
	return "clarity update"
}

// result prints human text in table mode and the status in JSON/YAML.
func (a *app) result(s updateStatus, text string) error {
	if a.format == "table" {
		_, err := fmt.Fprintln(a.out, text)
		return err
	}
	return a.print(s)
}

// updateCheck carries the background release check from PersistentPreRun to PersistentPostRun.
type updateCheck struct {
	done chan struct{} // closed once the cache holds this run's result
	path string
}

// startUpdateCheck begins the release check behind the update notice. A fresh
// cache answers at once; otherwise one GitHub lookup runs in the background.
// Scripts, CI, opt-outs, quiet mode, dev builds, and the commands that report
// versions themselves never check.
func (a *app) startUpdateCheck(cmd *cobra.Command) {
	switch name := cmd.Name(); {
	case name == "update", name == "version", name == "completion", name == "help", strings.HasPrefix(name, "__complete"):
		return
	}
	if a.quiet || !update.IsRelease(build.Version) || os.Getenv("CI") != "" ||
		os.Getenv("CLARITY_NO_UPDATE_NOTIFIER") != "" || os.Getenv("NO_UPDATE_NOTIFIER") != "" || !a.terminal(a.errOut) {
		return
	}
	path, err := updateCachePath()
	if err != nil {
		return
	}
	c := &updateCheck{done: make(chan struct{}), path: path}
	a.check = c
	cache := update.LoadCache(path)
	if cache.Fresh(time.Now()) {
		close(c.done)
		return
	}
	// Record the attempt before the request: a lookup that fails, or that the
	// process exits before, counts as the day's check, so GitHub is asked at most
	// once a day.
	cache.CheckedAt = time.Now()
	if cache.Save(path) != nil {
		return
	}
	ctx := cmd.Context()
	go func() {
		defer close(c.done)
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		r, err := a.releases.Latest(ctx)
		if err != nil {
			return
		}
		// Reload so a notice another command recorded meanwhile is kept.
		cache := update.LoadCache(path)
		cache.CheckedAt, cache.LatestVersion = time.Now(), r.Tag
		_ = cache.Save(path)
	}()
}

// printUpdateNotice prints the notice only if the check has already finished;
// it never delays the command.
func (a *app) printUpdateNotice() {
	c := a.check
	if c == nil {
		return
	}
	select {
	case <-c.done:
	default:
		return
	}
	// Read the cache now, not at start, so commands that ran meanwhile and
	// already showed this release keep it to once a day.
	cache := update.LoadCache(c.path)
	if !cache.ShouldNotify(build.Version, time.Now()) {
		return
	}
	exe, err := a.executable()
	fmt.Fprint(a.errOut, "\n"+update.Notice(build.Version, cache.LatestVersion, err == nil && update.InGoBin(exe)))
	cache.NotifiedVersion, cache.NotifiedAt = cache.LatestVersion, time.Now()
	_ = cache.Save(c.path)
}

func updateCachePath() (string, error) {
	p, err := config.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), update.CacheFile), nil
}

// executable is the running binary with symlinks resolved, the file an update replaces.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func isTerminal(stream any) bool {
	f, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
