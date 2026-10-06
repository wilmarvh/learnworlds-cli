package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/wilmarvh/learnworlds-cli/skills"
)

func ok(data any, summary string, next ...Crumb) error {
	render(os.Stdout, g.mode(), Envelope{OK: true, Data: data, Summary: summary, Breadcrumbs: next}, nil)
	return nil
}

// ---- auth ----

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Log in, inspect and log out"}
	cmd.AddCommand(newAuthLoginCmd(), newAuthStatusCmd(), newAuthTokenCmd(), newAuthLogoutCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var school, clientID string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store API credentials for a school and verify them",
		Long: `Store API credentials for a school and verify them by minting a token.

Create credentials in your school under Settings → Developers → API.
The client secret is read from stdin when piped, otherwise prompted for:

  op read "op://Vault/LearnWorlds/secret" | lw auth login --school myschool.learnworlds.com --client-id abc -P myschool`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return &Error{Code: "config", Msg: err.Error()}
			}
			name := resolveProfile(g.profile, cfg)
			p := cfg.Profiles[name]
			p.SchoolURL = normalizeSchool(firstNonEmpty(school, p.SchoolURL))
			p.ClientID = firstNonEmpty(clientID, p.ClientID)
			if p.SchoolURL == "" || p.ClientID == "" {
				return &Error{Code: "usage", Msg: "--school and --client-id are required for a new profile"}
			}
			secret, err := readSecret()
			if err != nil {
				return err
			}
			c := &Client{Profile: name, School: p.SchoolURL, ClientID: p.ClientID, secret: secret, HTTP: &http.Client{Timeout: 60 * time.Second}}
			tok, exp, err := c.mint()
			if err != nil {
				return err
			}
			cfg.Profiles[name] = p
			if cfg.Default == "" {
				cfg.Default = name
			}
			creds, err := loadCreds()
			if err != nil {
				return &Error{Code: "config", Msg: err.Error()}
			}
			creds[name] = Credential{ClientSecret: secret, AccessToken: tok, ExpiresAt: exp}
			if err := saveConfig(cfg); err != nil {
				return &Error{Code: "config", Msg: err.Error()}
			}
			if err := saveCreds(creds); err != nil {
				return &Error{Code: "config", Msg: err.Error()}
			}
			return ok(map[string]any{"profile": name, "school_url": p.SchoolURL, "client_id": p.ClientID, "expires_at": exp},
				fmt.Sprintf("Logged in to %s as profile %q", p.SchoolURL, name),
				Crumb{"check", "lw doctor"}, Crumb{"try", "lw courses list"})
		},
	}
	cmd.Flags().StringVar(&school, "school", "", "school URL, e.g. myschool.learnworlds.com")
	cmd.Flags().StringVar(&clientID, "client-id", "", "API client id")
	return cmd
}

func readSecret() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, nil
		}
		return "", &Error{Code: "usage", Msg: "empty client secret on stdin"}
	}
	fmt.Fprint(os.Stderr, "Client secret: ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if s := strings.TrimSpace(string(b)); s != "" {
		return s, nil
	}
	return "", &Error{Code: "usage", Msg: "empty client secret"}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active profile and token state (no network)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := g.client()
			if err != nil {
				return err
			}
			st := map[string]any{
				"profile": c.Profile, "school_url": c.School, "client_id": c.ClientID,
				"secret_stored": c.secret != "", "token_valid": c.token != "" && time.Now().Unix() < c.expiresAt-60,
				"config_dir": configDir(),
			}
			if c.expiresAt > 0 && !c.envToken {
				st["token_expires_at"] = c.expiresAt
			}
			if err := c.checkConfigured(); err != nil {
				return err
			}
			return ok(st, "Profile "+c.Profile+" → "+c.School)
		},
	}
}

func newAuthTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Print a valid access token for scripts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := g.client()
			if err != nil {
				return err
			}
			tok, err := c.Token(false)
			if err != nil {
				return err
			}
			if g.mode() == ModeJSON {
				return ok(map[string]any{"access_token": tok, "expires_at": c.expiresAt}, "")
			}
			fmt.Println(tok)
			return nil
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored secret and token for a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			name := resolveProfile(g.profile, cfg)
			creds, err := loadCreds()
			if err != nil {
				return err
			}
			delete(creds, name)
			if err := saveCreds(creds); err != nil {
				return err
			}
			return ok(map[string]any{"profile": name}, "Logged out of "+name)
		},
	}
}

