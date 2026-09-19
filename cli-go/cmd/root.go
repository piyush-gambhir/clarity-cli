package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/build"
	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/client"
	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/config"
	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/output"
	"github.com/spf13/cobra"
)

type app struct {
	format, profile, token            string
	timeout                           time.Duration
	noInput, quiet, verbose, readOnly bool
	in                                io.Reader
	out, errOut                       io.Writer
	newClient                         func(string, time.Duration) *client.Client
}

func envBool(name string) bool { s := os.Getenv(name); return s == "1" || strings.EqualFold(s, "true") }

func NewRoot(in io.Reader, out, errOut io.Writer) *cobra.Command {
	return newRoot(&app{in: in, out: out, errOut: errOut, newClient: client.New})
}

func newRoot(a *app) *cobra.Command {
	root := &cobra.Command{
		Use: "clarity", Short: "Microsoft Clarity analytics and session recordings from your terminal",
		Long:         "Read Microsoft Clarity data using project API tokens.\nExport dashboard metrics, query analytics, find session recordings, and search documentation.\nAll remote operations are read-only. Profiles store one token per project.",
		SilenceUsage: true, SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !output.Valid(a.format) {
				return fmt.Errorf("unsupported output %q; use table, json, or yaml", a.format)
			}
			if a.timeout <= 0 {
				return fmt.Errorf("--timeout must be positive")
			}
			if a.readOnly && cmd.Annotations["writes-local"] == "true" {
				return fmt.Errorf("%s changes local state and is blocked by --read-only", cmd.CommandPath())
			}
			return nil
		},
	}
	root.SetIn(a.in)
	root.SetOut(a.out)
	root.SetErr(a.errOut)
	f := root.PersistentFlags()
	f.StringVarP(&a.format, "output", "o", "table", "Output format: table, json, yaml")
	f.StringVar(&a.profile, "profile", "", "Named project profile (or CLARITY_PROFILE)")
	f.StringVar(&a.token, "token", "", "API token override (prefer CLARITY_API_TOKEN or auth login)")
	f.DurationVar(&a.timeout, "timeout", 30*time.Second, "HTTP request timeout")
	f.BoolVar(&a.noInput, "no-input", envBool("CLARITY_NO_INPUT"), "Disable interactive prompts")
	f.BoolVarP(&a.quiet, "quiet", "q", envBool("CLARITY_QUIET"), "Suppress informational stderr output")
	f.BoolVarP(&a.verbose, "verbose", "v", envBool("CLARITY_VERBOSE"), "Log request method, URL, and status to stderr (no tokens/bodies)")
	f.BoolVar(&a.readOnly, "read-only", envBool("CLARITY_READ_ONLY"), "Also block local credential changes and self-update")
	root.AddCommand(a.auth(), a.export(), a.analytics(), a.recordings(), a.docs(), a.update())
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print build information", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.print(map[string]string{"version": build.Version, "commit": build.Commit, "date": build.Date})
	}})
	login := a.login()
	login.Use = "login"
	root.AddCommand(login)
	status := a.status()
	status.Use = "status"
	root.AddCommand(status)
	root.AddCommand(a.configCmd())
	root.AddCommand(&cobra.Command{Use: "completion [bash|zsh|fish|powershell]", Short: "Generate shell completion script", Args: cobra.ExactArgs(1), ValidArgs: []string{"bash", "zsh", "fish", "powershell"}, RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(a.out)
		case "zsh":
			return root.GenZshCompletion(a.out)
		case "fish":
			return root.GenFishCompletion(a.out, true)
		case "powershell":
			return root.GenPowerShellCompletionWithDesc(a.out)
		default:
			return fmt.Errorf("unsupported shell %q", args[0])
		}
	}})
	root.CompletionOptions.DisableDefaultCmd = true
	return root
}

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	a := &app{in: in, out: out, errOut: errOut, newClient: client.New}
	root := newRoot(a)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		message := err.Error()
		for _, secret := range []string{a.token, os.Getenv("CLARITY_API_TOKEN")} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[REDACTED]")
			}
		}
		payload := map[string]any{"error": message}
		var apiError *client.APIError
		if errors.As(err, &apiError) {
			payload["status"] = apiError.Status
			if apiError.RetryAfter != "" {
				payload["retry_after"] = apiError.RetryAfter
			}
		}
		if a.format == "json" || a.format == "yaml" {
			_ = output.Print(errOut, a.format, payload)
		} else {
			fmt.Fprintln(errOut, "Error:", message)
		}
		return 1
	}
	return 0
}

func (a *app) load() (*config.Config, string, error) {
	p, err := config.Path()
	if err != nil {
		return nil, "", err
	}
	c, err := config.Load(p)
	return c, p, err
}

func (a *app) client() (*client.Client, error) {
	c, _, err := a.load()
	if err != nil {
		return nil, err
	}
	token, _, err := c.Resolve(a.profile, a.token)
	if err != nil {
		return nil, err
	}
	cl := a.newClient(token, a.timeout)
	if a.verbose {
		cl.Log = a.errOut
	}
	return cl, nil
}

func (a *app) print(data any) error { return output.Print(a.out, a.format, data) }
func (a *app) info(s string, args ...any) {
	if !a.quiet {
		fmt.Fprintf(a.errOut, s+"\n", args...)
	}
}
