package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Error is the CLI's structured failure. Code is stable for scripts and agents.
type Error struct {
	Code      string `json:"code"`
	Msg       string `json:"error"`
	Hint      string `json:"hint,omitempty"`
	Retryable bool   `json:"retryable"`
	Status    int    `json:"status,omitempty"`
}

func (e *Error) Error() string { return e.Msg }

// ExitCode maps error codes to process exit codes.
func (e *Error) ExitCode() int {
	switch e.Code {
	case "usage":
		return 2
	case "auth", "forbidden", "config":
		return 3
	case "not_found":
		return 4
	case "rate_limited":
		return 5
	}
	return 1
}

type Crumb struct {
	Action string `json:"action"`
	Cmd    string `json:"cmd"`
}

type Envelope struct {
	OK          bool           `json:"ok"`
	Data        any            `json:"data"`
	Summary     string         `json:"summary,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	Breadcrumbs []Crumb        `json:"breadcrumbs,omitempty"`
}

type Mode int

const (
	ModeHuman Mode = iota
	ModeJSON
	ModeQuiet
)

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// resolveMode: --quiet > --json > TTY human > JSON when piped.
func resolveMode(jsonFlag, quiet bool) Mode {
	switch {
	case quiet:
		return ModeQuiet
	case jsonFlag || !isTTY(os.Stdout):
		return ModeJSON
	}
	return ModeHuman
}

func writeJSONTo(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func render(w io.Writer, mode Mode, env Envelope, cols []string) {
	switch mode {
	case ModeQuiet:
		writeJSONTo(w, env.Data)
		return
	case ModeJSON:
		writeJSONTo(w, env)
		return
	}
	switch d := env.Data.(type) {
	case []any:
		renderTable(w, d, cols)
	case map[string]any:
		if dry, _ := d["dry_run"].(bool); dry {
			writeJSONTo(w, d)
			break
		}
		renderObject(w, d)
	case nil:
	default:
		fmt.Fprintln(w, human("", d))
	}
	if env.Summary != "" {
		fmt.Fprintln(w, dim(env.Summary))
	}
	for _, c := range env.Breadcrumbs {
		fmt.Fprintln(w, dim(fmt.Sprintf("  %-10s %s", c.Action, c.Cmd)))
	}
}

func renderError(mode Mode, e *Error) {
	if mode != ModeHuman {
		writeJSONTo(os.Stdout, struct {
			OK bool `json:"ok"`
			*Error
		}{false, e})
		return
	}
	fmt.Fprintln(os.Stderr, "Error: "+e.Msg)
	if e.Hint != "" {
		fmt.Fprintln(os.Stderr, dim("Hint: "+e.Hint))
	}
}

var preferredCols = []string{"id", "title", "name", "email", "username", "code", "type", "status", "role", "access", "final_price", "price", "created"}

func autoCols(rows []any) []string {
	first, _ := rows[0].(map[string]any)
	var cols []string
	for _, k := range preferredCols {
		if v, ok := first[k]; ok && isScalar(v) {
			cols = append(cols, k)
		}
		if len(cols) == 5 {
			return cols
		}
	}
	if len(cols) > 0 {
		return cols
	}
	keys := make([]string, 0, len(first))
	for k, v := range first {
		if isScalar(v) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > 5 {
		keys = keys[:5]
	}
	return keys
}

func renderTable(w io.Writer, rows []any, cols []string) {
	if len(rows) == 0 {
		fmt.Fprintln(w, dim("(none)"))
		return
	}
	if _, ok := rows[0].(map[string]any); !ok {
		for _, r := range rows {
			fmt.Fprintln(w, human("", r))
		}
		return
	}
	if len(cols) == 0 {
		cols = autoCols(rows)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.ToUpper(strings.Join(cols, "\t")))
	for _, r := range rows {
		m, _ := r.(map[string]any)
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = truncate(human(c, dig(m, c)), 48)
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	tw.Flush()
}

func renderObject(w io.Writer, m map[string]any) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return rank(keys[i]) < rank(keys[j]) || rank(keys[i]) == rank(keys[j]) && keys[i] < keys[j]
	})
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, k := range keys {
		fmt.Fprintf(tw, "%s\t%s\n", bold(k), truncate(human(k, m[k]), 100))
	}
	tw.Flush()
}

func rank(k string) int {
	for i, p := range preferredCols {
		if p == k {
			return i
		}
	}
	return len(preferredCols)
}

// dig resolves dotted paths like "course.title".
func dig(m map[string]any, path string) any {
	var v any = m
	for _, p := range strings.Split(path, ".") {
		mm, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = mm[p]
	}
	return v
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	}
	return true
}

var timeKeys = []string{"created", "modified", "expires", "_at", "date", "issued", "Timestamp", "registration"}

// human formats a value for terminal display; unix timestamps in date-ish fields become dates.
func human(key string, v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case string:
		return x
	case float64:
		if x > 1e9 && x < 1e10 && isTimeKey(key) {
			return time.Unix(int64(x), 0).Local().Format("2006-01-02 15:04")
		}
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case bool:
		return fmt.Sprintf("%t", x)
	case []any:
		if len(x) == 0 {
			return "[]"
		}
		allScalar := true
		parts := make([]string, len(x))
		for i, e := range x {
			allScalar = allScalar && isScalar(e)
			parts[i] = human("", e)
		}
		if allScalar {
			return strings.Join(parts, ", ")
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func isTimeKey(k string) bool {
	for _, t := range timeKeys {
		if strings.Contains(k, t) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

var color = os.Getenv("NO_COLOR") == "" && isTTY(os.Stdout)

func bold(s string) string {
	if !color {
		return s
	}
	return "\x1b[1m" + s + "\x1b[0m"
}

func dim(s string) string {
	if !color {
		return s
	}
	return "\x1b[2m" + s + "\x1b[0m"
}
