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
	githubTestID     = "Iv23-gh-client"
	githubTestSecret = "gh-secret"
)

// fakeGitHub is GitHub's OAuth (GitHub App user tokens), REST search and
// GraphQL on httptest (no live calls).
type fakeGitHub struct {
	url string

	mu         sync.Mutex
	challenge  string
	rotateNext int
	refreshes  int
	revoked    []string
	lastQuery  url.Values
	lastVars   map[string]any
	headers    http.Header
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *httptest.Server) {
	f := &fakeGitHub{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	// GitHub answers a refused code / refresh token with 200 and {"error"}
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != githubTestID || r.Form.Get("client_secret") != githubTestSecret || r.Header.Get("Accept") != "application/json" {
			writeJSON(w, 200, map[string]string{"error": "incorrect_client_credentials"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Form.Get("grant_type") == "refresh_token" {
			if r.Form.Get("refresh_token") != "ghr_"+strconv.Itoa(f.rotateNext) {
				writeJSON(w, 200, map[string]string{"error": "bad_refresh_token"})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			writeJSON(w, 200, map[string]any{"access_token": "ghu_" + n, "refresh_token": "ghr_" + n, "expires_in": 28800, "refresh_token_expires_in": 15897600, "scope": "", "token_type": "bearer"})
			return
		}
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("code") != "gh-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
			writeJSON(w, 200, map[string]string{"error": "bad_verification_code"})
			return
		}
		writeJSON(w, 200, map[string]any{"access_token": "ghu_0", "refresh_token": "ghr_0", "expires_in": 28800, "refresh_token_expires_in": 15897600, "scope": "", "token_type": "bearer"})
	})
	mux.HandleFunc("DELETE /applications/{client}/grant", func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != githubTestID || secret != githubTestSecret || r.PathValue("client") != githubTestID {
			w.WriteHeader(401)
			return
		}
		var b struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		if !strings.HasPrefix(b.AccessToken, "ghu_") {
			writeJSON(w, 422, map[string]string{"message": "Validation Failed"})
			return
		}
		f.mu.Lock()
		f.revoked = append(f.revoked, b.AccessToken)
		f.mu.Unlock()
		w.WriteHeader(204)
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.headers = r.Header.Clone()
		f.mu.Unlock()
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noperm":
			writeJSON(w, 403, map[string]any{"message": "Resource not accessible by integration"})
			return false
		case tok == "limited":
			w.Header().Set("X-RateLimit-Remaining", "0")
			writeJSON(w, 403, map[string]any{"message": "API rate limit exceeded"})
			return false
		case tok == "secondary":
			w.Header().Set("Retry-After", "60")
			writeJSON(w, 403, map[string]any{"message": "You have exceeded a secondary rate limit"})
			return false
		case tok == "toomany":
			writeJSON(w, 429, map[string]any{"message": "slow down"})
			return false
		case tok == "busy":
			writeJSON(w, 503, nil)
			return false
		case !strings.HasPrefix(tok, "ghu_"):
			writeJSON(w, 401, map[string]any{"message": "Bad credentials"})
			return false
		}
		return true
	}
	mux.HandleFunc("GET /search/issues", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		q := r.URL.Query()
		f.mu.Lock()
		f.lastQuery = q
		f.mu.Unlock()
		if !strings.Contains(q.Get("q"), "is:pr") && !strings.Contains(q.Get("q"), "is:issue") {
			writeJSON(w, 422, map[string]any{"message": "Validation Failed"}) // a GitHub App's user token needs is:pr / is:issue
			return
		}
		writeJSON(w, 200, map[string]any{"total_count": 42, "incomplete_results": false, "items": []map[string]any{
			{"title": "Fix the login", "number": 12, "html_url": "https://github.com/acme/app/pull/12", "repository_url": f.url + "/repos/acme/app", "updated_at": "2026-11-10T09:00:00Z"},
			{"title": "evil", "number": 3, "html_url": "https://evil.example/x", "repository_url": "https://elsewhere/repos/x/y", "updated_at": "2026-11-09T09:00:00Z"},
		}})
	})
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		var b struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		f.mu.Lock()
		f.lastVars = b.Variables
		f.mu.Unlock()
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ghu_gqllimit") {
			writeJSON(w, 200, map[string]any{"errors": []map[string]any{{"type": "RATE_LIMITED", "message": "API rate limit exceeded"}}})
			return
		}
		day := func(d string, n int) map[string]any { return map[string]any{"date": d, "contributionCount": n} }
		writeJSON(w, 200, map[string]any{"data": map[string]any{"viewer": map[string]any{"contributionsCollection": map[string]any{
			"contributionCalendar": map[string]any{"totalContributions": 9, "weeks": []map[string]any{
				{"contributionDays": []any{day("2026-11-08", 0), day("2026-11-09", 4)}},
				{"contributionDays": []any{day("2026-11-10", 2), day("2026-11-11", 3), day("2026-11-12", 7)}},
			}},
		}}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeGitHub) wire(srv *httptest.Server) githubWire {
	return githubWire{AuthURL: srv.URL + "/login/oauth/authorize", TokenURL: srv.URL + "/login/oauth/access_token",
		APIBase: srv.URL, Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }}
}

func useGitHub(t *testing.T) (*env, *fakeGitHub) {
	f, srv := newFakeGitHub(t)
	old := githubWireDefault
	githubWireDefault = f.wire(srv)
	t.Cleanup(func() { githubWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[GitHub] = true
		c.Secrets = map[string]Secrets{GitHub: {ClientID: githubTestID, ClientSecret: githubTestSecret}}
	}})
	return e, f
}

