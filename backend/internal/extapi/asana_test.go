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
	asanaTestID     = "as-client"
	asanaTestSecret = "as-secret"
)

// fakeAsana is Asana's OAuth and REST API 1.0 on httptest (no live calls).
type fakeAsana struct {
	url string

	mu         sync.Mutex
	challenge  string
	refreshes  int
	revoked    []string
	workspaces []string
	completed  []string
	added      []string
	wsCalls    int
	lastPath   string
	lastQuery  url.Values
	lastBody   map[string]any
}

func newFakeAsana(t *testing.T) (*fakeAsana, *httptest.Server) {
	f := &fakeAsana{workspaces: []string{"1100"}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /-/oauth_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != asanaTestID || r.Form.Get("client_secret") != asanaTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "as-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "as-access-0", "refresh_token": "as-refresh", "expires_in": 3600, "token_type": "bearer",
				"data": map[string]any{"gid": "42", "name": "x"}})
		case "refresh_token":
			// not rotated: the same refresh token keeps working; no refresh_token in the answer
			if r.Form.Get("refresh_token") != "as-refresh" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.refreshes++
			writeJSON(w, 200, map[string]any{"access_token": "as-access-" + strconv.Itoa(f.refreshes), "expires_in": 3600, "token_type": "bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("POST /-/oauth_revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != asanaTestID || r.Form.Get("client_secret") != asanaTestSecret {
			w.WriteHeader(401)
			return
		}
		if r.Form.Get("token") != "as-refresh" { // bearer tokens are refused
			writeJSON(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
		w.WriteHeader(200)
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			writeJSON(w, 403, map[string]any{"errors": []map[string]string{{"message": "missing scope"}}})
			return false
		case tok == "busy":
			writeJSON(w, 503, map[string]any{"errors": []map[string]string{{"message": "down"}}})
			return false
		case tok == "limited":
			w.Header().Set("Retry-After", "30")
			writeJSON(w, 429, map[string]any{"errors": []map[string]string{{"message": "rate limit"}}})
			return false
		case !strings.HasPrefix(tok, "as-access-"):
			writeJSON(w, 401, map[string]any{"errors": []map[string]string{{"message": "Not Authorized"}}})
			return false
		}
		return true
	}
	tasks := []map[string]any{
		{"gid": "1201", "name": "資料を送る", "due_on": "2026-11-12", "completed": false, "permalink_url": "https://app.asana.com/0/0/1201"},
		{"gid": "1202", "name": "定例", "due_at": "2026-11-12T01:00:00.000Z", "due_on": "2026-11-12", "permalink_url": "https://evil.example.com/x"},
	}
	record := func(r *http.Request) {
		f.lastPath, f.lastQuery = r.URL.Path, r.URL.Query()
	}
	mux.HandleFunc("GET /api/1.0/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.wsCalls++
		var data []map[string]string
		for _, g := range f.workspaces {
			data = append(data, map[string]string{"gid": g})
		}
		writeJSON(w, 200, map[string]any{"data": data})
	})
	mux.HandleFunc("GET /api/1.0/tasks", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		record(r)
		if r.URL.Query().Get("workspace") == "404" {
			writeJSON(w, 400, map[string]any{"errors": []map[string]string{{"message": "workspace: Not a recognized ID"}}})
			return
		}
		writeJSON(w, 200, map[string]any{"data": tasks, "next_page": nil})
	})
	mux.HandleFunc("GET /api/1.0/projects/{gid}/tasks", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		record(r)
		if r.PathValue("gid") == "404" {
			writeJSON(w, 404, map[string]any{"errors": []map[string]string{{"message": "project: Unknown object"}}})
			return
		}
		writeJSON(w, 200, map[string]any{"data": tasks[:1]})
	})
	mux.HandleFunc("PUT /api/1.0/tasks/{gid}", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		record(r)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, 400, map[string]any{"errors": []map[string]string{{"message": "bad body"}}})
			return
		}
		f.lastBody = body
		id := r.PathValue("gid")
		if id == "404" {
			writeJSON(w, 404, map[string]any{"errors": []map[string]string{{"message": "task: Unknown object"}}})
			return
		}
		f.completed = append(f.completed, id)
		writeJSON(w, 200, map[string]any{"data": map[string]any{"gid": id, "name": "資料を送る", "completed": true, "permalink_url": "https://app.asana.com/0/0/" + id}})
	})
	mux.HandleFunc("POST /api/1.0/tasks", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		record(r)
		var body struct {
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, 400, map[string]any{"errors": []map[string]string{{"message": "bad body"}}})
			return
		}
		f.lastBody = body.Data
		if body.Data["workspace"] == "404" {
			writeJSON(w, 400, map[string]any{"errors": []map[string]string{{"message": "workspace: Not a recognized ID"}}})
			return
		}
		if ps, _ := body.Data["projects"].([]any); len(ps) == 1 && ps[0] == "404" {
			writeJSON(w, 404, map[string]any{"errors": []map[string]string{{"message": "project: Unknown object"}}})
			return
		}
		id := strconv.Itoa(1300 + len(f.added))
		f.added = append(f.added, id)
		due, _ := body.Data["due_on"].(string)
		writeJSON(w, 201, map[string]any{"data": map[string]any{"gid": id, "name": body.Data["name"], "due_on": due, "completed": false,
			"permalink_url": "https://app.asana.com/0/0/" + id}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeAsana) wire(srv *httptest.Server) asanaWire {
	return asanaWire{AuthURL: srv.URL + "/-/oauth_authorize", TokenURL: srv.URL + "/-/oauth_token", RevokeURL: srv.URL + "/-/oauth_revoke",
		APIBase: srv.URL + "/api/1.0", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }}
}

