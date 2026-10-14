package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resolution is what one set of arguments would run, decided by the real parser.
type resolution struct {
	Command     string            `json:"command"`
	Effect      string            `json:"effect"` // read, remote_write, or local_write
	Conditional bool              `json:"conditional"`
	Interactive bool              `json:"interactive"`
	Secret      bool              `json:"secret_output"`
	Flags       map[string]string `json:"flags"`
	Args        []string          `json:"args"`
}

// commandsCmd groups offline discovery for agents. resolve answers "what would these
// arguments run?" without running them, so a runner can authorise by effect.
func (a *app) commandsCmd() *cobra.Command {
	c := &cobra.Command{Use: "commands", Short: "Offline command discovery for agents"}
	c.AddCommand(&cobra.Command{Use: "resolve -- <arguments>", Short: "Show the command, effect and flags that arguments would run, without running them",
		DisableFlagParsing: true,
		Example:            "  clarity commands resolve -- sites list",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Everything after "--" is the command being resolved; the answer is always JSON.
			for i, arg := range args {
				if arg == "--" {
					args = args[i+1:]
					break
				}
			}
			a.format = "json"
			probe := NewRoot(strings.NewReader(""), io.Discard, io.Discard)
			target, rest, err := probe.Find(args)
			if err != nil || target == probe || !target.Runnable() {
				return fmt.Errorf("arguments do not name a runnable clarity command")
			}
			if err := target.ParseFlags(rest); err != nil {
				return err
			}
			res := resolution{Command: target.CommandPath(), Effect: "read", Flags: map[string]string{}, Args: target.Flags().Args()}
			ann := target.Annotations
			switch {
			case ann["mutates"] != "":
				res.Effect, res.Conditional = "remote_write", ann["mutates"] == "conditional"
			case ann["writes-local"] != "" || ann["self-update"] != "":
				res.Effect, res.Conditional = "local_write", ann["writes-local"] == "conditional"
			}
			res.Interactive = ann["interactive"] == "true"
			target.Flags().Visit(func(f *pflag.Flag) { res.Flags[f.Name] = f.Value.String() })
			return a.print(res)
		}})
	return c
}
