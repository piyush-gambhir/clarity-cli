package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"unicode"

	"go.yaml.in/yaml/v3"
)

func Valid(format string) bool { return format == "table" || format == "json" || format == "yaml" }

func Print(w io.Writer, format string, data any) error {
	if !Valid(format) {
		return fmt.Errorf("unsupported output %q; use table, json, or yaml", format)
	}
	if format == "json" {
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		return e.Encode(data)
	}
	// Normalize typed structs and maps through JSON while preserving numeric precision.
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if format == "yaml" {
		var node yaml.Node
		if err := yaml.Unmarshal(b, &node); err != nil {
			return err
		}
		var block func(*yaml.Node)
		block = func(n *yaml.Node) {
			n.Style = 0
			for _, child := range n.Content {
				block(child)
			}
		}
		block(&node)
		e := yaml.NewEncoder(w)
		e.SetIndent(2)
		defer e.Close()
		return e.Encode(&node)
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var normalized any
	if err := dec.Decode(&normalized); err != nil {
		return err
	}
	if rows, ok := normalized.([]any); ok {
		if len(rows) == 0 {
			_, err := fmt.Fprintln(w, "No results.")
			return err
		}
		// Export API: each metric has a separate information table.
		if m, ok := rows[0].(map[string]any); ok && m["metricName"] != nil && m["information"] != nil {
			for _, row := range rows {
				m, ok := row.(map[string]any)
				if !ok {
					return table(w, rows)
				}
				fmt.Fprintln(w, cell(m["metricName"]))
				if err := Print(w, "table", m["information"]); err != nil {
					return err
				}
				fmt.Fprintln(w)
			}
			return nil
		}
		return table(w, rows)
	}
	if m, ok := normalized.(map[string]any); ok {
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, key := range keys {
			fmt.Fprintf(tw, "%s\t%s\n", cell(key), cell(m[key]))
		}
		return tw.Flush()
	}
	_, err = fmt.Fprintln(w, cell(normalized))
	return err
}

func table(w io.Writer, rows []any) error {
	keysSet := map[string]bool{}
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			for key := range m {
				keysSet[key] = true
			}
		} else {
			for _, r := range rows {
				if _, err := fmt.Fprintln(w, cell(r)); err != nil {
					return err
				}
			}
			return nil
		}
	}
	keys := make([]string, 0, len(keysSet))
	for key := range keysSet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	headers := make([]string, len(keys))
	for i, key := range keys {
		headers[i] = cell(key)
	}
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		m := row.(map[string]any)
		cells := make([]string, len(keys))
		for i, key := range keys {
			cells[i] = cell(m[key])
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

func cell(v any) string {
	if v == nil {
		return "-"
	}
	s, ok := v.(string)
	if !ok {
		b, _ := json.Marshal(v)
		s = string(b)
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