func useAsana(t *testing.T) (*env, *fakeAsana) {
	f, srv := newFakeAsana(t)
	old := asanaWireDefault
	asanaWireDefault = f.wire(srv)
	t.Cleanup(func() { asanaWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Asana] = true
		c.Secrets = map[string]Secrets{Asana: {ClientID: asanaTestID, ClientSecret: asanaTestSecret}}
	}})
	return e, f
}

// connectAsana connects user asking for scopes; want is the scope param.
func (e *env) connectAsana(f *fakeAsana, scopes []string, want string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Asana, scopes)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/-/oauth_authorize?") || q.Get("scope") != want || q.Get("code_challenge_method") != "S256" ||
		q.Get("client_id") != asanaTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/asana" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Asana, q.Get("state"), "as-code"); back != "tailorit://connected?provider=asana&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE, reads) → myTasks → complete needs tasks:write (409,
// connect again asking for it, reads kept) → refresh (not rotated) →
// revoke with the refresh token.
func TestAsanaFlow(t *testing.T) {
	e, f := useAsana(t)
	ctx := context.Background()
	e.connectAsana(f, nil, "tasks:read workspaces:read")

	res, err := e.call(e.req(Asana, "myTasks", map[string]string{"limit": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "資料を送る", Value: "2026-11-12", URL: "https://app.asana.com/0/0/1201"}) ||
		rows[1] != (Row{Title: "定例", Value: "2026-11-12T01:00:00.000Z"}) {
		t.Fatalf("rows %+v", rows)
	}
	q := f.lastQuery
	if f.lastPath != "/api/1.0/tasks" || q.Get("assignee") != "me" || q.Get("workspace") != "1100" || q.Get("completed_since") != "now" ||
		q.Get("limit") != "10" || q.Get("opt_fields") != asanaTaskFields {
		t.Fatalf("request %s %v", f.lastPath, q)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// read-only grant: completing is 409 and gives the run back
	if _, err := e.call(e.req(Asana, "complete", map[string]string{"taskId": "1201"})); !errors.Is(err, ErrNotConnected) || e.runs() != 1 {
		t.Fatalf("complete without scope: %v runs=%d", err, e.runs())
	}
	// connecting again for tasks:write asks for the reads too (not additive)
	e.connectAsana(f, []string{asanaScopeTasksWrite}, "tasks:read workspaces:read tasks:write")
	res, err = e.call(e.req(Asana, "complete", map[string]string{"taskId": "1201"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "1201" || res.Output.Item.Title != "資料を送る" || res.Output.Item.Subtitle != "完了" ||
		f.lastPath != "/api/1.0/tasks/1201" || len(f.completed) != 1 {
		t.Fatalf("completed %+v %s", res.Output, f.lastPath)
	}
	if d, _ := f.lastBody["data"].(map[string]any); d["completed"] != true || len(d) != 1 {
		t.Fatalf("body %v", f.lastBody)
	}

	// expired → refreshed; the refresh token is kept
	tok, _ := e.svc.Conns.Token(ctx, user, Asana)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Asana, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Asana, "projectTasks", map[string]string{"projectId": "1300"})); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Asana)
	if err != nil || tok.AccessToken != "as-access-1" || tok.RefreshToken != "as-refresh" || f.refreshes != 1 {
		t.Fatalf("refreshed: %v refreshes=%d", err, f.refreshes)
	}

	// cut: the refresh token is revoked at Asana
	if err := e.svc.Revoke(ctx, user, Asana); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Asana); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "as-refresh" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

// A 401 on a call turns the connection to error.
func TestAsanaRevokedAtProvider(t *testing.T) {
	e, f := useAsana(t)
	e.connectAsana(f, nil, "tasks:read workspaces:read")
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, Asana, Token{AccessToken: "dead"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Asana, "myTasks", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Asana); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func asanaTestClient(t *testing.T) (*asana, *fakeAsana) {
	f, srv := newFakeAsana(t)
	c, err := newAsana(Secrets{ClientID: asanaTestID, ClientSecret: asanaTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestAsanaWorkspaces(t *testing.T) {
	c, f := asanaTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "as-access-0"}
	// a given workspace is used as is (no workspaces call needed)
	if _, err := c.myTasks(ctx, Input{Token: tok, Args: map[string]string{"workspaceId": "2200"}}); err != nil || f.lastQuery.Get("workspace") != "2200" || f.lastQuery.Get("limit") != "20" {
		t.Fatalf("given: %v %v", err, f.lastQuery)
	}
	// several workspaces: workspaceId is required
	f.workspaces = []string{"1100", "2200"}
	f.lastPath = ""
	if _, err := c.myTasks(ctx, Input{Token: tok}); Reason(err) != ReasonBadRequest || f.lastPath != "" {
		t.Fatalf("several: %v", err)
	}
	f.workspaces = nil
	if _, err := c.myTasks(ctx, Input{Token: tok}); Reason(err) != ReasonBadRequest {
		t.Fatalf("none: %v", err)
	}
	// a workspace the person does not have is a bad request, not a retry
	_, err := c.myTasks(ctx, Input{Token: tok, Args: map[string]string{"workspaceId": "404"}})
	var pe *ProviderError
	if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("unknown workspace: %v", err)
	}
}

func TestAsanaProjectTasks(t *testing.T) {
	c, f := asanaTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "as-access-0"}
	out, err := c.projectTasks(ctx, Input{Token: tok, Args: map[string]string{"projectId": "1300", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.lastPath != "/api/1.0/projects/1300/tasks" || f.lastQuery.Get("limit") != "5" || f.lastQuery.Get("completed_since") != "now" || len(out.Rows) != 1 {
		t.Fatalf("%s %v %+v", f.lastPath, f.lastQuery, out.Rows)
	}
	if _, err := c.projectTasks(ctx, Input{Token: tok, Args: map[string]string{"projectId": "404"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("404: %v", err)
	}
}

func TestAsanaErrors(t *testing.T) {
	c, f := asanaTestClient(t)
	ctx := context.Background()
	args := map[string]string{"workspaceId": "1100"}
	if _, err := c.myTasks(ctx, Input{Token: &Token{AccessToken: "noscope"}, Args: args}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	if _, err := c.myTasks(ctx, Input{Token: &Token{AccessToken: "dead"}, Args: args}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	for _, tok := range []string{"busy", "limited"} {
		_, err := c.myTasks(ctx, Input{Token: &Token{AccessToken: tok}, Args: args})
		var pe *ProviderError
		if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	if _, err := c.myTasks(ctx, Input{Args: args}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	// complete: tasks:read alone does not cover it; an unknown task is 400
	if _, err := c.complete(ctx, Input{Token: &Token{AccessToken: "as-access-0", Scopes: asanaReadScopes}, Args: map[string]string{"taskId": "1201"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("complete with reads only: %v", err)
	}
	write := &Token{AccessToken: "as-access-0", Scopes: []string{asanaScopeTasksWrite}}
	if _, err := c.complete(ctx, Input{Token: write, Args: map[string]string{"taskId": "404"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("complete 404: %v", err)
	}
	if _, err := c.complete(ctx, Input{Token: &Token{AccessToken: "busy", Scopes: []string{asanaScopeTasksWrite}}, Args: map[string]string{"taskId": "1"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("complete 503: %v", err)
	}
	if len(f.completed) != 0 {
		t.Fatalf("completed %v", f.completed)
	}
	// OAuth edges
	if _, err := c.Refresh(ctx, Token{AccessToken: "x"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a refresh token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{RefreshToken: "gone"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refused refresh: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/asana"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	// a refresh token already gone (400) or none at all is done
	if err := c.Revoke(ctx, Token{RefreshToken: "gone"}); err != nil {
		t.Fatalf("gone: %v", err)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "x"}); err != nil || len(f.revoked) != 0 {
		t.Fatalf("no refresh token: %v %v", err, f.revoked)
	}
}

// Check refuses bad arguments before anything is counted.
func TestAsanaCheck(t *testing.T) {
	p, _ := Lookup(Asana)
	bad := map[string][]map[string]string{
		"myTasks":      {{"workspaceId": "abc"}, {"workspaceId": "../1"}, {"workspaceId": strings.Repeat("1", 33)}, {"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"projectId": "1"}},
		"projectTasks": {{}, {"projectId": ""}, {"projectId": "1 2"}, {"projectId": "1", "limit": "51"}},
		"complete":     {{}, {"taskId": ""}, {"taskId": "x1"}, {"taskId": "1/../2"}, {"taskId": "1", "name": "x"}},
		"addTask": {{}, {"name": "  "}, {"name": strings.Repeat("あ", asanaNameMax+1)}, {"name": "a\x00b"}, {"name": "a\x7f"},
			{"name": "x", "notes": strings.Repeat("n", asanaNotesMax+1)}, {"name": "x", "notes": "a\x1bb"}, {"name": "\xff"},
			{"name": "x", "dueOn": "2026/11/12"}, {"name": "x", "dueOn": "2026-13-01"}, {"name": "x", "dueOn": "2026-11-12T10:00:00Z"}, {"name": "x", "dueOn": "2026-2-1"},
			{"name": "x", "workspaceId": "ws"}, {"name": "x", "projectId": "1/2"}, {"name": "x", "taskId": "1"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{
		"myTasks":      {},
		"projectTasks": {"projectId": "1300", "limit": "50"},
		"complete":     {"taskId": "1201"},
		"addTask":      {"name": strings.Repeat("あ", asanaNameMax), "notes": "1行目\n\t2行目", "dueOn": "2026-11-30", "workspaceId": "1100", "projectId": "1300"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// without the client id / secret the provider stays off; off by default
// and never in the review build; only complete and addTask write (the DSL
// asks for a confirmation).
func TestAsanaDefinition(t *testing.T) {
	p, ok := Lookup(Asana)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "ASANA" || len(p.Hosts) != 1 {
		t.Fatalf("%+v", p)
	}
	w := asanaWireDefault
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase + "/tasks"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, o := range p.Ops {
		if o.Write != (o.Name == "complete" || o.Name == "addTask") {
			t.Errorf("%s write=%v", o.Name, o.Write)
		}
		for _, s := range o.Scopes {
			if (s == asanaScopeTasksWrite && !o.Write) || strings.HasSuffix(s, ":delete") || s == "default" {
				t.Errorf("%s asks for a wide scope %s", o.Name, s)
			}
		}
	}
	c, err := newAsana(Secrets{ClientID: "a", ClientSecret: "b"}, w)
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
	if cfg.Enabled[Asana] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}

// addTask through the service: a read-only grant is 409 and gives the run
// back; connecting again for tasks:write asks for the reads too; the task
// is added to the person's only workspace, assigned to them.
func TestAsanaAddTaskFlow(t *testing.T) {
	e, f := useAsana(t)
	e.connectAsana(f, nil, "tasks:read workspaces:read")
	args := map[string]string{"name": " 資料を送る ", "notes": "先方へ\n添付あり", "dueOn": "2026-11-20"}
	if _, err := e.call(e.req(Asana, "addTask", args)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 || len(f.added) != 0 {
		t.Fatalf("addTask without scope: %v runs=%d added=%v", err, e.runs(), f.added)
	}
	e.connectAsana(f, []string{asanaScopeTasksWrite}, "tasks:read workspaces:read tasks:write")
	res, err := e.call(e.req(Asana, "addTask", args))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "1300" || res.Output.Item == nil || len(res.Output.Rows) != 0 ||
		*res.Output.Item != (Row{Title: "資料を送る", Value: "2026-11-20", URL: "https://app.asana.com/0/0/1300"}) {
		t.Fatalf("output %+v", res.Output)
	}
	want := map[string]any{"name": "資料を送る", "notes": "先方へ\n添付あり", "due_on": "2026-11-20", "assignee": "me", "workspace": "1100"}
	if len(f.lastBody) != len(want) || f.lastPath != "/api/1.0/tasks" || f.lastQuery.Get("opt_fields") != asanaTaskFields {
		t.Fatalf("request %s %v %v", f.lastPath, f.lastQuery, f.lastBody)
	}
	for k, v := range want {
		if f.lastBody[k] != v {
			t.Fatalf("body %s = %v, want %v", k, f.lastBody[k], v)
		}
	}
	if e.runs() != 1 || res.Charge.Coins != 0 || len(f.added) != 1 {
		t.Fatalf("runs = %d added=%v", e.runs(), f.added)
	}
}

func TestAsanaAddTask(t *testing.T) {
	c, f := asanaTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "as-access-0", Scopes: []string{asanaScopeTasksWrite}}

	// a given workspace is used as is (no workspaces call)
	out, err := c.addTask(ctx, Input{Token: tok, Args: map[string]string{"name": "定例", "workspaceId": "2200"}})
	if err != nil || out.ResultID != "1300" || out.Item.Title != "定例" || out.Item.Value != "" || f.wsCalls != 0 {
		t.Fatalf("given workspace: %v %+v ws=%d", err, out, f.wsCalls)
	}
	if f.lastBody["workspace"] != "2200" || f.lastBody["projects"] != nil || f.lastBody["notes"] != nil || f.lastBody["due_on"] != nil {
		t.Fatalf("body %v", f.lastBody)
	}
	// a project alone: Asana takes its workspace (no workspaces call)
	if _, err := c.addTask(ctx, Input{Token: tok, Args: map[string]string{"name": "x", "projectId": "1300"}}); err != nil || f.wsCalls != 0 {
		t.Fatalf("project: %v ws=%d", err, f.wsCalls)
	}
	if ps, _ := f.lastBody["projects"].([]any); len(ps) != 1 || ps[0] != "1300" || f.lastBody["workspace"] != nil {
		t.Fatalf("project body %v", f.lastBody)
	}
	if _, err := c.addTask(ctx, Input{Token: tok, Args: map[string]string{"name": "x", "projectId": "404"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("unknown project: %v", err)
	}

	// several workspaces / none without workspaceId: 400 and nothing added
	n := len(f.added)
	for _, ws := range [][]string{{"1100", "2200"}, nil} {
		f.workspaces = ws
		if _, err := c.addTask(ctx, Input{Token: tok, Args: map[string]string{"name": "x"}}); Reason(err) != ReasonBadRequest || len(f.added) != n {
			t.Fatalf("workspaces %v: %v added=%v", ws, err, f.added)
		}
	}
	// a workspace the person does not have is a bad request, not a retry
	_, err = c.addTask(ctx, Input{Token: tok, Args: map[string]string{"name": "x", "workspaceId": "404"}})
	var pe *ProviderError
	if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("unknown workspace: %v", err)
	}

	// scope missing: reads only (before any call), or Asana's 403 naming a scope
	reads := &Token{AccessToken: "as-access-0", Scopes: asanaReadScopes}
	if _, err := c.addTask(ctx, Input{Token: reads, Args: map[string]string{"name": "x", "workspaceId": "1100"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("reads only: %v", err)
	}
	if _, err := c.addTask(ctx, Input{Token: &Token{AccessToken: "noscope"}, Args: map[string]string{"name": "x", "workspaceId": "1100"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403 scope: %v", err)
	}
	busy := &Token{AccessToken: "busy", Scopes: []string{asanaScopeTasksWrite}}
	if _, err := c.addTask(ctx, Input{Token: busy, Args: map[string]string{"name": "x", "workspaceId": "1100"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	// bad arguments are refused before anything is sent
	f.lastPath = ""
	for _, a := range []map[string]string{{}, {"name": "x", "dueOn": "11/20"}, {"name": "x", "workspaceId": "a"}} {
		if _, err := c.addTask(ctx, Input{Token: tok, Args: a}); Reason(err) != ReasonBadRequest || f.lastPath != "" {
			t.Fatalf("%v: %v %s", a, err, f.lastPath)
		}
	}
}
