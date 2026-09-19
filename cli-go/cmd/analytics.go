package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/client"
	"github.com/spf13/cobra"
)

func (a *app) export() *cobra.Command {
	var days int
	var dimensions []string
	c := &cobra.Command{Use: "export", Short: "Export dashboard metrics for the previous 1–3 days", Args: cobra.NoArgs,
		Long:    "Download aggregate dashboard data from the documented Export API.\nLimits: 10 requests/project/day; previous 1–3 days; up to 3 dimensions;\n1,000 rows without pagination. Times are UTC. No automatic retries.",
		Example: "  clarity export --days 3 --dimension Device --dimension Country/Region -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := client.ExportParams(days, dimensions); err != nil {
				return err
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			data, err := c.Export(cmd.Context(), days, dimensions)
			if err != nil {
				return err
			}
			return a.print(data)
		},
	}
	c.Flags().IntVar(&days, "days", 1, "Lookback window in days (1, 2, or 3)")
	c.Flags().StringSliceVar(&dimensions, "dimension", nil, "Group by dimension; repeat up to 3 times (see export dimensions)")
	c.AddCommand(&cobra.Command{Use: "dimensions", Short: "List supported export dimensions (offline)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return a.print(client.Dimensions) }})
	return c
}

func (a *app) analytics() *cobra.Command {
	var timezone string
	c := &cobra.Command{Use: "analytics", Short: "Query analytics through the endpoints used by Microsoft's MCP server"}
	query := &cobra.Command{Use: "query QUESTION", Short: "Ask a focused analytics question with an explicit time range", Args: cobra.ExactArgs(1),
		Example: "  clarity analytics query 'Top pages by rage clicks in the last 3 days' --timezone Asia/Kolkata -o json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("query must not be empty")
			}
			if _, err := time.LoadLocation(timezone); err != nil {
				return fmt.Errorf("invalid IANA timezone %q", timezone)
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			data, err := c.Query(cmd.Context(), args[0], timezone)
			if err != nil {
				return err
			}
			return a.print(data)
		},
	}
	query.Flags().StringVar(&timezone, "timezone", "UTC", "IANA timezone used to interpret the query")
	c.AddCommand(query)
	return c
}

func (a *app) docs() *cobra.Command {
	c := &cobra.Command{Use: "docs", Short: "Search Microsoft Clarity documentation"}
	c.AddCommand(&cobra.Command{Use: "search QUESTION", Short: "Retrieve documentation snippets (requires a project token)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(args[0]) == "" {
			return fmt.Errorf("query must not be empty")
		}
		c, err := a.client()
		if err != nil {
			return err
		}
		data, err := c.Documentation(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return a.print(data)
	}})
	return c
}
