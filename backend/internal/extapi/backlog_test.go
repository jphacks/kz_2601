package extapi

import (
	"context"
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
	backlogTestID     = "bl-client"
	backlogTestSecret = "bl-secret"
	backlogTestSpace  = "acme.backlog.jp"
	backlogTestHost   = "https://" + backlogTestSpace
)

// fakeBacklog は 1 つのスペース(OAuth と API v2)の偽物(httptest・本物は呼ばない)。
type fakeBacklog struct {
	url string

	mu         sync.Mutex
	rotateNext int
	refreshes  int
	paths      []string
	created    []url.Values
}

func newFakeBacklog(t *testing.T) (*fakeBacklog, *httptest.Server) {
	f := &fakeBacklog{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /api/v2/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != backlogTestID || r.Form.Get("client_secret") != backlogTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "bl-code" || r.Form.Get("redirect_uri") == "" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "bl_oauth_0", "refresh_token": "bl_refresh_0", "expires_in": 3600, "token_type": "Bearer"})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "bl_refresh_"+strconv.Itoa(f.rotateNext) {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			writeJSON(w, 200, map[string]any{"access_token": "bl_oauth_" + n, "refresh_token": "bl_refresh_" + n, "expires_in": 3600, "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("/api/v2/", func(w http.ResponseWriter, r *http.Request) {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noperm":
			writeJSON(w, 403, map[string]any{"errors": []map[string]any{{"message": "No permission.", "code": 11}}})
			return
		case tok == "limited":
			w.Header().Set("Retry-After", "30")
			writeJSON(w, 429, map[string]any{"errors": []map[string]any{{"message": "Rate limit exceeded", "code": 13}}})
			return
		case tok == "busy":
			writeJSON(w, 503, nil)
			return
		case !strings.HasPrefix(tok, "bl_oauth_"):
			writeJSON(w, 401, map[string]any{"errors": []map[string]any{{"message": "Authentication failure.", "code": 11}}})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.paths = append(f.paths, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.URL.Path == "/api/v2/users/myself":
			writeJSON(w, 200, map[string]any{"id": 77, "userId": "taro", "name": "太郎"})
		case r.URL.Path == "/api/v2/issues" && r.Method == http.MethodGet:
			writeJSON(w, 200, []map[string]any{
				{"id": 1, "issueKey": "APP-12", "summary": "ログインを直す", "dueDate": "2026-11-20T00:00:00Z", "status": map[string]any{"id": 2, "name": "処理中"}},
				{"id": 2, "issueKey": "APP-3", "summary": "期限なし", "dueDate": nil, "status": nil},
			})
		case r.URL.Path == "/api/v2/issues" && r.Method == http.MethodPost:
			_ = r.ParseForm()
			if r.Form.Get("summary") == "invalid" {
				writeJSON(w, 400, map[string]any{"errors": []map[string]any{{"message": "Invalid Request", "code": 7}}})
				return
			}
			if r.Form.Get("projectId") == "404" {
				writeJSON(w, 404, map[string]any{"errors": []map[string]any{{"message": "No project.", "code": 6}}})
				return
			}
			f.created = append(f.created, r.PostForm)
			n := strconv.Itoa(len(f.created) + 99)
			writeJSON(w, 201, map[string]any{"id": 500, "issueKey": "APP-" + n, "summary": r.Form.Get("summary"), "dueDate": nil, "status": map[string]any{"name": "未対応"}})
		case r.URL.Path == "/api/v2/projects":
			writeJSON(w, 200, []map[string]any{
				{"id": 10, "projectKey": "APP", "name": "アプリ"},
				{"id": 11, "projectKey": "OPS", "name": "運用"},
			})
		case r.URL.Path == "/api/v2/priorities":
			writeJSON(w, 200, []map[string]any{{"id": 2, "name": "高"}, {"id": 3, "name": "中"}, {"id": 4, "name": "低"}})
		default:
			writeJSON(w, 404, map[string]any{"errors": []map[string]any{{"message": "Not found", "code": 5}}})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

// wire は本番と違い、どのスペースのホストも httptest に写す。
func (f *fakeBacklog) wire(srv *httptest.Server) backlogWire {
	return backlogWire{Base: func(string) string { return srv.URL }, Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }}
}

func (f *fakeBacklog) lastPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.paths) == 0 {
		return ""
	}
	return f.paths[len(f.paths)-1]
}

func useBacklog(t *testing.T) (*env, *fakeBacklog) {
	f, srv := newFakeBacklog(t)
	old := backlogWireDefault
	backlogWireDefault = f.wire(srv)
	t.Cleanup(func() { backlogWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Backlog] = true
		c.Secrets = map[string]Secrets{Backlog: {ClientID: backlogTestID, ClientSecret: backlogTestSecret}}
	}})
	return e, f
}

