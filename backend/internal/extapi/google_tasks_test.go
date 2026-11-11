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

// fakeTasks is Google's token / revoke endpoints and the Tasks API on
// httptest (no live calls).
type fakeTasks struct {
	url string

	mu        sync.Mutex
	challenge string
	revoked   []string
	added     []map[string]any
	patched   []string // list/task
	lastPath  string
	lastQuery url.Values
}

func newFakeTasks(t *testing.T) (*fakeTasks, gcalWire) {
	f := &fakeTasks{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	gerr := func(w http.ResponseWriter, status int, reason string) {
		writeJSON(w, status, map[string]any{"error": map[string]any{"code": status, "errors": []map[string]string{{"reason": reason}}}})
	}
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("client_id") != gcalTestID || r.Form.Get("client_secret") != gcalTestSecret || r.Form.Get("code") != "good-code" ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || !strings.HasSuffix(r.Form.Get("redirect_uri"), "/oauth/callback/googleTasks") {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		writeJSON(w, 200, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600, "scope": gtasksScopeRead})
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
		w.WriteHeader(200)
	})
	api := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
			case "access-1", "wide":
			case "narrow":
				writeJSON(w, 403, map[string]any{"error": map[string]any{"code": 403, "status": "PERMISSION_DENIED",
					"details": []map[string]string{{"reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT"}}}})
				return
			case "forbidden":
				gerr(w, 403, "forbidden")
				return
			case "quota":
				gerr(w, 403, "userRateLimitExceeded")
				return
			case "limited":
				gerr(w, 429, "rateLimitExceeded")
				return
			case "busy":
				gerr(w, 503, "backendError")
				return
			default:
				writeJSON(w, 401, map[string]any{"error": map[string]any{"code": 401, "status": "UNAUTHENTICATED"}})
				return
			}
			if r.PathValue("list") == "missing" || r.PathValue("task") == "missing" {
				gerr(w, 404, "notFound")
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.lastPath, f.lastQuery = r.URL.EscapedPath(), r.URL.Query()
			h(w, r)
		}
	}
	mux.HandleFunc("GET /tasks/v1/users/@me/lists", api(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]string{{"id": "L1", "title": "仕事"}, {"id": "bad id!", "title": ""}}})
	}))
	mux.HandleFunc("GET /tasks/v1/lists/{list}/tasks", api(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"items": []map[string]string{
			{"id": "T1", "title": "牛乳を買う", "notes": "低脂肪\n2本", "status": "needsAction", "due": "2026-11-12T00:00:00.000Z", "webViewLink": "https://tasks.google.com/task/T1"},
			{"id": "T2", "status": "completed", "webViewLink": "javascript:alert(1)"},
		}})
	}))
	mux.HandleFunc("POST /tasks/v1/lists/{list}/tasks", api(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || r.Header.Get("Content-Type") != "application/json" {
			gerr(w, 400, "invalid")
			return
		}
		f.added = append(f.added, b)
		b["id"], b["status"] = "NEW1", "needsAction"
		writeJSON(w, 200, b)
	}))
	mux.HandleFunc("PATCH /tasks/v1/lists/{list}/tasks/{task}", api(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b["status"] != "completed" {
			gerr(w, 400, "invalid")
			return
		}
		f.patched = append(f.patched, r.PathValue("list")+"/"+r.PathValue("task"))
		writeJSON(w, 200, map[string]string{"id": r.PathValue("task"), "title": "牛乳を買う", "status": "completed"})
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, gcalWire{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke",
		APIBase: srv.URL + "/tasks/v1", Client: srv.Client(),
		// 23:30 UTC on the 11th is the 12th in JST
		Now: func() time.Time { return time.Date(2026, 11, 11, 23, 30, 0, 0, time.UTC) }}
}

func gtasksTestClient(t *testing.T) (*googleTasks, *fakeTasks) {
	f, w := newFakeTasks(t)
	g, err := newGoogleTasks(Secrets{ClientID: gcalTestID, ClientSecret: gcalTestSecret}, w)
	if err != nil {
		t.Fatal(err)
	}
	return g, f
}

// connect (PKCE, with Calendar's OAuth client) → lists / tasks → a write
// without its scope is 409 → with it, addTask and complete → revoke.
func TestGoogleTasksFlow(t *testing.T) {
	f, w := newFakeTasks(t)
	old := googleTasksWire
	googleTasksWire = w
	t.Cleanup(func() { googleTasksWire = old })
	c, err := ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_GOOGLE_CALENDAR_CLIENT_ID": gcalTestID, "EXT_GOOGLE_CALENDAR_CLIENT_SECRET": gcalTestSecret}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, opts{cfg: func(cfg *Config) {
		cfg.Enabled[GoogleTasks] = true
		cfg.Secrets = map[string]Secrets{GoogleTasks: c.Secrets[GoogleTasks]}
	}})
	ctx := context.Background()

	auth, err := e.svc.Start(ctx, user, GoogleTasks, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/auth?") || q.Get("scope") != gtasksScopeRead || q.Get("client_id") != gcalTestID ||
		q.Get("access_type") != "offline" || q.Get("redirect_uri") != "https://runner.example/oauth/callback/googleTasks" {
		t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, GoogleTasks, q.Get("state"), "good-code"); back != "tailorit://connected?provider=googleTasks&status=ok" {
		t.Fatalf("callback → %s", back)
	}

	res, err := e.call(e.req(GoogleTasks, "lists", nil))
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Output.Rows; len(r) != 2 || r[0].Title != "仕事" || r[0].Value != "L1" || r[1].Title != gtasksNoTitle || r[1].Value != "" {
		t.Fatalf("lists %+v", r)
	}
	res, err = e.call(e.req(GoogleTasks, "tasks", map[string]string{"listId": "L1", "due": "today"}))
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Output.Rows; len(r) != 2 || r[0].Title != "牛乳を買う" || r[0].Subtitle != "低脂肪" || r[0].Value != "2026-11-12" ||
		r[0].URL != "https://tasks.google.com/task/T1" || r[1].Title != gtasksNoTitle || r[1].Subtitle != gtasksDone || r[1].URL != "" {
		t.Fatalf("tasks %+v", r)
	}
	if fq := f.lastQuery; f.lastPath != "/tasks/v1/lists/L1/tasks" || fq.Get("dueMin") != "2026-11-12T00:00:00Z" ||
		fq.Get("dueMax") != "2026-11-13T00:00:00Z" || fq.Get("showCompleted") != "false" || fq.Get("maxResults") != "20" {
		t.Fatalf("query %s %v", f.lastPath, fq)
	}
	if e.runs() != 2 {
		t.Fatalf("runs = %d", e.runs())
	}

	// the read-only grant cannot write: 409, nothing sent, the run given back
	if _, err := e.call(e.req(GoogleTasks, "addTask", map[string]string{"title": "x"})); !errors.Is(err, ErrNotConnected) || e.runs() != 2 || len(f.added) != 0 {
		t.Fatalf("write without scope: %v", err)
	}
	if err := e.svc.Conns.Save(ctx, user, GoogleTasks, Token{AccessToken: "access-1", RefreshToken: "refresh-1",
		Expiry: e.c.t.Add(time.Hour), Scopes: []string{gtasksScopeWrite}}); err != nil {
		t.Fatal(err)
	}
	res, err = e.call(e.req(GoogleTasks, "addTask", map[string]string{"title": "書類を出す", "notes": "役所", "due": "2026-11-20"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "NEW1" || res.Output.Item.Title != "書類を出す" || res.Output.Item.Value != "2026-11-20" ||
		f.added[0]["due"] != "2026-11-20T00:00:00Z" || f.added[0]["notes"] != "役所" || f.lastPath != "/tasks/v1/lists/@default/tasks" {
		t.Fatalf("addTask %+v %v %s", res.Output, f.added, f.lastPath)
	}
	res, err = e.call(e.req(GoogleTasks, "complete", map[string]string{"taskId": "T1", "listId": "L1"}))
	if err != nil || res.Output.ResultID != "T1" || res.Output.Item.Subtitle != gtasksDone || len(f.patched) != 1 || f.patched[0] != "L1/T1" {
		t.Fatalf("complete %+v %v %v", res.Output, err, f.patched)
	}

	if err := e.svc.Revoke(ctx, user, GoogleTasks); err != nil {
		t.Fatal(err)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "refresh-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

func TestGoogleTasksQueries(t *testing.T) {
	g, f := gtasksTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "wide"}
	cases := []struct {
		args map[string]string
		want map[string]string // "" = not sent
	}{
		{nil, map[string]string{"showCompleted": "false", "showHidden": "", "dueMin": "", "dueMax": "", "maxResults": "20"}},
		{map[string]string{"due": "overdue"}, map[string]string{"dueMin": "", "dueMax": "2026-11-12T00:00:00Z"}},
		{map[string]string{"due": "week", "limit": "50"}, map[string]string{"dueMin": "2026-11-12T00:00:00Z", "dueMax": "2026-11-19T00:00:00Z", "maxResults": "50"}},
		{map[string]string{"due": "2026-12-01"}, map[string]string{"dueMin": "2026-12-01T00:00:00Z", "dueMax": "2026-12-02T00:00:00Z"}},
		{map[string]string{"status": "all"}, map[string]string{"showCompleted": "true", "showHidden": "true", "completedMin": ""}},
		{map[string]string{"status": "completed"}, map[string]string{"showCompleted": "true", "showHidden": "true", "completedMin": "1970-01-01T00:00:00Z"}},
	}
	for i, c := range cases {
		out, err := g.tasks(ctx, Input{Token: tok, Args: c.args})
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		for k, v := range c.want {
			if got := f.lastQuery.Get(k); got != v {
				t.Errorf("case %d: %s = %q, want %q", i, k, got, v)
			}
		}
		// completed only: the open task the fake sends anyway is left out
		if c.args["status"] == "completed" && (len(out.Rows) != 1 || out.Rows[0].Subtitle != gtasksDone) {
			t.Errorf("completed rows %+v", out.Rows)
		}
	}
	if f.lastPath != "/tasks/v1/lists/@default/tasks" {
		t.Fatalf("default list path %s", f.lastPath)
	}
}

func TestGoogleTasksArgs(t *testing.T) {
	p, _ := Lookup(GoogleTasks)
	bad := map[string][]map[string]string{
		"lists": {{"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"listId": "L1"}},
		"tasks": {{"due": "tomorrow"}, {"due": "2026/11/12"}, {"status": "done"}, {"listId": "../x"}, {"listId": strings.Repeat("a", 129)}, {"limit": "100"}},
		"addTask": {{}, {"title": " "}, {"title": strings.Repeat("あ", 1025)}, {"title": "a\x00b"}, {"title": "x", "notes": strings.Repeat("n", 8193)},
			{"title": "x", "due": "2026-11-12T10:00:00+09:00"}, {"title": "x", "listId": "a/b"}},
		"complete": {{}, {"taskId": "a b"}, {"taskId": "T1", "listId": "a?b"}, {"taskId": "T1", "extra": "x"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	good := map[string]map[string]string{
		"lists":    {"limit": "5"},
		"tasks":    {"listId": "@default", "due": "today", "status": "open", "limit": "1"},
		"addTask":  {"title": "x", "notes": "y\nz", "due": "2026-11-12", "listId": "L1"},
		"complete": {"taskId": "T1"},
	}
	for op, a := range good {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
	// the op checks again: nothing reaches Google
	g, f := gtasksTestClient(t)
	tok := &Token{AccessToken: "wide"}
	if _, err := g.addTask(context.Background(), Input{Token: tok, Args: map[string]string{"title": ""}}); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	if _, err := g.complete(context.Background(), Input{Token: tok, Args: map[string]string{"taskId": "a/b"}}); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	if len(f.added) != 0 || len(f.patched) != 0 || f.lastQuery != nil {
		t.Fatal("a bad argument reached Google")
	}
}

// 401 → the connection turns to error; 403 → connect again; 404 → 400;
// 429 / 5xx / rate-limit 403 → 502 upstream_unavailable.
func TestGoogleTasksErrors(t *testing.T) {
	g, _ := gtasksTestClient(t)
	ctx := context.Background()
	list := func(tok string, args map[string]string) error {
		_, err := g.tasks(ctx, Input{Token: &Token{AccessToken: tok}, Args: args})
		return err
	}
	if err := list("dead", nil); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	for _, tok := range []string{"narrow", "forbidden"} {
		if err := list(tok, nil); !errors.Is(err, ErrNotConnected) || Reason(err) != ReasonNotConnected {
			t.Fatalf("403 %s: %v", tok, err)
		}
	}
	for _, tok := range []string{"limited", "busy", "quota"} {
		err := list(tok, nil)
		var pe *ProviderError
		if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	if err := list("wide", map[string]string{"listId": "missing"}); Reason(err) != ReasonBadRequest {
		t.Fatalf("404 list: %v", err)
	}
	_, err := g.complete(ctx, Input{Token: &Token{AccessToken: "wide"}, Args: map[string]string{"taskId": "missing"}})
	if Reason(err) != ReasonBadRequest {
		t.Fatalf("404 task: %v", err)
	}
	if _, err := g.lists(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := g.complete(ctx, Input{Token: &Token{AccessToken: "wide", Scopes: []string{gtasksScopeRead}}, Args: map[string]string{"taskId": "T1"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("read-only complete: %v", err)
	}
	if _, err := g.lists(ctx, Input{Token: &Token{AccessToken: "wide", Scopes: []string{gcalScopeRead}}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("calendar-only token: %v", err)
	}
}

// Off by default, only Google's token and Tasks hosts, writes are writes,
// Calendar's OAuth client unless its own is set, never in the review build.
func TestGoogleTasksDefinition(t *testing.T) {
	p, ok := Lookup(GoogleTasks)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "GOOGLE_TASKS" || p.ClientFrom != GoogleCalendar {
		t.Fatalf("%+v", p)
	}
	w := googleTasksWire
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase + "/users/@me/lists"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, o := range p.Ops {
		write := o.Name == "addTask" || o.Name == "complete"
		want := gtasksScopeRead
		if write {
			want = gtasksScopeWrite
		}
		if o.Write != write || len(o.Scopes) != 1 || o.Scopes[0] != want || o.Check == nil {
			t.Errorf("%s: %+v", o.Name, o)
		}
	}
	g, err := newGoogleTasks(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.g.hc.Get("https://www.googleapis.com/calendar/v3/calendars/primary/events"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil || !strings.Contains(err.Error(), "EXT_GOOGLE_TASKS_CLIENT_ID") {
		t.Fatalf("built without a secret: %v", err)
	}

	env := map[string]string{"EXT_GOOGLE_CALENDAR_CLIENT_ID": "cal-id", "EXT_GOOGLE_CALENDAR_CLIENT_SECRET": "cal-secret"}
	c, _ := ConfigFromEnv(func(k string) string { return env[k] })
	if c.Enabled[GoogleTasks] || c.Secrets[GoogleTasks].ClientID != "cal-id" || c.Secrets[GoogleTasks].ClientSecret != "cal-secret" {
		t.Fatal("not Calendar's client, or on by default")
	}
	env["EXT_GOOGLE_TASKS_CLIENT_ID"], env["EXT_GOOGLE_TASKS_CLIENT_SECRET"] = "tasks-id", "tasks-secret"
	c, _ = ConfigFromEnv(func(k string) string { return env[k] })
	if c.Secrets[GoogleTasks].ClientID != "tasks-id" || c.Secrets[GoogleCalendar].ClientID != "cal-id" {
		t.Fatal("its own client must win")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
