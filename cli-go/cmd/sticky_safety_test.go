package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// Safety settings from the environment cannot be switched off by a flag: an agent
// runner sets CLARITY_READ_ONLY and must be able to rely on it.
func TestEnvironmentReadOnlyIsSticky(t *testing.T) {
	t.Setenv("CLARITY_READ_ONLY", "true")
	t.Setenv("CLARITY_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out, errOut bytes.Buffer
	root := NewRoot(strings.NewReader(""), &out, &errOut)
	root.SetArgs([]string{"auth", "logout", "--read-only=false", "-o", "json"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("want a read-only refusal, got %v", err)
	}
}
