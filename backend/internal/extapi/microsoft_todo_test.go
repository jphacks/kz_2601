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
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMicrosoftTodo is Microsoft identity's token endpoint and Graph's To Do
// on httptest (no live calls).
type fakeMicrosoftTodo struct {
	url string

	mu        sync.Mutex
	challenge string
	created   []map[string]any
	lastQuery url.Values
	lastPath  string
}

const (
	mstodoTestID     = "todo-client-id"
	mstodoTestSecret = "todo-client-secret"
	mstodoTestList   = "AQMkADAwATM0MDAAMS0xYzQ4LTYwNTktMDACLTAwCgAuAAAD=="
)

func newFakeMicrosoftTodo(t *testing.T) (*fakeMicrosoftTodo, *httptest.Server) {
	f := &fakeMicrosoftTodo{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /common/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != mstodoTestID || r.Form.Get("client_secret") != mstodoTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || !strings.HasSuffix(r.Form.Get("redirect_uri"), "/oauth/callback/microsoftTodo") {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600,
				"scope": "Tasks.Read offline_access openid", "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "access-1", "wide":
			return true
		case "narrow":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": "ErrorAccessDenied", "message": "Access is denied."}})
		case "busy":
			writeJSON(w, 503, map[string]any{"error": map[string]any{"code": "ServiceNotAvailable"}})
		default:
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "InvalidAuthenticationToken"}})
		}
		return false
	}
	mux.HandleFunc("GET /v1.0/me/todo/lists", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		f.lastQuery, f.lastPath = r.URL.Query(), r.URL.Path
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"value": []map[string]any{
			{"id": mstodoTestList, "displayName": "タスク", "wellknownListName": "defaultList"},
			{"id": "list-2", "displayName": "買い物", "wellknownListName": "none"},
			{"id": "bad id!", "displayName": ""},
		}})
	})
	mux.HandleFunc("GET /v1.0/me/todo/lists/{list}/tasks", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.PathValue("list") == "missing" {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "ErrorItemNotFound"}})
			return
		}
		f.mu.Lock()
		f.lastQuery, f.lastPath = r.URL.Query(), r.URL.Path
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"value": []map[string]any{
			{"id": "t1", "title": "牛乳を買う", "status": "notStarted",
				"dueDateTime": map[string]string{"dateTime": "2026-11-20T00:00:00.0000000", "timeZone": "Tokyo Standard Time"}},
			{"id": "t2", "title": "", "status": "completed"},
			{"id": "t3", "title": "電話", "status": "somethingNew"},
		}})
	})
	mux.HandleFunc("POST /v1.0/me/todo/lists/{list}/tasks", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.PathValue("list") == "missing" {
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "ErrorItemNotFound"}})
			return
		}
		var task map[string]any
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil || r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "BadRequest"}})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastPath = r.URL.Path
		f.created = append(f.created, task)
		task["id"] = "new-1"
		task["status"] = "notStarted"
		writeJSON(w, 201, task)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeMicrosoftTodo) wire(srv *httptest.Server) outlookWire {
	return outlookWire{AuthURL: srv.URL + "/common/oauth2/v2.0/authorize", TokenURL: srv.URL + "/common/oauth2/v2.0/token",
		APIBase: srv.URL + "/v1.0", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, gcalJST) }}
}

func useMicrosoftTodo(t *testing.T) (*env, *fakeMicrosoftTodo) {
	f, srv := newFakeMicrosoftTodo(t)
	old := microsoftTodoWire
	microsoftTodoWire = f.wire(srv)
	t.Cleanup(func() { microsoftTodoWire = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[MicrosoftTodo] = true
		c.Secrets = map[string]Secrets{MicrosoftTodo: {ClientID: mstodoTestID, ClientSecret: mstodoTestSecret}}
	}})
	return e, f
}

func (e *env) connectMicrosoftTodo(f *fakeMicrosoftTodo) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, MicrosoftTodo, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/common/oauth2/v2.0/authorize?") || q.Get("scope") != mstodoScopeRead+" offline_access" ||
		q.Get("code_challenge_method") != "S256" || q.Get("client_id") != mstodoTestID ||
		q.Get("redirect_uri") != "https://runner.example/oauth/callback/microsoftTodo" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, MicrosoftTodo, q.Get("state"), "good-code"); back != "tailorit://connected?provider=microsoftTodo&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE) → lists → tasks → a write without its scope is 409 (the
