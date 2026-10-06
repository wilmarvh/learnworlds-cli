package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var Version = "dev"

type globals struct {
	profile string
	json    bool
	quiet   bool
	dryRun  bool
	verbose bool
}

var g globals

func (g *globals) mode() Mode { return resolveMode(g.json, g.quiet) }

func (g *globals) client() (*Client, error) {
	c, err := NewClient(g.profile)
	if err != nil {
		return nil, err
	}
	c.DryRun, c.Verbose = g.dryRun, g.verbose
	return c, nil
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	root := NewRoot()
	err := root.Execute()
	if err == nil {
		return 0
	}
	var e *Error
	if !errors.As(err, &e) {
		// cobra arg/flag errors are usage errors
		e = &Error{Code: "usage", Msg: err.Error(), Hint: "see: " + root.Name() + " --help"}
	}
	renderError(g.mode(), e)
	return e.ExitCode()
}

func NewRoot() *cobra.Command {
	g = globals{}
	root := &cobra.Command{
		Use:           "lw",
		Short:         "LearnWorlds from the command line",
		Long:          "lw talks to the LearnWorlds API (learnworlds.dev).\n\nOutput is a table in a terminal and a JSON envelope when piped.\nUse --json for the envelope, --quiet for the bare data.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pf := root.PersistentFlags()
	pf.StringVarP(&g.profile, "profile", "P", "", "profile (school) to use; env LW_PROFILE")
	pf.BoolVar(&g.json, "json", false, "output JSON envelope {ok, data, summary, breadcrumbs}")
	pf.BoolVarP(&g.quiet, "quiet", "q", false, "output bare JSON data only")
	pf.BoolVar(&g.dryRun, "dry-run", false, "print the request instead of sending it")
	pf.BoolVarP(&g.verbose, "verbose", "v", false, "log HTTP requests to stderr")
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return &Error{Code: "usage", Msg: err.Error(), Hint: "see: " + c.CommandPath() + " --help"}
	})

	groups := map[string]*cobra.Command{}
	var group func(path string) *cobra.Command
	group = func(path string) *cobra.Command {
		if c, ok := groups[path]; ok {
			return c
		}
		parent := root
		name := path
		if i := strings.LastIndex(path, " "); i >= 0 {
			parent, name = group(path[:i]), path[i+1:]
		}
		c := &cobra.Command{Use: name, Short: groupShort[path], GroupID: ""}
		if parent == root {
			c.GroupID = "api"
		}
		parent.AddCommand(c)
		groups[path] = c
		return c
	}
	root.AddGroup(&cobra.Group{ID: "api", Title: "Resources:"}, &cobra.Group{ID: "cli", Title: "CLI:"})
	for i := range endpoints {
		group(endpoints[i].Group).AddCommand(endpointCmd(&endpoints[i]))
	}
	for _, c := range []*cobra.Command{newAuthCmd(), newProfileCmd(), newDoctorCmd(), newAPICmd(), newCommandsCmd(), newSkillCmd()} {
		c.GroupID = "cli"
		root.AddCommand(c)
	}
	root.SetCompletionCommandGroupID("cli")
	root.SetHelpCommandGroupID("cli")
	return root
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

func kebab(s string) string {
	s = regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(s, "$1-$2")
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}

