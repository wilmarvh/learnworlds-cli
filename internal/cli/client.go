package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Profile is one LearnWorlds school the CLI can talk to.
type Profile struct {
	SchoolURL string `json:"school_url"`
	ClientID  string `json:"client_id"`
}

type Config struct {
	Default  string             `json:"default,omitempty"`
	Profiles map[string]Profile `json:"profiles"`
}

// Credential holds the secret and the cached access token for a profile.
// ponytail: 0600 file, move to OS keyring if this ever runs on shared machines.
type Credential struct {
	ClientSecret string `json:"client_secret"`
	AccessToken  string `json:"access_token,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
}

func configDir() string {
	if d := os.Getenv("LW_CONFIG_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "learnworlds")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "learnworlds")
}

func readJSON(name string, v any) error {
	b, err := os.ReadFile(filepath.Join(configDir(), name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(name string, v any, perm os.FileMode) error {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	p := filepath.Join(configDir(), name)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), perm); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func loadConfig() (*Config, error) {
	c := &Config{Profiles: map[string]Profile{}}
	if err := readJSON("config.json", c); err != nil {
		return nil, err
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	return c, nil
}

func saveConfig(c *Config) error { return writeJSON("config.json", c, 0o600) }

func loadCreds() (map[string]Credential, error) {
	m := map[string]Credential{}
	return m, readJSON("credentials.json", &m)
}

func saveCreds(m map[string]Credential) error { return writeJSON("credentials.json", m, 0o600) }

func normalizeSchool(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s != "" && !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return s
}

func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

var sleepFn = time.Sleep // swapped in tests

// Client is an authenticated LearnWorlds API client.
type Client struct {
	Profile   string
	School    string
	ClientID  string
	secret    string
	token     string
	expiresAt int64
	envToken  bool // token came from LW_ACCESS_TOKEN; never refresh or persist
	DryRun    bool
	Verbose   bool
	HTTP      *http.Client
	sleep     func(time.Duration)
}

// resolveProfile picks the profile name: flag > LW_PROFILE > config default > only profile.
func resolveProfile(flag string, cfg *Config) string {
	if flag != "" {
		return flag
	}
	if p := os.Getenv("LW_PROFILE"); p != "" {
		return p
	}
	if cfg.Default != "" {
		return cfg.Default
	}
	if len(cfg.Profiles) == 1 {
		for k := range cfg.Profiles {
			return k
		}
	}
	return "default"
}

// NewClient resolves settings: env vars override the stored profile.
func NewClient(profileFlag string) (*Client, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, &Error{Code: "config", Msg: "reading config: " + err.Error()}
	}
	creds, err := loadCreds()
	if err != nil {
		return nil, &Error{Code: "config", Msg: "reading credentials: " + err.Error()}
	}
	name := resolveProfile(profileFlag, cfg)
	p := cfg.Profiles[name]
	cr := creds[name]
	c := &Client{
		Profile:   name,
		School:    normalizeSchool(firstNonEmpty(envFirst("LW_SCHOOL_URL", "LEARNWORLDS_SCHOOL_URL"), p.SchoolURL)),
		ClientID:  firstNonEmpty(envFirst("LW_CLIENT_ID", "LEARNWORLDS_CLIENT_ID"), p.ClientID),
		secret:    firstNonEmpty(envFirst("LW_CLIENT_SECRET", "LEARNWORLDS_CLIENT_SECRET"), cr.ClientSecret),
		token:     cr.AccessToken,
		expiresAt: cr.ExpiresAt,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		sleep:     sleepFn,
	}
	if t := os.Getenv("LW_ACCESS_TOKEN"); t != "" {
		c.token, c.expiresAt, c.envToken = t, math.MaxInt64, true
	}
	return c, nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func (c *Client) checkConfigured() error {
	if c.School == "" || c.ClientID == "" {
		return &Error{Code: "auth", Msg: fmt.Sprintf("profile %q is not configured", c.Profile),
			Hint: "run: lw auth login --school <school-url> --client-id <id>"}
	}
	return nil
}

// Token returns a valid access token, minting a new one when needed.
func (c *Client) Token(force bool) (string, error) {
	if err := c.checkConfigured(); err != nil {
		return "", err
	}
	if !force && c.token != "" && time.Now().Unix() < c.expiresAt-60 {
		return c.token, nil
	}
	if c.envToken {
		return c.token, nil
	}
	if c.secret == "" {
		return "", &Error{Code: "auth", Msg: "no client secret for profile " + c.Profile,
			Hint: "run: lw auth login, or set LW_CLIENT_SECRET"}
	}
	tok, exp, err := c.mint()
	if err != nil {
		return "", err
	}
	c.token, c.expiresAt = tok, exp
	// Cache only when the secret is stored (not env-only setups).
	if creds, err := loadCreds(); err == nil {
		if cr, ok := creds[c.Profile]; ok && cr.ClientSecret == c.secret {
			cr.AccessToken, cr.ExpiresAt = tok, exp
			creds[c.Profile] = cr
			_ = saveCreds(creds)
		}
	}
	return tok, nil
}

// mint calls the client_credentials grant. LearnWorlds wants the JSON payload
// inside a form field named "data".
func (c *Client) mint() (string, int64, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id": c.ClientID, "client_secret": c.secret, "grant_type": "client_credentials",
	})
	form := url.Values{"data": {string(payload)}}
	req, _ := http.NewRequest("POST", c.School+"/admin/api/oauth2/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Lw-Client", c.ClientID)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", 0, &Error{Code: "network", Msg: err.Error(), Retryable: true}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	var out struct {
		TokenData struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		} `json:"tokenData"`
	}
	if json.Unmarshal(body, &out) != nil || out.TokenData.AccessToken == "" {
		e := apiError(res.StatusCode, body)
		e.Code, e.Retryable = "auth", res.StatusCode >= 500
		e.Hint = "check school URL, client id and secret (Settings → Developers → API)"
		return "", 0, e
	}
	return out.TokenData.AccessToken, time.Now().Unix() + out.TokenData.ExpiresIn, nil
}

// Request is one API call. Path is relative to /admin/api, e.g. /v2/users.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   any
}

// Do sends the request, retrying rate limits and transient failures.
func (c *Client) Do(r Request) (any, error) {
	u := c.School + "/admin/api" + r.Path
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	if c.DryRun {
		return map[string]any{"dry_run": true, "method": r.Method, "url": u, "body": r.Body}, nil
	}
	var payload []byte
	if r.Body != nil {
		payload, _ = json.Marshal(r.Body)
	}
	refreshed := false
	for attempt := 0; ; attempt++ {
		tok, err := c.Token(false)
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequest(r.Method, u, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Lw-Client", c.ClientID)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "→ %s %s\n", r.Method, u)
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			// Only GETs are safe to resend after a network error.
			if r.Method == "GET" && attempt < 3 {
				c.backoff(attempt, nil)
				continue
			}
			return nil, &Error{Code: "network", Msg: err.Error(), Retryable: true}
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if c.Verbose {
			fmt.Fprintf(os.Stderr, "← %d (%d bytes)\n", res.StatusCode, len(body))
		}
		switch {
		case res.StatusCode == 401 && !refreshed && !c.envToken:
			refreshed = true
			if _, err := c.Token(true); err != nil {
				return nil, err
			}
			continue
		case res.StatusCode == 429 && attempt < 5,
			res.StatusCode >= 502 && res.StatusCode <= 504 && r.Method == "GET" && attempt < 3:
			c.backoff(attempt, res)
			continue
		case res.StatusCode >= 400:
			return nil, apiError(res.StatusCode, body)
		}
		if len(bytes.TrimSpace(body)) == 0 {
			return nil, nil
		}
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			return string(body), nil
		}
		return v, nil
	}
}

// backoff waits Retry-After if given, else exponential. LearnWorlds allows
// 30 requests per 10 seconds, so cap at 10s.
func (c *Client) backoff(attempt int, res *http.Response) {
	d := time.Duration(1<<attempt) * time.Second
	if res != nil {
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil {
			d = time.Duration(s) * time.Second
		}
	}
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	if c.Verbose {
		fmt.Fprintf(os.Stderr, "… retrying in %s\n", d)
	}
	c.sleep(d)
}

func apiError(status int, body []byte) *Error {
	var parsed struct {
		Error  string `json:"error"`
		Errors []struct {
			Context string `json:"context"`
			Message string `json:"message"`
		} `json:"errors"`
		Message string `json:"message"`
	}
	msg := ""
	if json.Unmarshal(body, &parsed) == nil {
		var parts []string
		for _, e := range parsed.Errors {
			parts = append(parts, firstNonEmpty(e.Message, e.Context))
		}
		msg = firstNonEmpty(strings.Join(parts, "; "), parsed.Error, parsed.Message)
	}
	if msg == "" {
		msg = strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
	}
	e := &Error{Status: status, Msg: fmt.Sprintf("%s (HTTP %d)", firstNonEmpty(msg, http.StatusText(status)), status)}
	switch {
	case status == 401:
		e.Code, e.Hint = "auth", "run: lw auth login"
	case status == 403:
		e.Code, e.Hint = "forbidden", "your API client may lack access to this endpoint (plan or permissions)"
	case status == 404:
		e.Code = "not_found"
	case status == 400 || status == 422:
		e.Code = "validation"
	case status == 429:
		e.Code, e.Retryable = "rate_limited", true
		e.Hint = "LearnWorlds allows 30 requests per 10 seconds"
	default:
		e.Code, e.Retryable = "api_error", status >= 500 && status != 501
	}
	return e
}

// Paged fetches one page, or every page when all is set, following meta.totalPages.
func (c *Client) Paged(r Request, all bool) (any, map[string]any, error) {
	if !all {
		v, err := c.Do(r)
		if err != nil {
			return nil, nil, err
		}
		m, _ := v.(map[string]any)
		meta, _ := m["meta"].(map[string]any)
		if d, ok := m["data"]; ok {
			return d, meta, nil
		}
		return v, meta, nil
	}
	var items []any
	var meta map[string]any
	q := url.Values{}
	for k, v := range r.Query {
		q[k] = v
	}
	for page := 1; ; page++ {
		q.Set("page", strconv.Itoa(page))
		r.Query = q
		v, err := c.Do(r)
		if err != nil {
			return nil, nil, err
		}
		if c.DryRun {
			return v, nil, nil
		}
		m, _ := v.(map[string]any)
		d, _ := m["data"].([]any)
		items = append(items, d...)
		meta, _ = m["meta"].(map[string]any)
		total, _ := meta["totalPages"].(float64)
		if len(d) == 0 || float64(page) >= total {
			break
		}
	}
	if meta != nil {
		meta = map[string]any{"totalItems": len(items), "totalPages": meta["totalPages"], "all": true}
	}
	if items == nil {
		items = []any{}
	}
	return items, meta, nil
}
