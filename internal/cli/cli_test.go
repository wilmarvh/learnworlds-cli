package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSchool mimics LearnWorlds: token endpoint, paged users, one 429, enrollment echo.
func fakeSchool(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var mints, hits429 atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/api/oauth2/access_token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		var p map[string]string
		json.Unmarshal([]byte(r.PostForm.Get("data")), &p)
		if p["client_secret"] != "s3cret" || r.Header.Get("Lw-Client") != "cid" {
			w.WriteHeader(400)
			io.WriteString(w, `{"errors":[{"code":400,"context":"access_denied","message":"denied"}],"success":false}`)
			return
		}
		mints.Add(1)
		io.WriteString(w, `{"tokenData":{"access_token":"tok","token_type":"Bearer","expires_in":8000},"success":true}`)
	})
	mux.HandleFunc("GET /admin/api/v2/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		if hits429.Add(1) == 1 {
			w.WriteHeader(429)
			io.WriteString(w, `{"error":"Too many requests"}`)
			return
		}
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		io.WriteString(w, `{"data":[{"id":"u`+page+`","email":"u`+page+`@x.com"}],"meta":{"page":`+page+`,"totalItems":3,"totalPages":3,"itemsPerPage":1}}`)
	})
	mux.HandleFunc("POST /admin/api/v2/users/{id}/enrollment", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.WriteHeader(201)
		io.WriteString(w, `{"user":"`+r.PathValue("id")+`","body":`+string(b)+`}`)
	})
	mux.HandleFunc("GET /admin/api/v2/courses/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		io.WriteString(w, `{"errors":[{"code":404,"message":"Course not found"}],"success":false}`)
	})
	return httptest.NewServer(mux), &mints
}

func run(t *testing.T, args ...string) (map[string]any, int) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	root := NewRoot()
	root.SetArgs(append(args, "--json"))
	code := 0
	if err := root.Execute(); err != nil {
		e, _ := err.(*Error)
		if e == nil {
			e = &Error{Code: "usage", Msg: err.Error()}
		}
		renderError(ModeJSON, e)
		code = e.ExitCode()
	}
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("bad json %q: %v", buf.String(), err)
	}
	return out, code
}

func TestCLI(t *testing.T) {
	srv, mints := fakeSchool(t)
	defer srv.Close()
	t.Setenv("LW_CONFIG_DIR", t.TempDir())
	for _, k := range []string{"LW_PROFILE", "LW_SCHOOL_URL", "LW_CLIENT_ID", "LW_CLIENT_SECRET", "LW_ACCESS_TOKEN", "LEARNWORLDS_SCHOOL_URL", "LEARNWORLDS_CLIENT_ID", "LEARNWORLDS_CLIENT_SECRET"} {
		t.Setenv(k, "")
	}
	sleepFn = func(time.Duration) {}

	// Unconfigured: auth error with hint.
	out, code := run(t, "users", "list")
	if out["ok"] != false || out["code"] != "auth" || code != 3 {
		t.Fatalf("unconfigured: %v code %d", out, code)
	}

	// Login with secret on stdin.
	stdin := os.Stdin
	pr, pw, _ := os.Pipe()
	pw.WriteString("s3cret\n")
	pw.Close()
	os.Stdin = pr
	out, code = run(t, "auth", "login", "--school", srv.URL, "--client-id", "cid", "-P", "school")
	os.Stdin = stdin
	if code != 0 || out["ok"] != true {
		t.Fatalf("login: %v", out)
	}

	// --all walks 3 pages and survives one 429; token is cached (one mint from login).
	out, code = run(t, "users", "list", "--all")
	data, _ := out["data"].([]any)
	if code != 0 || len(data) != 3 || mints.Load() != 1 {
		t.Fatalf("users list --all: %v (mints %d)", out, mints.Load())
	}

	// Single page shows next-page breadcrumb.
	out, _ = run(t, "users", "list", "--page", "2")
	if !strings.Contains(toJSON(out["breadcrumbs"]), "--page 3") {
		t.Fatalf("breadcrumbs: %v", out["breadcrumbs"])
	}

	// Enroll: email path-escaped, positional product into body, defaults applied.
	out, _ = run(t, "users", "enroll", "jane+lw@x.com", "intro-course", "--notify")
	body := out["data"].(map[string]any)["body"].(map[string]any)
	if out["data"].(map[string]any)["user"] != "jane+lw@x.com" || body["productId"] != "intro-course" || body["productType"] != "course" || body["price"] != 0.0 || body["send_enrollment_email"] != true {
		t.Fatalf("enroll: %v", out)
	}

	// Dry run sends nothing.
	out, _ = run(t, "users", "unenroll", "jane@x.com", "intro-course", "--dry-run")
	if d := out["data"].(map[string]any); d["dry_run"] != true || d["method"] != "DELETE" {
		t.Fatalf("dry run: %v", out)
	}

	// API errors map to stable codes.
	out, code = run(t, "courses", "show", "nope")
	if out["code"] != "not_found" || code != 4 || !strings.Contains(out["error"].(string), "Course not found") {
		t.Fatalf("not found: %v", out)
	}

	// Wrong arg count is a usage error.
	if _, code = run(t, "users", "enroll", "only-one"); code != 2 {
		t.Fatalf("usage code %d", code)
	}

	// Every endpoint is reachable as a command.
	out, _ = run(t, "commands")
	n := 0
	for _, c := range out["data"].([]any) {
		if c.(map[string]any)["endpoint"] != nil {
			n++
		}
	}
	if n != len(endpoints) {
		t.Fatalf("commands lists %d endpoints, table has %d", n, len(endpoints))
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
