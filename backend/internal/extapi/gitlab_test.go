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
	gitlabTestID     = "gl-client"
	gitlabTestSecret = "gl-secret"
)

// fakeGitLab は gitlab.com の OAuth と API v4 の偽物(httptest・本物は呼ばない)。
type fakeGitLab struct {
	url string

	mu         sync.Mutex
	challenge  string
	rotateNext int
	refreshes  int
	revoked    []string
	paths      []string
}

func newFakeGitLab(t *testing.T) (*fakeGitLab, *httptest.Server) {
	f := &fakeGitLab{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != gitlabTestID || r.Form.Get("client_secret") != gitlabTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "gl-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("redirect_uri") == "" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "gl_oauth_0", "refresh_token": "gl_refresh_0", "expires_in": 7200, "scope": "read_api", "token_type": "Bearer", "created_at": 1762862400})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "gl_refresh_"+strconv.Itoa(f.rotateNext) {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			writeJSON(w, 200, map[string]any{"access_token": "gl_oauth_" + n, "refresh_token": "gl_refresh_" + n, "expires_in": 7200, "scope": "read_api", "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != gitlabTestID || r.Form.Get("client_secret") != gitlabTestSecret {
			w.WriteHeader(401)
			return
		}
		if !strings.HasPrefix(r.Form.Get("token"), "gl_") {
			w.WriteHeader(400) // 取り消し済み
			return
		}
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{})
	})
	mux.HandleFunc("GET /api/v4/", func(w http.ResponseWriter, r *http.Request) {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			writeJSON(w, 403, map[string]string{"error": "insufficient_scope"})
			return
		case tok == "limited":
			w.Header().Set("Retry-After", "30")
			writeJSON(w, 429, map[string]string{"message": "Rate limit exceeded"})
			return
		case tok == "busy":
			writeJSON(w, 503, nil)
			return
		case !strings.HasPrefix(tok, "gl_oauth_"):
			writeJSON(w, 401, map[string]string{"message": "401 Unauthorized"})
			return
		}
		if r.Header.Get("User-Agent") == "" {
			writeJSON(w, 400, map[string]string{"message": "no user agent"})
			return
		}
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.RequestURI())
		f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/v4/issues":
			writeJSON(w, 200, []map[string]any{
				{"title": "ログインを直す", "state": "opened", "due_date": "2026-11-20", "updated_at": "2026-11-12T01:02:03.000Z", "web_url": "https://gitlab.com/acme/app/-/issues/12", "references": map[string]any{"full": "acme/app#12"}},
				{"title": "期限なし", "state": "opened", "due_date": nil, "web_url": "https://evil.example/x", "references": map[string]any{"full": ""}},
			})
		case r.URL.Path == "/api/v4/merge_requests":
			writeJSON(w, 200, []map[string]any{
				{"title": "feat: 検索", "state": "merged", "updated_at": "2026-11-11T00:00:00.000Z", "web_url": "https://gitlab.com/acme/app/-/merge_requests/3", "references": map[string]any{"full": "acme/app!3"}},
			})
		case r.URL.Path == "/api/v4/projects/123/pipelines", r.URL.Path == "/api/v4/projects/acme%2Fapp/pipelines", r.URL.RawPath == "/api/v4/projects/acme%2Fapp/pipelines":
			writeJSON(w, 200, []map[string]any{
				{"id": 9001, "ref": "main", "status": "success", "created_at": "2026-11-12T03:00:00.000Z", "web_url": "https://gitlab.com/acme/app/-/pipelines/9001"},
				{"id": 9000, "ref": "feat/x", "status": "failed", "created_at": "2026-11-12T02:00:00.000Z", "web_url": "https://gitlab.com/acme/app/-/pipelines/9000"},
			})
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/"):
			writeJSON(w, 404, map[string]string{"message": "404 Project Not Found"})
		default:
			writeJSON(w, 404, map[string]string{"message": "404 Not Found"})
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeGitLab) wire(srv *httptest.Server) gitlabWire {
	return gitlabWire{AuthURL: srv.URL + "/oauth/authorize", TokenURL: srv.URL + "/oauth/token", RevokeURL: srv.URL + "/oauth/revoke",
		APIBase: srv.URL + "/api/v4", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }}
}