// run given back) → a write with it.
func TestMicrosoftTodoFlow(t *testing.T) {
	e, f := useMicrosoftTodo(t)
	ctx := context.Background()
	if _, err := e.call(e.req(MicrosoftTodo, "lists", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("before connecting: %v", err)
	}
	e.connectMicrosoftTodo(f)
	c, _, _ := e.mem.Get(ctx, user, MicrosoftTodo)
	if c.Status != StatusActive || len(c.Scopes) != 1 || c.Scopes[0] != mstodoScopeRead {
		t.Fatalf("connection %+v", c)
	}

	res, err := e.call(e.req(MicrosoftTodo, "lists", map[string]string{"limit": "5"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 3 || rows[0].Title != "タスク" || rows[0].Value != mstodoTestList || rows[0].Subtitle != mstodoDefaultList ||
		rows[1].Title != "買い物" || rows[1].Subtitle != "" || rows[2].Title != mstodoNoTitle || rows[2].Value != "" || res.Charge.Coins != 0 {
		t.Fatalf("rows %+v", rows)
	}
	if q := f.lastQuery; q.Get("$top") != "5" || q.Get("$select") != "id,displayName,wellknownListName" {
		t.Fatalf("query %v", q)
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	res, err = e.call(e.req(MicrosoftTodo, "tasks", map[string]string{"listId": mstodoTestList}))
	if err != nil {
		t.Fatal(err)
	}
	rows = res.Output.Rows
	if len(rows) != 3 || rows[0].Title != "牛乳を買う" || rows[0].Subtitle != "2026-11-20" || rows[0].Value != "未着手" ||
		rows[1].Title != mstodoNoTitle || rows[1].Subtitle != "" || rows[1].Value != "完了" || rows[2].Value != "somethingNew" {
		t.Fatalf("rows %+v", rows)
	}
	if f.lastPath != "/v1.0/me/todo/lists/"+mstodoTestList+"/tasks" || f.lastQuery.Get("$top") != "20" {
		t.Fatalf("path %s query %v", f.lastPath, f.lastQuery)
	}

	// the read-only grant cannot write: 409, the run given back
	_, err = e.call(e.req(MicrosoftTodo, "addTask", map[string]string{"listId": mstodoTestList, "title": "x"}))
	if !errors.Is(err, ErrNotConnected) || e.runs() != 2 {
		t.Fatalf("write without scope: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, MicrosoftTodo); c.Status != StatusActive {
		t.Fatal("a missing scope must not break the connection")
	}

	// connected again with the write scope
	if err := e.svc.Conns.Save(ctx, user, MicrosoftTodo, Token{AccessToken: "wide", RefreshToken: "refresh-1",
		Expiry: e.c.t.Add(time.Hour), Scopes: []string{mstodoScopeWrite}}); err != nil {
		t.Fatal(err)
	}
	res, err = e.call(e.req(MicrosoftTodo, "addTask", map[string]string{"listId": mstodoTestList, "title": " 牛乳 ", "dueDate": "2026-11-20", "body": "低脂肪"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "new-1" || res.Output.Item == nil || res.Output.Item.Title != "牛乳" || res.Output.Item.Subtitle != "2026-11-20" || res.Output.Item.Value != "未着手" || len(f.created) != 1 {
		t.Fatalf("addTask %+v created %v", res.Output, f.created)
	}
	task := f.created[0]
	if task["title"] != "牛乳" || task["dueDateTime"].(map[string]any)["dateTime"] != "2026-11-20T00:00:00" ||
		task["dueDateTime"].(map[string]any)["timeZone"] != outlookTZ || task["body"].(map[string]any)["content"] != "低脂肪" ||
		task["body"].(map[string]any)["contentType"] != "text" {
		t.Fatalf("sent %v", task)
	}
	if e.runs() != 3 {
		t.Fatalf("runs = %d", e.runs())
	}
}

func mstodoTestClient(t *testing.T) (*microsoftTodo, *fakeMicrosoftTodo) {
	f, srv := newFakeMicrosoftTodo(t)
	m, err := newMicrosoftTodo(Secrets{ClientID: mstodoTestID, ClientSecret: mstodoTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return m, f
}

func TestMicrosoftTodoArgs(t *testing.T) {
	m, f := mstodoTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "wide"}
	p, _ := Lookup(MicrosoftTodo)
	badLists := []map[string]string{{"limit": "0"}, {"limit": "51"}, {"limit": "x"}}
	for i, a := range badLists {
		if _, err := m.lists(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("lists case %d: %v", i, err)
		}
		if _, err := p.CheckArgs("lists", a); !errors.Is(err, ErrUnknown) {
			t.Errorf("lists check %d: %v", i, err)
		}
	}
	badTasks := []map[string]string{{}, {"listId": "bad id!"}, {"listId": mstodoTestList, "limit": "51"}, {"listId": strings.Repeat("a", 257)}}
	for i, a := range badTasks {
		if _, err := m.tasks(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("tasks case %d: %v", i, err)
		}
		if _, err := p.CheckArgs("tasks", a); !errors.Is(err, ErrUnknown) {
			t.Errorf("tasks check %d: %v", i, err)
		}
	}
	badAdd := []map[string]string{
		{"title": "x"},                             // no list
		{"listId": mstodoTestList},                 // no title
		{"listId": mstodoTestList, "title": "   "}, // blank title
		{"listId": mstodoTestList, "title": strings.Repeat("あ", 256)},
		{"listId": mstodoTestList, "title": "a\x00b"}, // control char
		{"listId": mstodoTestList, "title": "x", "dueDate": "11/20"},
		{"listId": mstodoTestList, "title": "x", "dueDate": "2026-11-20T10:00:00Z"},
		{"listId": mstodoTestList, "title": "x", "body": strings.Repeat("b", 8001)},
	}
	for i, a := range badAdd {
		if _, err := m.addTask(ctx, Input{Token: tok, RequestID: "x", Args: a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("addTask case %d: %v", i, err)
		}
		if _, err := p.CheckArgs("addTask", a); !errors.Is(err, ErrUnknown) {
			t.Errorf("addTask check %d: %v", i, err)
		}
	}
	if len(f.created) != 0 || f.lastQuery != nil {
		t.Fatal("a bad argument reached Graph")
	}
	// no body / due: neither is sent
	if _, err := m.addTask(ctx, Input{Token: tok, Args: map[string]string{"listId": mstodoTestList, "title": "x"}}); err != nil {
		t.Fatal(err)
	}
	if task := f.created[0]; task["dueDateTime"] != nil || task["body"] != nil {
		t.Fatalf("sent %v", task)
	}
}

func TestMicrosoftTodoErrors(t *testing.T) {
	m, _ := mstodoTestClient(t)
	ctx := context.Background()
	if _, err := m.lists(ctx, Input{Token: &Token{AccessToken: "narrow"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403 scope: %v", err)
	}
	_, err := m.lists(ctx, Input{Token: &Token{AccessToken: "busy"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || pe.Billable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	_, err = m.tasks(ctx, Input{Token: &Token{AccessToken: "wide"}, Args: map[string]string{"listId": "missing"}})
	if !errors.As(err, &pe) || pe.Retryable || pe.Status != 404 || !errors.Is(err, ErrUnknown) || errors.Is(err, ErrBadArgs) || Reason(err) != ReasonBadRequest {
		t.Fatalf("404: %v", err)
	}
	if _, err := m.lists(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := m.lists(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := m.addTask(ctx, Input{Token: &Token{AccessToken: "x", Scopes: []string{mstodoScopeRead}}, Args: map[string]string{"listId": mstodoTestList, "title": "x"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("read scope writes: %v", err)
	}
	if _, err := m.tasks(ctx, Input{Token: &Token{AccessToken: "x", Scopes: []string{outlookScopeRead}}, Args: map[string]string{"listId": mstodoTestList}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("calendar scope reads tasks: %v", err)
	}
	if err := m.o.Revoke(ctx, Token{RefreshToken: "r"}); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
}

// The real endpoints are on the allowlist (common tenant); nothing else is
// reachable; the OAuth client is Outlook calendar's unless its own is set;
// off by default and never in the review build.
func TestMicrosoftTodoDefinition(t *testing.T) {
	p, ok := Lookup(MicrosoftTodo)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "MICROSOFT_TODO" || p.ClientFrom != OutlookCalendar {
		t.Fatalf("%+v", p)
	}
	w := microsoftTodoWire
	if !strings.Contains(w.TokenURL, "/common/") || !strings.Contains(w.AuthURL, "/common/") {
		t.Fatal("not the common tenant")
	}
	for _, raw := range []string{w.TokenURL, w.APIBase + "/me/todo/lists", w.APIBase + "/me/todo/lists/x/tasks"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, o := range p.Ops {
		for _, s := range o.Scopes {
			if s != mstodoScopeRead && s != mstodoScopeWrite {
				t.Errorf("%s asks for %s", o.Name, s)
			}
		}
		if o.Write != (o.Name == "addTask") {
			t.Errorf("%s write=%v", o.Name, o.Write)
		}
	}
	m, err := newMicrosoftTodo(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.o.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil || !strings.Contains(err.Error(), "EXT_MICROSOFT_TODO_CLIENT_ID") {
		t.Fatalf("built without a secret: %v", err)
	}
	c, _ := ConfigFromEnv(func(string) string { return "" })
	if c.Enabled[MicrosoftTodo] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
	// Outlook calendar's client unless its own is set
	env := map[string]string{"EXT_MICROSOFT_TODO_ENABLED": "1", "EXT_OUTLOOK_CALENDAR_CLIENT_ID": "oc-id", "EXT_OUTLOOK_CALENDAR_CLIENT_SECRET": "oc-secret"}
	c, err = ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil || !c.Enabled[MicrosoftTodo] || c.Secrets[MicrosoftTodo].ClientID != "oc-id" || c.Secrets[MicrosoftTodo].ClientSecret != "oc-secret" {
		t.Fatalf("ClientFrom: %v %+v", err, c.Secrets[MicrosoftTodo])
	}
	env["EXT_MICROSOFT_TODO_CLIENT_ID"], env["EXT_MICROSOFT_TODO_CLIENT_SECRET"] = "own-id", "own-secret"
	c, _ = ConfigFromEnv(func(k string) string { return env[k] })
	if c.Secrets[MicrosoftTodo].ClientID != "own-id" {
		t.Fatalf("own client: %+v", c.Secrets[MicrosoftTodo])
	}
}

// 期限は JST の日付: UTC で返った時も日本の日付に直す。
func TestMicrosoftTodoDueJST(t *testing.T) {
	for _, c := range []struct {
		w    outlookWhen
		want string
	}{
		{outlookWhen{DateTime: "2026-11-20T00:00:00.0000000", TimeZone: "Tokyo Standard Time"}, "2026-11-20"},
		{outlookWhen{DateTime: "2026-11-19T15:00:00.0000000", TimeZone: "UTC"}, "2026-11-20"},
		{outlookWhen{DateTime: "2026-11-19T14:59:59", TimeZone: "utc"}, "2026-11-19"},
		{outlookWhen{DateTime: "2026-11-20", TimeZone: ""}, "2026-11-20"},
		{outlookWhen{DateTime: "bad", TimeZone: "UTC"}, ""},
		{outlookWhen{DateTime: "2026-13-01T00:00:00", TimeZone: "Tokyo Standard Time"}, ""},
	} {
		if got := mstodoDue(c.w); got != c.want {
			t.Errorf("%+v: %q, want %q", c.w, got, c.want)
		}
	}
	r := mstodoTask{Title: "x", DueDateTime: &outlookWhen{DateTime: "2026-11-19T15:00:00.0000000", TimeZone: "UTC"}}.row()
	if r.Subtitle != "2026-11-20" {
		t.Fatalf("row %+v", r)
	}
}