// ---- profile ----

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage profiles (one per school)"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "list", Short: "List profiles", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				creds, _ := loadCreds()
				var rows []any
				for _, n := range sortedKeys(cfg.Profiles) {
					p := cfg.Profiles[n]
					rows = append(rows, map[string]any{"name": n, "default": n == cfg.Default, "school_url": p.SchoolURL, "client_id": p.ClientID, "logged_in": creds[n].ClientSecret != ""})
				}
				if rows == nil {
					rows = []any{}
				}
				render(os.Stdout, g.mode(), Envelope{OK: true, Data: rows, Summary: fmt.Sprintf("%d profiles", len(rows))}, []string{"name", "default", "school_url", "logged_in"})
				return nil
			},
		},
		&cobra.Command{
			Use: "use <name>", Short: "Set the default profile", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				if _, found := cfg.Profiles[args[0]]; !found {
					return &Error{Code: "not_found", Msg: "no profile " + args[0], Hint: "see: lw profile list"}
				}
				cfg.Default = args[0]
				if err := saveConfig(cfg); err != nil {
					return err
				}
				return ok(map[string]any{"default": args[0]}, "Default profile: "+args[0])
			},
		},
		&cobra.Command{
			Use: "remove <name>", Short: "Remove a profile and its credentials", Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				delete(cfg.Profiles, args[0])
				if cfg.Default == args[0] {
					cfg.Default = ""
				}
				creds, _ := loadCreds()
				delete(creds, args[0])
				if err := saveConfig(cfg); err != nil {
					return err
				}
				if err := saveCreds(creds); err != nil {
					return err
				}
				return ok(map[string]any{"removed": args[0]}, "Removed profile "+args[0])
			},
		},
	)
	return cmd
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- doctor ----

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check configuration, credentials and API access",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			type check struct {
				Name   string `json:"name"`
				OK     bool   `json:"ok"`
				Detail string `json:"detail"`
			}
			var checks []check
			add := func(name string, okv bool, detail string) { checks = append(checks, check{name, okv, detail}) }
			c, err := g.client()
			if err != nil {
				return err
			}
			add("version", true, Version)
			add("profile", c.School != "" && c.ClientID != "", fmt.Sprintf("%s → %s (client %s)", c.Profile, firstNonEmpty(c.School, "no school URL"), firstNonEmpty(c.ClientID, "none")))
			add("secret", c.secret != "" || c.envToken, map[bool]string{true: "present", false: "missing — run lw auth login"}[c.secret != "" || c.envToken])
			if c.School != "" && c.ClientID != "" {
				start := time.Now()
				_, err := c.Token(true)
				detail := fmt.Sprintf("minted in %s", time.Since(start).Round(time.Millisecond))
				if c.envToken {
					detail = "from LW_ACCESS_TOKEN"
				}
				add("token", err == nil, errOr(err, detail))
				if err == nil {
					_, err = c.Do(Request{Method: "GET", Path: "/v2/courses", Query: url.Values{"page": {"1"}}})
					add("api", err == nil, errOr(err, "GET /v2/courses ok"))
				}
			}
			passed := 0
			for _, ch := range checks {
				if ch.OK {
					passed++
				}
			}
			if g.mode() == ModeHuman {
				for _, ch := range checks {
					mark := "✓"
					if !ch.OK {
						mark = "✗"
					}
					fmt.Printf("%s %-8s %s\n", mark, ch.Name, ch.Detail)
				}
			} else {
				render(os.Stdout, g.mode(), Envelope{OK: passed == len(checks), Data: checks, Summary: fmt.Sprintf("%d/%d checks passed", passed, len(checks))}, nil)
			}
			if passed != len(checks) {
				os.Exit(1)
			}
			return nil
		},
	}
}

func errOr(err error, s string) string {
	if err != nil {
		return err.Error()
	}
	return s
}

// ---- api ----

func newAPICmd() *cobra.Command {
	var method string
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Make a raw API request",
		Long: `Make an authenticated request to any endpoint. The path is relative to
<school>/admin/api, e.g. /v2/users. Method defaults to GET, or POST when a body is given.

  lw api /v2/users/jane@example.com
  lw api /v2/users/jane@example.com/tags -X PUT -F tags='["vip"]' -f action=attach`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := g.client()
			if err != nil {
				return err
			}
			p := args[0]
			if i := strings.Index(p, "/admin/api"); i >= 0 {
				p = p[i+len("/admin/api"):]
			}
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			u, err := url.Parse(p)
			if err != nil {
				return &Error{Code: "usage", Msg: err.Error()}
			}
			body, err := buildBody(cmd, nil)
			if err != nil {
				return err
			}
			m := strings.ToUpper(method)
			if m == "" {
				m = map[bool]string{true: "POST", false: "GET"}[body != nil]
			}
			data, err := c.Do(Request{Method: m, Path: u.Path, Query: u.Query(), Body: body})
			if err != nil {
				return err
			}
			render(os.Stdout, g.mode(), Envelope{OK: true, Data: data}, nil)
			return nil
		},
	}
	cmd.Flags().StringVarP(&method, "method", "X", "", "HTTP method")
	addBodyFlags(cmd)
	return cmd
}

// ---- commands ----

func newCommandsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List every command with its API endpoint (use --json for agents)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var rows []any
			var walk func(c *cobra.Command)
			walk = func(c *cobra.Command) {
				if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
					return
				}
				if c.Runnable() && c.HasParent() {
					row := map[string]any{"command": c.CommandPath(), "usage": c.UseLine(), "short": c.Short}
					if m := c.Annotations["method"]; m != "" {
						row["endpoint"] = m + " " + c.Annotations["path"]
					}
					var flags []string
					c.LocalFlags().VisitAll(func(f *pflag.Flag) { flags = append(flags, "--"+f.Name) })
					row["flags"] = flags
					rows = append(rows, row)
				}
				for _, s := range c.Commands() {
					walk(s)
				}
			}
			walk(cmd.Root())
			render(os.Stdout, g.mode(), Envelope{OK: true, Data: rows, Summary: fmt.Sprintf("%d commands", len(rows))}, []string{"command", "endpoint", "short"})
			return nil
		},
	}
}

// ---- skill ----

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the agent skill (SKILL.md)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Print(skills.LearnWorlds)
			return nil
		},
	}
	var dir string
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the skill for Claude Code (~/.claude/skills/learnworlds)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir == "" {
				home, _ := os.UserHomeDir()
				dir = filepath.Join(home, ".claude", "skills", "learnworlds")
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			p := filepath.Join(dir, "SKILL.md")
			if err := os.WriteFile(p, []byte(skills.LearnWorlds), 0o644); err != nil {
				return err
			}
			return ok(map[string]any{"path": p}, "Installed skill to "+p)
		},
	}
	install.Flags().StringVar(&dir, "dir", "", "target directory (default ~/.claude/skills/learnworlds)")
	cmd.AddCommand(install)
	return cmd
}