func (e *env) connectBacklog(f *fakeBacklog) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.StartWith(ctx, user, Backlog, nil, map[string]string{"space": "ACME.backlog.jp"})
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/OAuth2AccessRequest.action?") || q.Get("response_type") != "code" || q.Get("client_id") != backlogTestID ||
		q.Get("redirect_uri") != "https://runner.example/oauth/callback/backlog" || q.Get("state") == "" || q.Get("code_challenge") != "" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Backlog, q.Get("state"), "bl-code"); back != "tailorit://connected?provider=backlog&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// space を付けてつなぐ → トークンがホストを持つ → myIssues / projects / addIssue(優先度の
// 既定)→ 期限切れは入れ替わるリフレッシュトークンで更新 → 古いものは error → 切る(取り消しの口は無い)。
func TestBacklogFlow(t *testing.T) {
	e, f := useBacklog(t)
	ctx := context.Background()
	e.connectBacklog(f)
	tok, err := e.svc.Conns.Token(ctx, user, Backlog)
	if err != nil || tok.Host != backlogTestHost || tok.AccessToken != "bl_oauth_0" {
		t.Fatalf("token after connect: %v host=%q", err, tok.Host)
	}

	res, err := e.call(e.req(Backlog, "myIssues", map[string]string{"limit": "10", "statusId": "2"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "ログインを直す", Subtitle: "APP · 2026-11-20 · 処理中", Value: "APP-12", URL: backlogTestHost + "/view/APP-12"}) ||
		rows[1] != (Row{Title: "期限なし", Subtitle: "APP", Value: "APP-3", URL: backlogTestHost + "/view/APP-3"}) {
		t.Fatalf("rows %+v", rows)
	}
	if p := f.lastPath(); p != "GET /api/v2/issues?assigneeId%5B%5D=77&count=10&order=desc&sort=updated&statusId%5B%5D=2" {
		t.Fatalf("path %s", p)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	res, err = e.call(e.req(Backlog, "projects", map[string]string{"limit": "1"}))
	if err != nil || len(res.Output.Rows) != 1 || res.Output.Rows[0] != (Row{Title: "アプリ", Subtitle: "APP", Value: "10", URL: backlogTestHost + "/projects/APP"}) {
		t.Fatalf("projects: %+v %v", res.Output.Rows, err)
	}

	res, err = e.call(e.req(Backlog, "addIssue", map[string]string{"projectId": "10", "summary": "牛乳を買う", "issueTypeId": "5", "description": "2 本\n明日", "dueDate": "2026-11-30"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "APP-100" || res.Output.Item == nil || res.Output.Item.Title != "牛乳を買う" || res.Output.Item.URL != backlogTestHost+"/view/APP-100" {
		t.Fatalf("created %+v", res.Output)
	}
	if in := f.created[0]; in.Get("priorityId") != "3" || in.Get("issueTypeId") != "5" || in.Get("description") != "2 本\n明日" || in.Get("dueDate") != "2026-11-30" {
		t.Fatalf("form %v", in)
	}
	if _, err := e.call(e.req(Backlog, "addIssue", map[string]string{"projectId": "10", "summary": "x", "issueTypeId": "5", "priorityId": "2"})); err != nil || f.created[1].Get("priorityId") != "2" {
		t.Fatalf("priority given: %v %v", err, f.created)
	}
	// Backlog が受けない入力・無いプロジェクトは 400
	if _, err := e.call(e.req(Backlog, "addIssue", map[string]string{"projectId": "10", "summary": "invalid", "issueTypeId": "5"})); Reason(err) != ReasonBadRequest {
		t.Fatalf("invalid: %v", err)
	}
	if _, err := e.call(e.req(Backlog, "addIssue", map[string]string{"projectId": "404", "summary": "x", "issueTypeId": "5"})); Reason(err) != ReasonBadRequest {
		t.Fatalf("no project: %v", err)
	}

	// 期限切れ → 入れ替わったリフレッシュトークンで更新(ホストは残る)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Backlog, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Backlog, "projects", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Backlog)
	if err != nil || tok.AccessToken != "bl_oauth_1" || tok.RefreshToken != "bl_refresh_1" || tok.Host != backlogTestHost || f.refreshes != 1 {
		t.Fatalf("refreshed: %v %+v refreshes=%d", err, tok, f.refreshes)
	}

	// 古い(入れ替わった)リフレッシュトークンは invalid_grant → error
	if err := e.svc.Conns.Save(ctx, user, Backlog, Token{AccessToken: "bl_oauth_0", RefreshToken: "bl_refresh_0", Host: backlogTestHost, Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Backlog, "projects", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Backlog); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// つなぎ直して切る: 取り消しの口は無く、保存したトークンが消えるだけ
	e.connectBacklog(f)
	if err := e.svc.Revoke(ctx, user, Backlog); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Backlog); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
}

// space が無い・形が違う・知らない引数は 400 で、state は作られない。
func TestBacklogStartArgs(t *testing.T) {
	e, _ := useBacklog(t)
	ctx := context.Background()
	for _, args := range []map[string]string{nil, {}, {"space": "acme"}, {"space": "acme.backlog.org"}, {"space": "a.b.backlog.jp"}, {"space": "acme.backlog.jp/x"}, {"space": "acme.backlog.jp", "team": "x"}} {
		if _, err := e.svc.StartWith(ctx, user, Backlog, nil, args); !errors.Is(err, ErrUnknown) {
			t.Errorf("%v: %v", args, err)
		}
	}
	if len(e.mem.states) != 0 {
		t.Fatalf("states %d", len(e.mem.states))
	}
	// ConnectArgs の無い provider は引数を取らない
	if _, err := e.svc.StartWith(ctx, user, FakeOAuth, nil, map[string]string{"space": "x"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("fake with args: %v", err)
	}
	if h, err := backlogHost(map[string]string{"space": " https://Acme.backlog.com "}); err != nil || h != "https://acme.backlog.com" {
		t.Fatalf("host %q %v", h, err)
	}
}

func backlogTestClient(t *testing.T) (*backlog, *fakeBacklog) {
	f, srv := newFakeBacklog(t)
	c, err := newBacklog(Secrets{ClientID: backlogTestID, ClientSecret: backlogTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// 403 → つなぎ直し、401 → 接続は error、429 → やり直し可の 429、5xx → upstream、
// ホストの無いトークンはつなぎ直し。
func TestBacklogErrors(t *testing.T) {
	c, _ := backlogTestClient(t)
	ctx := context.Background()
	in := func(tok string) Input { return Input{Token: &Token{AccessToken: tok, Host: backlogTestHost}} }
	if _, err := c.projects(ctx, in("noperm")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	if _, err := c.projects(ctx, in("gone")); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	_, err := c.myIssues(ctx, in("limited"))
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != 429 || !pe.Retryable || Reason(err) != ReasonUpstream || !upstreamLimited(err) {
		t.Fatalf("429: %v", err)
	}
	if _, err := c.projects(ctx, in("busy")); !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := c.projects(ctx, Input{Token: &Token{AccessToken: "bl_oauth_0"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no host: %v", err)
	}
	if _, err := c.projects(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "bl_oauth_0", RefreshToken: "bl_refresh_0"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a host: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "bl_oauth_0", RefreshToken: "old", Host: backlogTestHost}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refused refresh: %v", err)
	}
	if _, err := c.ExchangeOn(ctx, backlogTestHost, "bad", "", "https://runner.example/oauth/callback/backlog"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	if _, err := c.Exchange(ctx, "bl-code", "", "https://runner.example/oauth/callback/backlog"); err == nil {
		t.Fatal("exchange without a host")
	}
	if err := c.Revoke(ctx, Token{AccessToken: "x"}); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
}

// トークン口の通信失敗は共通の upstream(やり直し可・502)で、秘密を文に出さない。
func TestBacklogTokenTransportError(t *testing.T) {
	f, srv := newFakeBacklog(t)
	c, err := newBacklog(Secrets{ClientID: backlogTestID, ClientSecret: backlogTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	_, err = c.ExchangeOn(context.Background(), backlogTestHost, "bl-code", "", "https://runner.example/oauth/callback/backlog")
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream || StatusOf(Reason(err)) != http.StatusBadGateway {
		t.Fatalf("transport: %v", err)
	}
	if strings.Contains(err.Error(), backlogTestSecret) || strings.Contains(err.Error(), "bl-code") {
		t.Fatalf("secret in the error: %v", err)
	}
}

func TestBacklogCheck(t *testing.T) {
	p, _ := Lookup(Backlog)
	bad := map[string][]map[string]string{
		"myIssues": {{"limit": "0"}, {"limit": "101"}, {"statusId": "x"}, {"statusId": "0"}, {"assigneeId": "1"}},
		"projects": {{"limit": "x"}},
		"addIssue": {
			{}, {"projectId": "1"}, {"projectId": "1", "summary": "x"}, {"projectId": "x", "summary": "x", "issueTypeId": "1"},
			{"projectId": "1", "summary": " ", "issueTypeId": "1"}, {"projectId": "1", "summary": "a\nb", "issueTypeId": "1"},
			{"projectId": "1", "summary": strings.Repeat("あ", 256), "issueTypeId": "1"},
			{"projectId": "1", "summary": "x", "issueTypeId": "1", "priorityId": "high"},
			{"projectId": "1", "summary": "x", "issueTypeId": "1", "dueDate": "2026/11/30"},
			{"projectId": "1", "summary": "x", "issueTypeId": "1", "dueDate": "2026-02-30"},
			{"projectId": "1", "summary": "x", "issueTypeId": "1", "description": "a\x00"},
		},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{
		"myIssues": {"limit": "100", "statusId": "4"},
		"projects": {},
		"addIssue": {"projectId": "10", "summary": "牛乳を買う", "issueTypeId": "5", "priorityId": "3", "description": "1 行目\n\t2 行目", "dueDate": "2026-11-30"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// addIssue だけが書く・scope は無い・space を取る・出口はスペースのホストだけ・
// 既定 off・審査用の版に入らない。
func TestBacklogDefinition(t *testing.T) {
	p, ok := Lookup(Backlog)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "BACKLOG" || p.ConnectHost == nil || len(p.ConnectArgs) != 1 || p.ConnectArgs[0] != "space" {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write != (o.Name == "addIssue") || len(o.Scopes) != 0 {
			t.Errorf("%s: write = %v scopes = %v", o.Name, o.Write, o.Scopes)
		}
	}
	c, err := newBacklog(Secrets{ClientID: "a", ClientSecret: "b"}, backlogWireDefault)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := any(c).(HostedOAuthClient); !ok {
		t.Fatal("not a HostedOAuthClient")
	}
	for _, raw := range []string{backlogTestHost + "/api/v2/oauth2/token", "https://acme.backlog.com/api/v2/issues"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, raw := range []string{"https://backlog.jp/", "https://a.b.backlog.jp/", "https://acme.backlog.jp:8443/", "http://acme.backlog.jp/", "https://acme.backlog.jp.evil.example/", "https://acmebacklog.jp/", "https://nulab.com/"} {
		u, _ := url.Parse(raw)
		if AllowedURL(p.Hosts, u) {
			t.Errorf("%s on the allowlist", raw)
		}
		if _, err := c.hc.Get(raw); !errors.Is(err, ErrEgress) {
			t.Errorf("%s: %v", raw, err)
		}
	}
	if _, err := p.New(Secrets{ClientSecret: "b"}); err == nil {
		t.Fatal("built without a client id")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[Backlog] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}

// Token.Host は暗号化して保存され、読み戻せる。
func TestBacklogTokenHostSealed(t *testing.T) {
	e, _ := useBacklog(t)
	ctx := context.Background()
	in := Token{AccessToken: "a", RefreshToken: "r", Host: backlogTestHost, Expiry: e.c.t.Add(time.Hour)}
	if err := e.svc.Conns.Save(ctx, user, Backlog, in); err != nil {
		t.Fatal(err)
	}
	out, err := e.svc.Conns.Token(ctx, user, Backlog)
	if err != nil || out.Host != backlogTestHost || out.AccessToken != "a" {
		t.Fatalf("%+v %v", out, err)
	}
	if s := out.String(); strings.Contains(s, backlogTestSpace) {
		t.Fatalf("host in String: %s", s)
	}
}