func endpointCmd(e *ep) *cobra.Command {
	nPath := len(pathParam.FindAllString(e.Path, -1))
	nArgs := nPath + len(e.ArgBody)
	use := e.Name
	if e.Args != "" {
		use += " " + e.Args
	}
	mutating := e.Method != "GET"
	cmd := &cobra.Command{
		Use:         use,
		Short:       e.Short,
		Long:        e.Short + "\n\nAPI: " + e.Method + " " + e.Path,
		Args:        cobra.ExactArgs(nArgs),
		Annotations: map[string]string{"method": e.Method, "path": e.Path},
	}
	f := cmd.Flags()
	for _, q := range e.Query {
		name, kind, _ := strings.Cut(q, ":")
		switch kind {
		case "bool":
			f.Bool(kebab(name), false, name)
		case "time":
			f.String(kebab(name), "", name+" (YYYY-MM-DD, RFC3339 or unix seconds)")
		default:
			f.String(kebab(name), "", name)
		}
	}
	f.StringArrayP("query", "Q", nil, "extra query param key=value (e.g. cf_city=Athens), repeatable")
	if e.Paged {
		f.Int("page", 0, "page number")
		f.Bool("all", false, "fetch every page")
	}
	if e.Limit {
		f.Int("limit", 0, "items per page")
	}
	for _, b := range e.Body {
		switch b.Kind {
		case "bool":
			f.Bool(b.Flag, b.Def == "true", b.Usage)
		default:
			f.String(b.Flag, strings.Trim(b.Def, "[]"), b.Usage)
		}
	}
	if e.Group == "users" && e.Name == "tag" {
		f.String("add", "", "comma-separated tags to attach")
		f.String("remove", "", "comma-separated tags to detach")
	}
	if mutating {
		addBodyFlags(cmd)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		c, err := g.client()
		if err != nil {
			return err
		}
		path := e.Path
		for i, m := range pathParam.FindAllString(e.Path, -1) {
			path = strings.Replace(path, m, escapeSegment(args[i]), 1)
		}
		q, err := buildQuery(cmd, e)
		if err != nil {
			return err
		}
		req := Request{Method: e.Method, Path: path, Query: q}
		if mutating {
			body, err := buildBody(cmd, e.Body)
			if err != nil {
				return err
			}
			if len(e.ArgBody) > 0 || e.Name == "tag" {
				m, _ := body.(map[string]any)
				if m == nil {
					m = map[string]any{}
				}
				for i, k := range e.ArgBody {
					m[k] = args[nPath+i]
				}
				if e.Name == "tag" {
					if err := tagBody(cmd, m); err != nil {
						return err
					}
				}
				body = m
			}
			req.Body = body
		}
		var data any
		var meta map[string]any
		if mutating {
			data, err = c.Do(req)
		} else {
			all, _ := f.GetBool("all")
			data, meta, err = c.Paged(req, all && e.Paged)
		}
		if err != nil {
			return err
		}
		env := Envelope{OK: true, Data: data, Meta: meta, Summary: summarize(e, data, meta, c.DryRun)}
		if !c.DryRun {
			env.Breadcrumbs = crumbs(cmd, e, args, meta)
		}
		render(os.Stdout, g.mode(), env, e.Cols)
		return nil
	}
	return cmd
}

// escapeSegment fully encodes ids; emails must arrive as user%40x.com, and "+" must not become a space.
func escapeSegment(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func addBodyFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.String("data", "", "JSON request body, @file, or - for stdin")
	f.StringArrayP("raw-field", "f", nil, "body field key=value as string, repeatable")
	f.StringArrayP("field", "F", nil, "body field key=value parsed as JSON (numbers, true/false, arrays), repeatable")
}

func buildQuery(cmd *cobra.Command, e *ep) (url.Values, error) {
	f := cmd.Flags()
	q := url.Values{}
	for _, spec := range e.Query {
		name, kind, _ := strings.Cut(spec, ":")
		fl := kebab(name)
		if !f.Changed(fl) {
			continue
		}
		v := f.Lookup(fl).Value.String()
		if kind == "time" {
			t, err := parseTime(v)
			if err != nil {
				return nil, &Error{Code: "usage", Msg: fmt.Sprintf("--%s: %v", fl, err)}
			}
			v = strconv.FormatInt(t, 10)
		}
		q.Set(name, v)
	}
	if e.Paged {
		if p, _ := f.GetInt("page"); p > 0 {
			q.Set("page", strconv.Itoa(p))
		}
	}
	if e.Limit {
		if l, _ := f.GetInt("limit"); l > 0 {
			q.Set("items_per_page", strconv.Itoa(l))
		}
	}
	extra, _ := f.GetStringArray("query")
	for _, kv := range extra {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, &Error{Code: "usage", Msg: "--query wants key=value, got " + kv}
		}
		q.Add(k, v)
	}
	return q, nil
}

func parseTime(s string) (int64, error) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("can't parse time %q", s)
}

