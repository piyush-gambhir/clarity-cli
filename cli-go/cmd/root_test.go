package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/client"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("CLARITY_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	for _, name := range []string{"CLARITY_API_TOKEN", "CLARITY_PROFILE", "CLARITY_NO_INPUT", "CLARITY_QUIET", "CLARITY_VERBOSE", "CLARITY_READ_ONLY"} {
		t.Setenv(name, "")
	}
}

func TestAuthLifecycle(t *testing.T) {
	isolate(t)
	run := func(input string, args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &errOut)
		return code, out.String(), errOut.String()
	}
	for _, name := range []string{"one", "two"} {
		code, out, err := run("secret-"+name+"\n", "auth", "login", "--profile", name, "--token-stdin", "--no-input", "-o", "json")
		if code != 0 || strings.Contains(out+err, "secret-") {
			t.Fatalf("login: %d %s %s", code, out, err)
		}
	}
	if code, _, err := run("", "auth", "use", "one"); code != 0 {
		t.Fatal(err)
	}
	code, out, err := run("", "auth", "status", "-o", "json")
	if code != 0 || !strings.Contains(out, "profile:one") || strings.Contains(out+err, "secret-") {
		t.Fatalf("status %d %s %s", code, out, err)
	}
	if code, out, err := run("", "auth", "list", "-o", "yaml"); code != 0 || !strings.Contains(out, "two") || strings.Contains(out+err, "secret-") {
		t.Fatalf("list %d %s %s", code, out, err)
	}
	if code, _, _ := run("", "auth", "logout", "--read-only"); code == 0 {
		t.Fatal("read-only allowed logout")
	}
	if code, _, err := run("", "auth", "logout"); code != 0 {
		t.Fatal(err)
	}
	if code, _, _ := run("", "auth", "status"); code == 0 {
		t.Fatal("status succeeded after logout")
	}
	if code, _, _ := run("", "auth", "login", "--no-input"); code == 0 {
		t.Fatal("missing noninteractive token accepted")
	}
}

func TestCommandRecordingRequest(t *testing.T) {
	isolate(t)
	t.Setenv("CLARITY_API_TOKEN", "secret")
	var out, errOut bytes.Buffer
	calls := 0
	a := &app{in: strings.NewReader(`{"rageClickPresent":true,"sessionDuration":{"min":1,"max":null}}`), out: &out, errOut: &errOut, newClient: func(token string, timeout time.Duration) *client.Client {
		c := client.New(token, timeout)
		c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			f := body["filters"].(map[string]any)
			if f["rageClickPresent"] != false || f["deviceType"].([]any)[0] != "Mobile" || body["count"] != float64(5) || body["start"] != "2026-09-01T00:00:00.000Z" {
				t.Fatalf("bad request %#v", body)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"session":"sample"}]`))}, nil
		})
		return c
	}}
	c := newRoot(a)
	c.SetArgs([]string{"recordings", "list", "--start", "2026-09-01", "--end", "2026-09-02", "--device", "Mobile", "--rage-clicks=false", "--count", "5", "--filters-file", "-", "--read-only", "-o", "json"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !json.Valid(out.Bytes()) || errOut.Len() != 0 {
		t.Fatalf("bad output %s %s calls=%d", out.String(), errOut.String(), calls)
	}
}

func TestValidationBeforeNetwork(t *testing.T) {
	isolate(t)
	for _, args := range [][]string{{"export", "--days", "4"}, {"export", "--dimension", "bad"}, {"recordings", "list", "--count", "251"}, {"recordings", "list", "--start", "bad"}, {"analytics", "query", "hi", "--timezone", "bogus"}, {"export", "-o", "csv"}} {
		var b bytes.Buffer
		c := newRoot(&app{in: strings.NewReader(""), out: &b, errOut: &b, newClient: func(string, time.Duration) *client.Client { t.Fatal("created client for invalid input"); return nil }})
		c.SetArgs(args)
		if err := c.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestMachineErrorsAndOfflineCommands(t *testing.T) {
	isolate(t)
	var out, errOut bytes.Buffer
	if code := Run(context.Background(), []string{"export", "-o", "json"}, strings.NewReader(""), &out, &errOut); code == 0 || out.Len() != 0 || !json.Valid(errOut.Bytes()) {
		t.Fatalf("code/output/error %d %s %s", code, out.String(), errOut.String())
	}
	for _, args := range [][]string{{"version", "-o", "json"}, {"export", "dimensions", "-o", "json"}, {"recordings", "filters", "-o", "json"}, {"completion", "zsh"}, {"--help"}} {
		out.Reset()
		errOut.Reset()
		if code := Run(context.Background(), args, strings.NewReader(""), &out, &errOut); code != 0 || out.Len() == 0 {
			t.Fatalf("offline %v: %s", args, errOut.String())
		}
	}
}
