package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const (
	vercelTestID     = "oac_client"
	vercelTestSecret = "vc-secret"
	vercelTestSlug   = "tailorit-deploys"
)

// fakeVercel is Vercel's integration OAuth and REST API on httptest.
type fakeVercel struct {
	mu        sync.Mutex
	teamID    any // the token answer's team_id (nil = a personal account)
	lastPath  string
	lastQuery url.Values
}

func newFakeVercel(t *testing.T) (*fakeVercel, *httptest.Server) {
	f := &fakeVercel{teamID: "team_abc123"}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /v2/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != vercelTestID || r.Form.Get("client_secret") != vercelTestSecret ||
			r.Form.Get("code") != "vc-code" || r.Form.Get("redirect_uri") != "https://runner.example/oauth/callback/vercel" {
			writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"access_token": "vc-access", "token_type": "Bearer", "installation_id": "icfg_1", "user_id": "u1", "team_id": f.teamID})
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.lastPath, f.lastQuery = r.URL.Path, r.URL.Query()
		f.mu.Unlock()
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); tok {
		case "vc-access":
			return true
		case "noaccess":
			writeJSON(w, 403, map[string]any{"error": map[string]string{"code": "forbidden"}})
		case "busy":
			writeJSON(w, 503, map[string]any{})
		default:
			writeJSON(w, 401, map[string]any{"error": map[string]string{"code": "unauthorized"}})
		}
		return false
	}
	mux.HandleFunc("GET /v9/projects", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, 200, map[string]any{"projects": []map[string]any{
			{"id": "prj_a", "name": "web", "targets": map[string]any{"production": map[string]any{
				"id": "dpl_1", "url": "web-abc.vercel.app", "readyState": "READY", "createdAt": int64(1762833600000),
				"meta": map[string]string{"githubCommitRef": "main", "githubCommitMessage": "fix: header\n\nbody"}}}},
			{"id": "prj_b", "name": "docs", "latestDeployments": []map[string]any{
				{"id": "dpl_2", "url": "evil.example.com", "readyState": "BUILDING", "target": nil, "createdAt": int64(1762833600000)}}},
			{"id": "prj_c", "name": "empty"},
		}})
	})
	mux.HandleFunc("GET /v6/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		if r.URL.Query().Get("projectId") == "missing" {
			writeJSON(w, 404, map[string]any{"error": map[string]string{"code": "not_found"}})
			return
		}
		writeJSON(w, 200, map[string]any{"deployments": []map[string]any{
			{"uid": "dpl_1", "name": "web", "url": "web-abc.vercel.app", "created": int64(1762833600000), "state": "ERROR", "readyState": "ERROR",
				"target": "production", "inspectorUrl": "https://vercel.com/acme/web/dpl_1", "meta": map[string]string{"gitlabCommitRef": "release"}},
			{"uid": "dpl_2", "name": "web", "url": "web-def.vercel.app", "created": int64(1762830000000), "readyState": "CANCELED",
				"target": nil, "inspectorUrl": "javascript:alert(1)"},
			{"uid": "dpl_3", "name": "web", "readyState": "QUEUED"},
		}, "pagination": map[string]any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func vercelTestWire(srv *httptest.Server) vercelWire {
	return vercelWire{AuthBase: srv.URL + "/integrations", TokenURL: srv.URL + "/v2/oauth/access_token", APIBase: srv.URL, Client: srv.Client()}
}

func vercelTestClient(t *testing.T) (*vercel, *fakeVercel) {
	f, srv := newFakeVercel(t)
	c, err := newVercel(Secrets{ClientID: vercelTestID, ClientSecret: vercelTestSecret}, vercelTestSlug, vercelTestWire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// install (state only) → the team id kept with the token → projects and
// deployments read that team → disconnect deletes the token only.
func TestVercelFlow(t *testing.T) {
	f, srv := newFakeVercel(t)
	old := vercelWireDefault
	vercelWireDefault = vercelTestWire(srv)
	t.Cleanup(func() { vercelWireDefault = old })
	t.Setenv("EXT_VERCEL_INTEGRATION_SLUG", vercelTestSlug)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Vercel] = true
		c.Secrets = map[string]Secrets{Vercel: {ClientID: vercelTestID, ClientSecret: vercelTestSecret}}
	}})
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Vercel, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	if !strings.HasPrefix(auth, srv.URL+"/integrations/"+vercelTestSlug+"/new?") || u.Query().Get("state") == "" || len(u.Query()) != 1 {
		t.Fatalf("install url %s", auth)
	}
	if back := e.svc.Callback(ctx, Vercel, u.Query().Get("state"), "vc-code"); back != "tailorit://connected?provider=vercel&status=ok" {
		t.Fatalf("callback → %s", back)
	}
	tok, err := e.svc.Conns.Token(ctx, user, Vercel)
	if err != nil || tok.AccessToken != "vc-access" || vercelTeam(&tok) != "team_abc123" {
		t.Fatalf("token: %v team=%q", err, vercelTeam(&tok))
	}

	res, err := e.call(e.req(Vercel, "projects", map[string]string{"limit": "5"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if f.lastPath != "/v9/projects" || f.lastQuery.Get("teamId") != "team_abc123" || f.lastQuery.Get("limit") != "5" {
		t.Fatalf("request %s %v", f.lastPath, f.lastQuery)
	}
	want := []Row{
		{Title: "web", Value: "成功", Subtitle: "本番 · main · fix: header · 2025-11-11 13:00", URL: "https://web-abc.vercel.app"},
		{Title: "docs", Value: "ビルド中", Subtitle: "プレビュー · 2025-11-11 13:00"}, // a host off *.vercel.app is not linked
		{Title: "empty"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	if err := e.svc.Revoke(ctx, user, Vercel); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Vercel); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
}

func TestVercelDeployments(t *testing.T) {
	c, f := vercelTestClient(t)
	ctx := context.Background()
	personal := &Token{AccessToken: "vc-access"} // no team
	out, err := c.deployments(ctx, Input{Token: personal, Args: map[string]string{"project": "prj_a1", "target": "production"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.lastPath != "/v6/deployments" || f.lastQuery.Get("projectId") != "prj_a1" || f.lastQuery.Get("target") != "production" ||
		f.lastQuery.Get("limit") != "20" || f.lastQuery.Has("teamId") {
		t.Fatalf("request %s %v", f.lastPath, f.lastQuery)
	}
	want := []Row{
		{Title: "web", Value: "失敗", Subtitle: "本番 · release · 2025-11-11 13:00", URL: "https://vercel.com/acme/web/dpl_1"},
		{Title: "web", Value: "取り消し", Subtitle: "プレビュー · 2025-11-11 12:00", URL: "https://web-def.vercel.app"},
		{Title: "web", Value: "待ち", Subtitle: "プレビュー"},
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	// a project name works too
	if _, err := c.deployments(ctx, Input{Token: personal, Args: map[string]string{"project": "my-site.v2"}}); err != nil || f.lastQuery.Get("projectId") != "my-site.v2" {
		t.Fatalf("by name: %v %v", err, f.lastQuery)
	}
}

func TestVercelErrors(t *testing.T) {
	c, _ := vercelTestClient(t)
	ctx := context.Background()
	if _, err := c.projects(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := c.projects(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := c.projects(ctx, Input{Token: &Token{AccessToken: "noaccess"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	_, err := c.projects(ctx, Input{Token: &Token{AccessToken: "busy"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	_, err = c.deployments(ctx, Input{Token: &Token{AccessToken: "vc-access"}, Args: map[string]string{"project": "missing"}})
	if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("404: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "", "https://runner.example/oauth/callback/vercel"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "vc-access"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh: %v", err)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "vc-access"}); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
	// a personal install has no team; a bad team id is not kept
	for _, team := range []any{nil, "../x"} {
		f, srv := newFakeVercel(t)
		f.teamID = team
		c, _ := newVercel(Secrets{ClientID: vercelTestID, ClientSecret: vercelTestSecret}, vercelTestSlug, vercelTestWire(srv))
		tok, err := c.Exchange(ctx, "vc-code", "", "https://runner.example/oauth/callback/vercel")
		if err != nil || len(tok.Scopes) != 0 || vercelTeam(&tok) != "" {
			t.Fatalf("team %v: %v %v", team, err, tok.Scopes)
		}
	}
}

func TestVercelCheck(t *testing.T) {
	p, _ := Lookup(Vercel)
	bad := map[string][]map[string]string{
		"projects": {{"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"project": "web"}},
		"deployments": {
			{"project": "../x"}, {"project": "Web"}, {"project": "a b"}, {"project": "prj_a/b"}, {"project": strings.Repeat("a", 101)},
			{"target": "staging"}, {"limit": "0"}, {"teamId": "team_x"},
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
		"projects":    {"limit": "10"},
		"deployments": {"project": "prj_AbC123", "target": "preview", "limit": "50"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
}

// Read ops only; the API host only; off by default; not in the review
// build; no client or slug keeps it off.
func TestVercelDefinition(t *testing.T) {
	p, ok := Lookup(Vercel)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "VERCEL" {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 {
			t.Errorf("%s must be an uncached read", o.Name)
		}
	}
	for _, raw := range []string{vercelWireDefault.TokenURL, vercelWireDefault.APIBase + "/v6/deployments"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	if len(p.Hosts) != 1 {
		t.Fatalf("hosts %v", p.Hosts)
	}
	c, err := newVercel(Secrets{ClientID: "a", ClientSecret: "b"}, vercelTestSlug, vercelWireDefault)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	for _, slug := range []string{"", "Bad Slug", "../x"} {
		if _, err := newVercel(Secrets{ClientID: "a", ClientSecret: "b"}, slug, vercelWireDefault); err == nil {
			t.Errorf("built with slug %q", slug)
		}
	}
	if _, err := p.New(Secrets{ClientSecret: "b"}); err == nil {
		t.Fatal("built without a client id")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[Vercel] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
