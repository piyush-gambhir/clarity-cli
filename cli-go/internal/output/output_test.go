package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestFormatsPreserveTypesAndPrecision(t *testing.T) {
	data := map[string]any{"id": json.Number("9007199254740993"), "numeric_string": "0012", "name": "page", "ok": true}
	for _, format := range []string{"json", "yaml"} {
		var b bytes.Buffer
		if err := Print(&b, format, data); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), "9007199254740993") {
			t.Fatal("lost precision", b.String())
		}
		if format == "yaml" {
			var got map[string]any
			if err := yaml.Unmarshal(b.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["numeric_string"] != "0012" || got["ok"] != true {
				t.Fatalf("wrong types: %#v", got)
			}
		}
	}
}

func TestTableEscapesControls(t *testing.T) {
	var b bytes.Buffer
	if err := Print(&b, "table", []map[string]any{{"name": "\x1b[31mtest\nrow\tvalue"}}); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(b.String(), '\x1b') {
		t.Fatal("terminal escape leaked")
	}
	if err := Print(&b, "bad", nil); err == nil {
		t.Fatal("bad format accepted")
	}
}
