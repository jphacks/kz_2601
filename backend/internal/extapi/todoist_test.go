package extapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	todoistTestID     = "td-client"
	todoistTestSecret = "td-secret"
)

// fakeTodoist is Todoist's OAuth and API v1 on httptest (no live calls).
type fakeTodoist struct {
	url string

	mu         sync.Mutex
	challenge  string
	scope      string // granted on the next code exchange
	refreshes  int
	revoked    []string
	tasks      map[string]map[string]any // by X-Request-Id
	closed     []string
	lastPath   string
	lastQuery  url.Values
	rotateNext int
}

func newFakeTodoist(t *testing.T) (*fakeTodoist, *httptest.Server) {
	f := &fakeTodoist{scope: todoistScopeRead, tasks: map[string]map[string]any{}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != todoistTestID || r.Form.Get("client_secret") != todoistTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "td-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "td-access-0", "refresh_token": "td-refresh-0", "expires_in": 3600, "scope": f.scope, "token_type": "Bearer"})
		case "refresh_token":
			// rotated: only the newest refresh token works
			if r.Form.Get("refresh_token") != "td-refresh-"+strconv.Itoa(f.rotateNext) {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			writeJSON(w, 200, map[string]any{"access_token": "td-access-" + n, "refresh_token": "td-refresh-" + n, "expires_in": 3600, "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("POST /api/v1/revoke", func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		_ = r.ParseForm()
		if !ok || id != todoistTestID || secret != todoistTestSecret || r.Form.Get("token_type_hint") != "access_token" {
			w.WriteHeader(401)
			return
		}
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
		writeJSON(w, 200, nil)
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			writeJSON(w, 403, map[string]any{"error": "Forbidden", "error_tag": "AUTH_INSUFFICIENT_TOKEN_SCOPE"})
			return false
		case tok == "busy":
			writeJSON(w, 503, map[string]any{"error_tag": "SERVICE_UNAVAILABLE"})
			return false
		case !strings.HasPrefix(tok, "td-access-") && tok != "legacy":
			writeJSON(w, 401, map[string]any{"error_tag": "AUTH_INVALID_TOKEN"})
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		f.lastPath, f.lastQuery = r.URL.Path, r.URL.Query()
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"results": []map[string]any{
			{"id": "6Jf8VQXxpwv56VQ7", "name": "Inbox", "inbox_project": true},
			{"id": "bad id", "name": "仕事"},
		}, "next_cursor": nil})
	})
	mux.HandleFunc("POST /api/v1/tasks/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastPath = r.URL.Path
		if r.PathValue("id") == "missing" {
			writeJSON(w, 404, map[string]any{"error_tag": "NOT_FOUND"})
			return
		}
		f.closed = append(f.closed, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	api := func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastPath, f.lastQuery = r.URL.Path, r.URL.Query()
		if r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]any{"results": []map[string]any{
				{"id": "6X7rM8997g3RQmvh", "content": "牛乳を買う", "due": map[string]any{"date": "2026-11-12", "string": "明日"}},
				{"id": "6X7rfFVPjhvv84XG", "content": "電話する", "due": map[string]any{"date": "2026-11-12T10:00:00", "datetime": "2026-11-12T10:00:00", "string": "明日 10時"}},
				{"id": "bad id/../x", "content": "期限なし"},
			}, "next_cursor": nil})
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, 400, map[string]any{"error_tag": "INVALID_ARGUMENT"})
			return
		}
		rid := r.Header.Get("X-Request-Id")
		if prev, ok := f.tasks[rid]; ok && rid != "" { // Todoist's idempotency
			writeJSON(w, 200, prev)
			return
		}
		body["id"] = "task" + strconv.Itoa(len(f.tasks)+1)
		if d, ok := body["due_date"].(string); ok {
			body["due"] = map[string]any{"date": d, "string": d}
		}
		f.tasks[rid] = body
		writeJSON(w, 200, body)
	}
	mux.HandleFunc("GET /api/v1/tasks", api)
	mux.HandleFunc("GET /api/v1/tasks/filter", api)
	mux.HandleFunc("POST /api/v1/tasks", api)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeTodoist) wire(srv *httptest.Server) todoistWire {
	return todoistWire{AuthURL: srv.URL + "/oauth/authorize", TokenURL: srv.URL + "/oauth/access_token", RevokeURL: srv.URL + "/api/v1/revoke",
		APIBase: srv.URL + "/api/v1", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }}
}

func useTodoist(t *testing.T) (*env, *fakeTodoist) {
	f, srv := newFakeTodoist(t)
	old := todoistWireDefault
	todoistWireDefault = f.wire(srv)
	t.Cleanup(func() { todoistWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Todoist] = true
		c.Secrets = map[string]Secrets{Todoist: {ClientID: todoistTestID, ClientSecret: todoistTestSecret}}
	}})
	return e, f
}

