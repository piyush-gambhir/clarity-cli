package cmd

import (
	"fmt"
	"strings"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/build"
	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/update"
	"github.com/spf13/cobra"
)

func (a *app) update() *cobra.Command {
	var check bool
	c := &cobra.Command{Use: "update", Short: "Install the latest GitHub release after SHA-256 verification", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if a.readOnly && !check {
			return fmt.Errorf("self-update is blocked by --read-only; use update --check")
		}
		r, err := update.Latest(cmd.Context())
		if err != nil {
			return err
		}
		different := strings.TrimPrefix(build.Version, "v") != strings.TrimPrefix(r.Tag, "v")
		if check || !different {
			return a.print(map[string]any{"current": build.Version, "latest": r.Tag, "different": different, "release_url": r.URL})
		}
		a.info("Installing %s...", r.Tag)
		if err := update.Install(cmd.Context(), r); err != nil {
			return err
		}
		return a.print(map[string]any{"version": r.Tag, "updated": true})
	}}
	c.Flags().BoolVar(&check, "check", false, "Only check the latest published release")
	return c
}
