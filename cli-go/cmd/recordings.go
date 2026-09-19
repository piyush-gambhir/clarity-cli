package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/client"
	"github.com/spf13/cobra"
)

func (a *app) recordings() *cobra.Command {
	group := &cobra.Command{Use: "recordings", Aliases: []string{"sessions"}, Short: "Find sampled session recordings and interaction timelines"}
	var days, count int
	var startString, endString, sortBy, file, visitedURL string
	arrays := map[string]*[]string{}
	booleans := map[string]*bool{}
	c := &cobra.Command{Use: "list", Short: "List up to 250 sampled recordings", Args: cobra.NoArgs,
		Long:    "List sampled recordings using Microsoft's MCP backend. This is not a full recording export.\nDefaults to the previous 2 days. Dates accept RFC3339 or YYYY-MM-DD (midnight UTC).\n--filters-file accepts a JSON filters object; explicit flags override matching fields.\nUse recordings filters for the accepted schema. Range objects require min and max (number or null).",
		Example: "  clarity recordings list --days 7 --device Mobile --rage-clicks -o json\n  clarity recordings list --start 2026-09-01 --end 2026-09-08 --count 25\n  clarity recordings list --filters-file filters.json -o yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			filters := map[string]any{}
			if file != "" {
				var r io.Reader = a.in
				if file != "-" {
					f, err := os.Open(file)
					if err != nil {
						return err
					}
					defer f.Close()
					r = f
				}
				b, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
				if err != nil {
					return err
				}
				if len(b) > 1<<20 {
					return fmt.Errorf("filters file exceeds 1 MiB")
				}
				if err := json.Unmarshal(b, &filters); err != nil || filters == nil {
					return fmt.Errorf("filters file must contain a JSON object")
				}
			}
			if cmd.Flags().Changed("days") && (startString != "" || filters["date"] != nil) {
				return fmt.Errorf("--days cannot be combined with --start or a filters-file date")
			}
			if days < 1 || days > 3650 {
				return fmt.Errorf("--days must be between 1 and 3650; availability depends on Clarity retention")
			}
			if date, exists := filters["date"]; exists {
				m, ok := date.(map[string]any)
				if !ok {
					return fmt.Errorf("filters.date must contain start and end strings")
				}
				if startString == "" {
					startString, _ = m["start"].(string)
					if startString == "" {
						return fmt.Errorf("filters.date.start is required")
					}
				}
				if endString == "" {
					endString, _ = m["end"].(string)
					if endString == "" {
						return fmt.Errorf("filters.date.end is required")
					}
				}
			}
			end := time.Now().UTC()
			var err error
			if endString != "" {
				end, err = parseDate(endString)
				if err != nil {
					return err
				}
			}
			start := end.Add(-time.Duration(days) * 24 * time.Hour)
			if startString != "" {
				start, err = parseDate(startString)
				if err != nil {
					return err
				}
			}
			for flag, values := range arrays {
				if !cmd.Flags().Changed(flag) {
					continue
				}
				key := map[string]string{"device": "deviceType", "event": "smartEvents", "javascript-error": "javascriptErrors"}[flag]
				if key == "" {
					key = flag
				}
				items := make([]any, len(*values))
				for i, v := range *values {
					items[i] = v
				}
				filters[key] = items
			}
			for flag, value := range booleans {
				if cmd.Flags().Changed(flag) {
					key := map[string]string{"rage-clicks": "rageClickPresent", "dead-clicks": "deadClickPresent", "quick-backs": "quickbackClickPresent", "excessive-scroll": "excessiveScrollPresent"}[flag]
					filters[key] = *value
				}
			}
			if cmd.Flags().Changed("url") {
				filters["visitedUrls"] = []any{map[string]any{"url": visitedURL, "operator": "contains"}}
			}
			body, err := client.RecordingBody(start, end, count, sortBy, filters)
			if err != nil {
				return err
			}
			cl, err := a.client()
			if err != nil {
				return err
			}
			data, err := cl.Recordings(cmd.Context(), body)
			if err != nil {
				return err
			}
			return a.print(data)
		},
	}
	f := c.Flags()
	f.IntVar(&days, "days", 2, "Lookback days; used when start is not specified")
	f.IntVar(&count, "count", 100, "Number of sampled recordings (1–250)")
	f.StringVar(&startString, "start", "", "Start date/time, inclusive interval boundary sent to Clarity")
	f.StringVar(&endString, "end", "", "End date/time sent to Clarity (default: now)")
	f.StringVar(&sortBy, "sort", "SessionStart_DESC", "Sort: "+strings.Join(client.SortOptions, ", "))
	f.StringVar(&file, "filters-file", "", "JSON filters file, or - for stdin")
	f.StringVar(&visitedURL, "url", "", "Match a substring of a visited URL")
	for _, flag := range []string{"device", "browser", "os", "country", "city", "state", "source", "medium", "campaign", "channel", "event", "javascript-error"} {
		arrays[flag] = f.StringArray(flag, nil, "Filter by "+flag+" (repeat flag for multiple values)")
	}
	for _, flag := range []string{"rage-clicks", "dead-clicks", "quick-backs", "excessive-scroll"} {
		booleans[flag] = f.Bool(flag, false, "Filter sessions by "+flag+"; explicit =false is supported")
	}
	group.AddCommand(c, &cobra.Command{Use: "filters", Short: "List supported JSON filter fields and enum values (offline)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return a.print(client.FilterSchema()) }})
	return group
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q; use RFC3339 or YYYY-MM-DD", s)
}