// connectTodoist connects user asking for scopes; want is the scope param.
func (e *env) connectTodoist(f *fakeTodoist, scopes []string, want string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Todoist, scopes)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth/authorize?") || q.Get("scope") != want || q.Get("code_challenge_method") != "S256" ||
		q.Get("client_id") != todoistTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/todoist" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Todoist, q.Get("state"), "td-code"); back != "tailorit://connected?provider=todoist&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE, data:read) → list → add needs task:add (409, connect again
// asking for it, reads kept) → refresh (rotated) → refused refresh needs a
// reconnect → revoke.
func TestTodoistFlow(t *testing.T) {
	e, f := useTodoist(t)
	ctx := context.Background()
	e.connectTodoist(f, nil, "data:read")

	res, err := e.call(e.req(Todoist, "listTasks", map[string]string{"filter": "today | overdue", "limit": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 3 || rows[0].Title != "牛乳を買う" || rows[0].Value != "2026-11-12" || rows[0].Subtitle != "明日" ||
		rows[0].URL != "https://app.todoist.com/app/task/6X7rM8997g3RQmvh" || rows[1].Value != "2026-11-12T10:00:00" || rows[2].URL != "" {
		t.Fatalf("rows %+v", rows)
	}
	if f.lastPath != "/api/v1/tasks/filter" || f.lastQuery.Get("query") != "today | overdue" || f.lastQuery.Get("limit") != "10" {
		t.Fatalf("request %s %v", f.lastPath, f.lastQuery)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// read-only grant: adding is 409 and gives the run back
	if _, err := e.call(e.req(Todoist, "addTask", map[string]string{"content": "x"})); !errors.Is(err, ErrNotConnected) || e.runs() != 1 {
		t.Fatalf("add without scope: %v runs=%d", err, e.runs())
	}
	// connecting again for task:add asks for data:read too
	f.scope = "data:read,task:add"
	e.connectTodoist(f, []string{todoistScopeAdd}, "data:read,task:add")
	r := e.req(Todoist, "addTask", map[string]string{"content": "牛乳を買う", "dueDate": "2026-11-12", "priority": "4"})
	res, err = e.call(r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "task1" || res.Output.Item.Title != "牛乳を買う" || res.Output.Item.Value != "2026-11-12" {
		t.Fatalf("added %+v", res.Output)
	}
	if task := f.tasks[r.RequestID]; task["priority"] != float64(4) || task["due_date"] != "2026-11-12" {
		t.Fatalf("task %v", task)
	}
	// a retried request (same request id) adds no second task
	if res, err := e.call(r); err != nil || res.Output.ResultID != "task1" || len(f.tasks) != 1 {
		t.Fatalf("retry: %v %+v", err, res.Output)
	}
	if _, err := e.call(e.req(Todoist, "listTasks", nil)); err != nil || f.lastPath != "/api/v1/tasks" {
		t.Fatalf("reads after the write connect: %v", err)
	}

	// expired → refreshed with the rotated refresh token
	tok, _ := e.svc.Conns.Token(ctx, user, Todoist)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Todoist, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Todoist, "listTasks", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Todoist)
	if err != nil || tok.AccessToken != "td-access-1" || tok.RefreshToken != "td-refresh-1" || f.refreshes != 1 {
		t.Fatalf("refreshed: %v refreshes=%d", err, f.refreshes)
	}

	// an old (rotated away) refresh token is invalid_grant → error
	if err := e.svc.Conns.Save(ctx, user, Todoist, Token{AccessToken: "td-access-0", RefreshToken: "td-refresh-0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Todoist, "listTasks", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Todoist); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect and cut: the access token is revoked at Todoist
	e.connectTodoist(f, nil, "data:read")
	if err := e.svc.Revoke(ctx, user, Todoist); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Todoist); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "td-access-0" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

// A 401 on a call turns the connection to error.
func TestTodoistRevokedAtProvider(t *testing.T) {
	e, f := useTodoist(t)
	e.connectTodoist(f, nil, "data:read")
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, Todoist, Token{AccessToken: "dead"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Todoist, "listTasks", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Todoist); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func todoistTestClient(t *testing.T) (*todoist, *fakeTodoist) {
	f, srv := newFakeTodoist(t)
	c, err := newTodoist(Secrets{ClientID: todoistTestID, ClientSecret: todoistTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestTodoistArgs(t *testing.T) {
	c, f := todoistTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "legacy"}
	bad := []map[string]string{
		{},                                    // no content
		{"content": "  "},                     // blank
		{"content": strings.Repeat("あ", 501)}, // too long
		{"content": "a\x00b"},                 // control char
		{"content": "x", "dueDate": "11/12"},  // not a date
		{"content": "x", "dueDate": "2026-11-12", "dueString": "明日"},
		{"content": "x", "priority": "5"},
		{"content": "x", "priority": "p1"},
		{"content": "x", "projectId": "../me"},
		{"content": "x", "dueString": strings.Repeat("d", 151)},
		{"content": "x", "description": strings.Repeat("d", 2001)},
	}
	for i, a := range bad {
		if _, err := c.addTask(ctx, Input{Token: tok, RequestID: "x", Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("addTask case %d: %v", i, err)
		}
	}
	badList := []map[string]string{
		{"limit": "0"}, {"limit": "51"}, {"limit": "x"},
		{"filter": "today", "projectId": "p1"},
		{"projectId": "a b"},
		{"filter": strings.Repeat("f", 1025)},
	}
	for i, a := range badList {
		if _, err := c.listTasks(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("listTasks case %d: %v", i, err)
		}
	}
	if len(f.tasks) != 0 || f.lastPath != "" {
		t.Fatal("a bad argument reached Todoist")
	}
	if _, err := c.listTasks(ctx, Input{Token: tok, Args: map[string]string{"projectId": "6Jf8VQXxpwv56VQ7"}}); err != nil {
		t.Fatal(err)
	}
	if f.lastPath != "/api/v1/tasks" || f.lastQuery.Get("project_id") != "6Jf8VQXxpwv56VQ7" || f.lastQuery.Get("limit") != "20" {
		t.Fatalf("project query %s %v", f.lastPath, f.lastQuery)
	}
}

func TestTodoistErrors(t *testing.T) {
	c, f := todoistTestClient(t)
	ctx := context.Background()
	if _, err := c.listTasks(ctx, Input{Token: &Token{AccessToken: "noscope"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	_, err := c.listTasks(ctx, Input{Token: &Token{AccessToken: "busy"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := c.listTasks(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := c.addTask(ctx, Input{Token: &Token{AccessToken: "td-access-0", Scopes: []string{todoistScopeRead}}, Args: map[string]string{"content": "x"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("add with data:read only: %v", err)
	}
	if _, err := c.addTask(ctx, Input{Token: &Token{AccessToken: "td-access-0", Scopes: []string{todoistScopeWrite}}, Args: map[string]string{"content": "x"}}); err != nil {
		t.Fatalf("data:read_write covers task:add: %v", err)
	}
	// a legacy token (no refresh token) cannot be renewed: connect again
	if _, err := c.Refresh(ctx, Token{AccessToken: "legacy"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("legacy refresh: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/todoist"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	// revoking an expired token renews it first (the endpoint takes an
	// access token); a refused refresh token is ErrInvalidGrant
	// (Connections.Revoke reads the stored one again)
	f.rotateNext = 0
	if err := c.Revoke(ctx, Token{AccessToken: "td-access-0", RefreshToken: "td-refresh-0", Expiry: c.now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "td-access-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "x", RefreshToken: "gone", Expiry: c.now().Add(-time.Hour)}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("gone: %v", err)
	}
	// a legacy app's 10-year expires_in is kept as no expiry
	if tk, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"td-refresh-1"}}, Token{Scopes: []string{"data:read"}}); err != nil || tk.Expiry.IsZero() || len(tk.Scopes) != 1 {
		t.Fatalf("token %v", err)
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// without the client id / secret the provider stays off; off by default
// and never in the review build.
func TestTodoistDefinition(t *testing.T) {
	p, ok := Lookup(Todoist)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "TODOIST" {
		t.Fatalf("%+v", p)
	}
	w := todoistWireDefault
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase + "/tasks"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	// only complete asks for data:read_write (Todoist has no narrower scope
	// to close a task); nothing asks for data:delete
	for _, o := range p.Ops {
		for _, s := range o.Scopes {
			if (s == todoistScopeWrite && o.Name != "complete") || s == "data:delete" {
				t.Errorf("%s asks for a wide scope %s", o.Name, s)
			}
		}
		if o.Name == "complete" && !o.Write {
			t.Error("complete is a write op")
		}
	}
	c, err := newTodoist(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientSecret: "b"}); err == nil {
		t.Fatal("built without a client id")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[Todoist] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}

// project (a name) and label become Todoist's filter / label query.
func TestTodoistListProjectLabel(t *testing.T) {
	c, f := todoistTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "legacy"}
	cases := []struct {
		args        map[string]string
		path, query string
		label, pid  string
	}{
		{map[string]string{"project": "買い物 リスト"}, "/api/v1/tasks/filter", "#買い物 リスト", "", ""},
		{map[string]string{"project": "仕事", "label": "急ぎ"}, "/api/v1/tasks/filter", "#仕事 & @急ぎ", "", ""},
		{map[string]string{"filter": "today | overdue", "label": "急ぎ"}, "/api/v1/tasks/filter", "(today | overdue) & @急ぎ", "", ""},
		{map[string]string{"label": "急ぎ"}, "/api/v1/tasks", "", "急ぎ", ""},
		{map[string]string{"projectId": "6Jf8VQXxpwv56VQ7", "label": "急ぎ"}, "/api/v1/tasks", "", "急ぎ", "6Jf8VQXxpwv56VQ7"},
	}
	for i, tc := range cases {
		out, err := c.listTasks(ctx, Input{Token: tok, Args: tc.args})
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		q := f.lastQuery
		if f.lastPath != tc.path || q.Get("query") != tc.query || q.Get("label") != tc.label || q.Get("project_id") != tc.pid || len(out.Rows) != 3 {
			t.Errorf("case %d: %s %v", i, f.lastPath, q)
		}
	}
}

func TestTodoistProjects(t *testing.T) {
	c, f := todoistTestClient(t)
	ctx := context.Background()
	out, err := c.projects(ctx, Input{Token: &Token{AccessToken: "legacy"}, Args: map[string]string{"limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.lastPath != "/api/v1/projects" || f.lastQuery.Get("limit") != "5" || len(out.Rows) != 2 ||
		out.Rows[0] != (Row{Title: "Inbox", URL: "https://app.todoist.com/app/project/6Jf8VQXxpwv56VQ7"}) ||
		out.Rows[1] != (Row{Title: "仕事"}) {
		t.Fatalf("%s %v %+v", f.lastPath, f.lastQuery, out.Rows)
	}
	if _, err := c.projects(ctx, Input{Token: &Token{AccessToken: "noscope"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
}

func TestTodoistComplete(t *testing.T) {
	c, f := todoistTestClient(t)
	ctx := context.Background()
	write := &Token{AccessToken: "td-access-0", Scopes: []string{todoistScopeRead, todoistScopeWrite}}
	out, err := c.complete(ctx, Input{Token: write, RequestID: "c1", Args: map[string]string{"taskId": "6X7rM8997g3RQmvh"}})
	if err != nil {
		t.Fatal(err)
	}
	if out.ResultID != "6X7rM8997g3RQmvh" || f.lastPath != "/api/v1/tasks/6X7rM8997g3RQmvh/close" || len(f.closed) != 1 {
		t.Fatalf("out %+v path %s closed %v", out, f.lastPath, f.closed)
	}
	// task:add / data:read do not cover closing: connect again for data:read_write
	for _, s := range [][]string{{todoistScopeRead}, {todoistScopeRead, todoistScopeAdd}} {
		if _, err := c.complete(ctx, Input{Token: &Token{AccessToken: "td-access-0", Scopes: s}, Args: map[string]string{"taskId": "x1"}}); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("complete with %v: %v", s, err)
		}
	}
	// a task the person does not have is a bad request, not a retry
	_, err = c.complete(ctx, Input{Token: write, Args: map[string]string{"taskId": "missing"}})
	var pe *ProviderError
	if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("404: %v", err)
	}
	if _, err := c.complete(ctx, Input{Token: &Token{AccessToken: "busy"}, Args: map[string]string{"taskId": "x1"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if len(f.closed) != 1 {
		t.Fatalf("closed %v", f.closed)
	}
}

// Check refuses bad arguments before anything is counted (and nothing
// reaches Todoist).
func TestTodoistCheck(t *testing.T) {
	p, _ := Lookup(Todoist)
	bad := map[string][]map[string]string{
		"listTasks": {
			{"filter": "today", "project": "仕事"},
			{"projectId": "p1", "project": "仕事"},
			{"filter": "today", "projectId": "p1"},
			{"project": "a & b"}, {"project": "a|b"}, {"project": "(a)"}, {"project": "#a"},
			{"label": "@a"}, {"label": "a,b"}, {"label": "a\\b"}, {"label": "a\nb"}, {"label": "a*"},
			{"project": strings.Repeat("あ", 121)},
			{"filter": "today, overdue", "label": "急ぎ"},
			{"limit": "0"}, {"limit": "51"},
			{"projectId": "a b"},
		},
		"projects": {{"limit": "0"}, {"limit": "x"}, {"limit": "51"}, {"project": "x"}},
		"complete": {{}, {"taskId": ""}, {"taskId": "../x"}, {"taskId": "a b"}, {"taskId": strings.Repeat("a", 65)}, {"taskId": "x", "content": "y"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{
		"listTasks": {"project": "買い物 リスト", "label": "急ぎ", "limit": "10"},
		"projects":  {},
		"complete":  {"taskId": "6X7rM8997g3RQmvh"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}
