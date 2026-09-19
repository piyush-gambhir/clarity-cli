package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/piyush-gambhir/clarity-cli/cli-go/cmd"
	"github.com/spf13/cobra"
)

func main() {
	root := cmd.NewRoot(strings.NewReader(""), io.Discard, io.Discard)
	fmt.Println("# Clarity CLI command reference\n\nGenerated from the command tree. Run `make docs` to refresh.\n\nGlobal flags apply to every command.")
	fmt.Printf("\n```text\n%s```\n", root.PersistentFlags().FlagUsages())
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		fmt.Printf("\n## %s\n\n%s\n", c.CommandPath(), c.Short)
		if c.Long != "" {
			fmt.Printf("\n%s\n", c.Long)
		}
		fmt.Printf("\n```text\n%s\n```\n", c.UseLine())
		if c.Example != "" {
			fmt.Printf("\n```bash\n%s\n```\n", c.Example)
		}
		if c.LocalNonPersistentFlags().HasAvailableFlags() {
			fmt.Printf("\n```text\n%s```\n", c.LocalNonPersistentFlags().FlagUsages())
		}
		children := c.Commands()
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			walk(child)
		}
	}
	walk(root)
	_ = os.Stdout.Sync()
}
