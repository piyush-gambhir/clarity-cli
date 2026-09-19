package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/piyush-gambhir/clarity-cli/cli-go/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func (a *app) auth() *cobra.Command {
	c := &cobra.Command{Use: "auth", Short: "Manage project tokens and named profiles"}
	c.AddCommand(a.login(), a.status(), a.listProfiles(), a.useProfile(), a.logout())
	return c
}

func (a *app) login() *cobra.Command {
	var stdin, verify bool
	c := &cobra.Command{Use: "login", Short: "Save a project token (hidden prompt, environment, or stdin)", Args: cobra.NoArgs,
		Long:        "Save a token generated in Clarity → Settings → Data Export.\nThis saves locally without making an API request unless --verify is set.\n--verify consumes one Export API request; that API allows 10 per project per day.\nAn existing profile's token is replaced, and the profile becomes current.",
		Example:     "  clarity auth login --profile website\n  clarity auth login --profile website --token-stdin < token.txt\n  CLARITY_API_TOKEN=... clarity auth login --no-input --profile website",
		Annotations: map[string]string{"writes-local": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := a.profile
			if name == "" {
				name = os.Getenv("CLARITY_PROFILE")
			}
			if name == "" {
				name = "default"
			}
			if strings.TrimSpace(name) != name || name == "" || strings.ContainsAny(name, "\r\n\t") {
				return fmt.Errorf("profile name must be nonempty and contain no surrounding whitespace or control characters")
			}
			token := a.token
			if stdin {
				if token != "" {
					return fmt.Errorf("--token and --token-stdin are mutually exclusive")
				}
				b, err := io.ReadAll(io.LimitReader(a.in, 65537))
				if err != nil {
					return err
				}
				if len(b) > 65536 {
					return fmt.Errorf("token input exceeds 64 KiB")
				}
				token = string(b)
			} else if token == "" {
				token = os.Getenv("CLARITY_API_TOKEN")
			}
			if token == "" {
				if a.noInput {
					return fmt.Errorf("provide CLARITY_API_TOKEN or --token-stdin with --no-input")
				}
				f, ok := a.in.(*os.File)
				if !ok || !term.IsTerminal(int(f.Fd())) {
					return fmt.Errorf("no terminal available; use --token-stdin or CLARITY_API_TOKEN")
				}
				fmt.Fprint(a.errOut, "Clarity API token: ")
				b, err := term.ReadPassword(int(f.Fd()))
				fmt.Fprintln(a.errOut)
				if err != nil {
					return err
				}
				token = string(b)
			}
			token = strings.TrimSpace(token)
			if token == "" || strings.ContainsAny(token, "\r\n\t ") {
				return fmt.Errorf("token must be nonempty and contain no whitespace")
			}
			if verify {
				a.info("Verifying token (uses one Export API request)...")
				cl := a.newClient(token, a.timeout)
				if _, err := cl.Export(cmd.Context(), 1, nil); err != nil {
					return err
				}
			}
			path, err := config.Path()
			if err != nil {
				return err
			}
			if err := config.Update(cmd.Context(), path, func(cfg *config.Config) error {
				cfg.Profiles[name] = config.Profile{Token: token}
				cfg.CurrentProfile = name
				return nil
			}); err != nil {
				return err
			}
			return a.print(map[string]any{"profile": name, "saved": true, "verified": verify})
		},
	}
	c.Flags().BoolVar(&stdin, "token-stdin", false, "Read token from stdin")
	c.Flags().BoolVar(&verify, "verify", false, "Verify remotely before saving (uses one export request)")
	return c
}

func (a *app) status() *cobra.Command {
	var verify bool
	c := &cobra.Command{Use: "status", Short: "Show credential source without revealing the token", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, path, err := a.load()
		if err != nil {
			return err
		}
		token, source, err := cfg.Resolve(a.profile, a.token)
		if err != nil {
			return err
		}
		if verify {
			a.info("Verifying token (uses one Export API request)...")
			if _, err := a.newClient(token, a.timeout).Export(cmd.Context(), 1, nil); err != nil {
				return err
			}
		}
		return a.print(map[string]any{"configured": true, "source": source, "config_file": path, "verified": verify})
	}}
	c.Flags().BoolVar(&verify, "verify", false, "Check remote access (uses one export request)")
	return c
}

func (a *app) listProfiles() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"list-profiles"}, Short: "List saved project profiles, without tokens", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _, err := a.load()
		if err != nil {
			return err
		}
		names := make([]string, 0, len(cfg.Profiles))
		for name := range cfg.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		rows := make([]map[string]any, 0, len(names))
		for _, name := range names {
			rows = append(rows, map[string]any{"profile": name, "current": name == cfg.CurrentProfile, "configured": cfg.Profiles[name].Token != ""})
		}
		return a.print(rows)
	}}
}

func (a *app) useProfile() *cobra.Command {
	return &cobra.Command{Use: "use NAME", Aliases: []string{"use-profile"}, Short: "Set the default project profile", Args: cobra.ExactArgs(1), Annotations: map[string]string{"writes-local": "true"}, RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		if err := config.Update(cmd.Context(), path, func(c *config.Config) error {
			if _, ok := c.Profiles[args[0]]; !ok {
				return fmt.Errorf("profile %q not found", args[0])
			}
			c.CurrentProfile = args[0]
			return nil
		}); err != nil {
			return err
		}
		return a.print(map[string]string{"current_profile": args[0]})
	}}
}

func (a *app) logout() *cobra.Command {
	return &cobra.Command{Use: "logout", Short: "Remove a saved token locally; does not revoke it at Microsoft", Args: cobra.NoArgs, Annotations: map[string]string{"writes-local": "true"}, RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		name := a.profile
		if name == "" {
			name = os.Getenv("CLARITY_PROFILE")
		}
		if err := config.Update(cmd.Context(), path, func(c *config.Config) error {
			if name == "" {
				name = c.CurrentProfile
			}
			if _, ok := c.Profiles[name]; !ok {
				return fmt.Errorf("profile %q not found", name)
			}
			delete(c.Profiles, name)
			if c.CurrentProfile == name {
				c.CurrentProfile = ""
			}
			return nil
		}); err != nil {
			return err
		}
		return a.print(map[string]any{"profile": name, "removed": true})
	}}
}

func (a *app) configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Inspect configuration and select profiles"}
	list := a.listProfiles()
	list.Use = "list-profiles"
	list.Aliases = nil
	use := a.useProfile()
	use.Use = "use-profile NAME"
	use.Aliases = nil
	show := a.status()
	show.Use = "show"
	c.AddCommand(list, use, show)
	return c
}