func (f *fakeGitLab) lastPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.paths) == 0 {
		return ""
	}
	return f.paths[len(f.paths)-1]
}

func useGitLab(t *testing.T) (*env, *fakeGitLab) {
	f, srv := newFakeGitLab(t)
	old := gitlabWireDefault
	gitlabWireDefault = f.wire(srv)
	t.Cleanup(func() { gitlabWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[GitLab] = true
		c.Secrets = map[string]Secrets{GitLab: {ClientID: gitlabTestID, ClientSecret: gitlabTestSecret}}
	}})
	return e, f
}

func (e *env) connectGitLab(f *fakeGitLab) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, GitLab, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth/authorize?") || q.Get("scope") != "read_api" || q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" ||
		q.Get("client_id") != gitlabTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/gitlab" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, GitLab, q.Get("state"), "gl-code"); back != "tailorit://connected?provider=gitlab&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// つなぐ(PKCE・read_api)→ myIssues → myMergeRequests → pipelines → 期限切れは入れ替わる
// リフレッシュトークンで更新 → 古いリフレッシュトークンは error → 切る(revoke)。
func TestGitLabFlow(t *testing.T) {
	e, f := useGitLab(t)
	ctx := context.Background()
	e.connectGitLab(f)

	res, err := e.call(e.req(GitLab, "myIssues", map[string]string{"limit": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "ログインを直す", Subtitle: "acme/app#12 · opened", Value: "2026-11-20", URL: "https://gitlab.com/acme/app/-/issues/12"}) ||
		rows[1] != (Row{Title: "期限なし", Subtitle: "opened"}) {
		t.Fatalf("rows %+v", rows)
	}
	if p := f.lastPath(); p != "/api/v4/issues?order_by=updated_at&per_page=10&scope=assigned_to_me&sort=desc&state=opened" {
		t.Fatalf("path %s", p)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	res, err = e.call(e.req(GitLab, "myMergeRequests", map[string]string{"state": "merged"}))
	if err != nil || len(res.Output.Rows) != 1 || res.Output.Rows[0] != (Row{Title: "feat: 検索", Subtitle: "acme/app!3 · merged", Value: "2026-11-11T00:00:00.000Z", URL: "https://gitlab.com/acme/app/-/merge_requests/3"}) {
		t.Fatalf("mrs: %+v %v", res.Output.Rows, err)
	}
	if p := f.lastPath(); p != "/api/v4/merge_requests?order_by=updated_at&per_page=20&scope=created_by_me&sort=desc&state=merged" {
		t.Fatalf("path %s", p)
	}

	res, err = e.call(e.req(GitLab, "pipelines", map[string]string{"projectId": "acme/app", "limit": "2"}))
	if err != nil || len(res.Output.Rows) != 2 || res.Output.Rows[0] != (Row{Title: "main", Subtitle: "success · 2026-11-12T03:00:00.000Z", Value: "9001", URL: "https://gitlab.com/acme/app/-/pipelines/9001"}) {
		t.Fatalf("pipelines: %+v %v", res.Output.Rows, err)
	}
	if p := f.lastPath(); p != "/api/v4/projects/acme%2Fapp/pipelines?order_by=id&per_page=2&sort=desc" {
		t.Fatalf("path %s", p)
	}
	if _, err := e.call(e.req(GitLab, "pipelines", map[string]string{"projectId": "123"})); err != nil {
		t.Fatal(err)
	}
	// 無いプロジェクトは 400(呼んだ後なので runs は戻らない)
	if _, err := e.call(e.req(GitLab, "pipelines", map[string]string{"projectId": "999"})); Reason(err) != ReasonBadRequest {
		t.Fatalf("no such project: %v", err)
	}

	// 期限切れ → 入れ替わったリフレッシュトークンで更新
	tok, _ := e.svc.Conns.Token(ctx, user, GitLab)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, GitLab, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GitLab, "myIssues", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, GitLab)
	if err != nil || tok.AccessToken != "gl_oauth_1" || tok.RefreshToken != "gl_refresh_1" || f.refreshes != 1 || len(tok.Scopes) != 1 {
		t.Fatalf("refreshed: %v refreshes=%d scopes=%v", err, f.refreshes, tok.Scopes)
	}

	// 古い(入れ替わった)リフレッシュトークンは invalid_grant → error
	if err := e.svc.Conns.Save(ctx, user, GitLab, Token{AccessToken: "gl_oauth_0", RefreshToken: "gl_refresh_0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GitLab, "myIssues", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, GitLab); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// つなぎ直して切る: リフレッシュトークンを GitLab で取り消す
	e.connectGitLab(f)
	if err := e.svc.Revoke(ctx, user, GitLab); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, GitLab); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "gl_refresh_0" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

func gitlabTestClient(t *testing.T) (*gitlab, *fakeGitLab) {
	f, srv := newFakeGitLab(t)
	c, err := newGitLab(Secrets{ClientID: gitlabTestID, ClientSecret: gitlabTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// 403 → つなぎ直し、401 → 接続は error、429 → やり直し可の 429(provider を止める)、5xx → upstream。
func TestGitLabErrors(t *testing.T) {
	c, _ := gitlabTestClient(t)
	ctx := context.Background()
	in := func(tok string) Input { return Input{Token: &Token{AccessToken: tok}} }
	if _, err := c.myIssues(ctx, in("noscope")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	if _, err := c.myIssues(ctx, in("gone")); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	_, err := c.myIssues(ctx, in("limited"))
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != 429 || !pe.Retryable || Reason(err) != ReasonUpstream || !upstreamLimited(err) {
		t.Fatalf("429: %v", err)
	}
	if _, err := c.myMergeRequests(ctx, in("busy")); !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := c.myIssues(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "gl_oauth_0"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a refresh token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "gl_oauth_0", RefreshToken: "old"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refused refresh: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/gitlab"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	// 取り消し済み(400)は済んだもの。トークンが無ければ何もしない
	if err := c.Revoke(ctx, Token{AccessToken: "x"}); err != nil {
		t.Fatalf("400: %v", err)
	}
	if err := c.Revoke(ctx, Token{}); err != nil {
		t.Fatal(err)
	}
}

func TestGitLabCheck(t *testing.T) {
	p, _ := Lookup(GitLab)
	bad := map[string][]map[string]string{
		"myIssues":        {{"state": "merged"}, {"state": "Opened"}, {"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"scope": "all"}},
		"myMergeRequests": {{"state": "done"}, {"limit": "-1"}},
		"pipelines": {
			{}, {"projectId": ""}, {"projectId": "acme"}, {"projectId": "acme/../x"}, {"projectId": "acme/app?x=1"},
			{"projectId": "https://gitlab.com/acme/app"}, {"projectId": "1234567890123"}, {"projectId": "123", "limit": "99"},
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
		"myIssues":        {"state": "closed", "limit": "50"},
		"myMergeRequests": {"state": "locked"},
		"pipelines":       {"projectId": "acme/sub/app", "limit": "1"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// 読む op だけ(書く op は無い)・scope は read_api だけ・本物の口は許可リストにある・
// 既定 off・審査用の版に入らない。
func TestGitLabDefinition(t *testing.T) {
	p, ok := Lookup(GitLab)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "GITLAB" {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write || len(o.Scopes) != 1 || o.Scopes[0] != gitlabScopeRead {
			t.Errorf("%s: write=%v scopes=%v", o.Name, o.Write, o.Scopes)
		}
	}
	w := gitlabWireDefault
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	c, err := newGitLab(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.hc.Get("https://api.gitlab.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientSecret: "b"}); err == nil {
		t.Fatal("built without a client id")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[GitLab] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