func (e *env) connectGitHub(f *fakeGitHub) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, GitHub, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/login/oauth/authorize?") || q.Get("code_challenge_method") != "S256" || q.Get("scope") != "" ||
		q.Get("client_id") != githubTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/github" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, GitHub, q.Get("state"), "gh-code"); back != "tailorit://connected?provider=github&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE, no scopes) → the three reads → refresh (rotated) → a
// refused refresh needs a reconnect → revoke.
func TestGitHubFlow(t *testing.T) {
	e, f := useGitHub(t)
	ctx := context.Background()
	e.connectGitHub(f)

	res, err := e.call(e.req(GitHub, "pullRequests", nil))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "Fix the login", Subtitle: "acme/app#12", Value: "2026-11-10T09:00:00Z", URL: "https://github.com/acme/app/pull/12"}) ||
		rows[1].URL != "" || rows[1].Subtitle != "" || res.Output.Item.Value != "42" {
		t.Fatalf("rows %+v item %+v", rows, res.Output.Item)
	}
	if q := f.lastQuery; q.Get("q") != "is:pr review-requested:@me is:open archived:false" || q.Get("sort") != "updated" || q.Get("per_page") != "20" {
		t.Fatalf("query %v", q)
	}
	h := f.headers
	if h.Get("Authorization") != "Bearer ghu_0" || h.Get("X-GitHub-Api-Version") != githubAPIVersion || h.Get("User-Agent") == "" || h.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("headers %v", h)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	if _, err := e.call(e.req(GitHub, "issues", map[string]string{"state": "closed", "limit": "5"})); err != nil {
		t.Fatal(err)
	}
	if q := f.lastQuery; q.Get("q") != "is:issue assignee:@me is:closed archived:false" || q.Get("per_page") != "5" {
		t.Fatalf("query %v", q)
	}

	res, err = e.call(e.req(GitHub, "contributions", map[string]string{"from": "2026-11-09", "to": "2026-11-11"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Output.Rows; len(got) != 3 || got[0] != (Row{Title: "2026-11-09", Value: "4"}) || got[2] != (Row{Title: "2026-11-11", Value: "3"}) {
		t.Fatalf("days %+v", got)
	}
	if it := res.Output.Item; it.Value != "9" || it.Title != "2026-11-09〜2026-11-11" {
		t.Fatalf("total %+v", it)
	}
	if f.lastVars["from"] != "2026-11-09T00:00:00+09:00" || f.lastVars["to"] != "2026-11-11T23:59:59+09:00" {
		t.Fatalf("vars %v", f.lastVars)
	}

	// expired → refreshed with the rotated refresh token
	tok, _ := e.svc.Conns.Token(ctx, user, GitHub)
	if tok.Expiry.IsZero() {
		t.Fatal("an expiring user token has no expiry")
	}
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, GitHub, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GitHub, "issues", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, GitHub)
	if err != nil || tok.AccessToken != "ghu_1" || tok.RefreshToken != "ghr_1" || f.refreshes != 1 {
		t.Fatalf("refreshed: %v refreshes=%d", err, f.refreshes)
	}

	// an old (rotated away) refresh token is bad_refresh_token (200) → error
	if err := e.svc.Conns.Save(ctx, user, GitHub, Token{AccessToken: "ghu_0", RefreshToken: "ghr_0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GitHub, "issues", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("bad_refresh_token: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, GitHub); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect and cut: the grant is deleted at GitHub
	e.connectGitHub(f)
	if err := e.svc.Revoke(ctx, user, GitHub); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, GitHub); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "ghu_0" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

func githubTestClient(t *testing.T) (*github, *fakeGitHub) {
	f, srv := newFakeGitHub(t)
	c, err := newGitHub(Secrets{ClientID: githubTestID, ClientSecret: githubTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// 403 → connect again (the app's permission), 401 → the connection is an
// error, a rate-limited 403 / 429 / GraphQL RATE_LIMITED → a retryable 429
// (pauses the provider), 5xx → upstream, 422 → bad request.
func TestGitHubErrors(t *testing.T) {
	c, _ := githubTestClient(t)
	ctx := context.Background()
	in := func(tok string) Input { return Input{Token: &Token{AccessToken: tok}} }
	if _, err := c.issues(ctx, in("noperm")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	if _, err := c.issues(ctx, in("dead")); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	for _, tok := range []string{"limited", "secondary", "toomany", "ghu_gqllimit"} {
		var err error
		if tok == "ghu_gqllimit" {
			_, err = c.contributions(ctx, in(tok))
		} else {
			_, err = c.issues(ctx, in(tok))
		}
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Status != 429 || !pe.Retryable || Reason(err) != ReasonUpstream || !upstreamLimited(err) {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	_, err := c.pullRequests(ctx, in("busy"))
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := c.search(ctx, in("ghu_0"), "author:@me", 5); Reason(err) != ReasonBadRequest {
		t.Fatalf("422: %v", err)
	}
	if _, err := c.issues(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	// a token without a refresh token (expiring tokens off) cannot be renewed
	if _, err := c.Refresh(ctx, Token{AccessToken: "ghu_x"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/github"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	// revoking an expired token renews it first; a token GitHub does not
	// know (422) and a refresh token already gone are done
	if err := c.Revoke(ctx, Token{AccessToken: "ghu_0", RefreshToken: "ghr_0", Expiry: c.now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "gone"}); err != nil {
		t.Fatalf("422: %v", err)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "x", RefreshToken: "gone", Expiry: c.now().Add(-time.Hour)}); err != nil {
		t.Fatalf("gone: %v", err)
	}
}

// Defaults: pullRequests waits for my review (open); contributions is the
// last 7 days to today (JST).
func TestGitHubDefaults(t *testing.T) {
	c, f := githubTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "ghu_0"}
	if _, err := c.pullRequests(ctx, Input{Token: tok, Args: map[string]string{"role": "author", "state": "merged"}}); err != nil {
		t.Fatal(err)
	}
	if q := f.lastQuery.Get("q"); q != "is:pr author:@me is:merged archived:false" {
		t.Fatalf("q %s", q)
	}
	if _, err := c.pullRequests(ctx, Input{Token: tok, Args: map[string]string{"state": "all"}}); err != nil {
		t.Fatal(err)
	}
	if q := f.lastQuery.Get("q"); q != "is:pr review-requested:@me archived:false" {
		t.Fatalf("q %s", q)
	}
	out, err := c.contributions(ctx, Input{Token: tok})
	if err != nil {
		t.Fatal(err)
	}
	// now is 2026-11-11 21:00 JST: 11-05..11-11 (the fake's 11-12 is cut)
	if f.lastVars["from"] != "2026-11-05T00:00:00+09:00" || len(out.Rows) != 4 || out.Rows[3].Title != "2026-11-11" {
		t.Fatalf("vars %v rows %+v", f.lastVars, out.Rows)
	}
}

func TestGitHubCheck(t *testing.T) {
	p, _ := Lookup(GitHub)
	bad := map[string][]map[string]string{
		"contributions": {
			{"from": "11/09"}, {"to": "2026-13-01"}, {"to": "2999-01-01"},
			{"from": "2026-11-10", "to": "2026-11-01"},
			{"from": "2024-01-01", "to": "2025-06-01"},
			{"from": "2007-12-31", "to": "2008-01-02"},
			{"login": "octocat"},
		},
		"pullRequests": {{"role": "assignee"}, {"state": "draft"}, {"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"q": "repo:x/y"}},
		"issues":       {{"state": "merged"}, {"limit": "100"}, {"role": "author"}},
	}
	for op, cases := range bad {
		for i, a := range cases {
			if _, err := p.CheckArgs(op, a); !errors.Is(err, ErrUnknown) {
				t.Errorf("%s case %d: %v", op, i, err)
			}
		}
	}
	for op, a := range map[string]map[string]string{
		"contributions": {"from": "2025-11-01", "to": "2026-10-31"},
		"pullRequests":  {"role": "author", "state": "merged", "limit": "50"},
		"issues":        {"state": "all"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// Read only, no scopes; the real endpoints are on the allowlist and
// nothing else; off by default and never in the review build.
func TestGitHubDefinition(t *testing.T) {
	p, ok := Lookup(GitHub)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "GITHUB" {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write || len(o.Scopes) != 0 {
			t.Errorf("%s: GitHub is read only and a GitHub App has no scopes", o.Name)
		}
	}
	w := githubWireDefault
	for _, raw := range []string{w.TokenURL, w.APIBase + "/search/issues", w.APIBase + "/graphql", w.APIBase + "/applications/x/grant"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	c, err := newGitHub(Secrets{ClientID: "a", ClientSecret: "b"}, w)
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
	if cfg.Enabled[GitHub] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