// buildBody merges --data, typed flags, then -f/-F fields (later wins).
func buildBody(cmd *cobra.Command, fields []bf) (any, error) {
	f := cmd.Flags()
	var body any
	m := map[string]any{}
	if d, _ := f.GetString("data"); d != "" {
		raw, err := readArg(d)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, &Error{Code: "usage", Msg: "--data is not valid JSON: " + err.Error()}
		}
		if bm, ok := body.(map[string]any); ok {
			m = bm
		}
	}
	for _, b := range fields {
		if !f.Changed(b.Flag) && b.Def == "" {
			continue
		}
		v := f.Lookup(b.Flag).Value.String()
		switch b.Kind {
		case "bool":
			m[b.Key] = v == "true"
		case "num", "int":
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, &Error{Code: "usage", Msg: fmt.Sprintf("--%s wants a number, got %q", b.Flag, v)}
			}
			m[b.Key] = n
		case "list":
			m[b.Key] = splitList(v)
		default:
			m[b.Key] = v
		}
	}
	raw, _ := f.GetStringArray("raw-field")
	typed, _ := f.GetStringArray("field")
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, &Error{Code: "usage", Msg: "-f wants key=value, got " + kv}
		}
		m[k] = v
	}
	for _, kv := range typed {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, &Error{Code: "usage", Msg: "-F wants key=value, got " + kv}
		}
		var parsed any
		if strings.HasPrefix(v, "@") {
			b, err := readArg(v)
			if err != nil {
				return nil, err
			}
			v = string(b)
		}
		if json.Unmarshal([]byte(v), &parsed) != nil {
			parsed = v
		}
		m[k] = parsed
	}
	if body != nil && len(m) == 0 {
		return body, nil
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

func splitList(v string) []any {
	out := []any{}
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// readArg reads "-" from stdin, "@path" from a file, else returns the literal.
func readArg(s string) ([]byte, error) {
	switch {
	case s == "-":
		return io.ReadAll(os.Stdin)
	case strings.HasPrefix(s, "@"):
		b, err := os.ReadFile(s[1:])
		if err != nil {
			return nil, &Error{Code: "usage", Msg: err.Error()}
		}
		return b, nil
	}
	return []byte(s), nil
}

func tagBody(cmd *cobra.Command, m map[string]any) error {
	add, _ := cmd.Flags().GetString("add")
	rm, _ := cmd.Flags().GetString("remove")
	switch {
	case add != "" && rm != "":
		return &Error{Code: "usage", Msg: "use --add or --remove, not both"}
	case add != "":
		m["tags"], m["action"] = splitList(add), "attach"
	case rm != "":
		m["tags"], m["action"] = splitList(rm), "detach"
	case m["action"] == nil:
		return &Error{Code: "usage", Msg: "pass --add <tags> or --remove <tags>"}
	}
	return nil
}

func summarize(e *ep, data any, meta map[string]any, dry bool) string {
	if dry {
		return "dry run: nothing sent"
	}
	if e.Method != "GET" {
		return e.Short + ": done"
	}
	list, ok := data.([]any)
	if !ok {
		return ""
	}
	noun := e.Group
	if e.Name != "list" {
		noun = e.Name
	}
	s := fmt.Sprintf("%d %s", len(list), noun)
	if meta != nil {
		if all, _ := meta["all"].(bool); all {
			return s + " (all pages)"
		}
		p, _ := meta["page"].(float64)
		tp, _ := meta["totalPages"].(float64)
		ti, _ := meta["totalItems"].(float64)
		if tp > 0 {
			s += fmt.Sprintf(" (page %d/%d, %d total)", int(p), int(tp), int(ti))
		}
	}
	return s
}

// crumbs fills <placeholders> from the args the user gave and adds a next-page hint.
func crumbs(cmd *cobra.Command, e *ep, args []string, meta map[string]any) []Crumb {
	names := strings.Fields(e.Args)
	var out []Crumb
	for _, c := range e.Next {
		s := c.Cmd
		for i, n := range names {
			if i < len(args) {
				s = strings.ReplaceAll(s, n, args[i])
			}
		}
		out = append(out, Crumb{c.Action, s})
	}
	if meta != nil {
		p, _ := meta["page"].(float64)
		tp, _ := meta["totalPages"].(float64)
		if p > 0 && p < tp {
			base := strings.TrimSpace(cmd.CommandPath() + " " + strings.Join(args, " "))
			out = append(out, Crumb{"next page", fmt.Sprintf("%s --page %d", base, int(p)+1)}, Crumb{"all pages", base + " --all"})
		}
	}
	return out
}
