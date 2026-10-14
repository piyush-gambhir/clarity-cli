package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// commands resolve uses the real parser: a flag before the subcommand cannot hide it.
func TestCommandsResolve(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		effect string
	}{{[]string{"export", "--days", "1"}, "read"}, {[]string{"-o", "json", "auth", "logout"}, "local_write"}} {
		var out bytes.Buffer
		root := NewRoot(strings.NewReader(""), &out, &bytes.Buffer{})
		root.SetArgs(append([]string{"commands", "resolve", "--"}, tc.args...))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		var got resolution
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Effect != tc.effect {
			t.Errorf("%v: got %+v (%v)", tc.args, got, err)
		}
	}
	root := NewRoot(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	root.SetArgs([]string{"commands", "resolve", "--", "nonsense"})
	if root.Execute() == nil {
		t.Fatal("an unknown command resolved")
	}
}
