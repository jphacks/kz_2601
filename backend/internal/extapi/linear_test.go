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
	linearTestID     = "ln-client"
	linearTestSecret = "ln-secret"
	linearTestTeam   = "9cfb482a-81e3-4154-b5b9-2c805e70a02d"
)

// fakeLinear is Linear's OAuth and GraphQL on httptest (no live calls).
type fakeLinear struct {
	url string

	mu         sync.Mutex
	challenge  string
	scope      string // granted on the next code exchange
	rotateNext int
	refreshes  int
	revoked    []string
	teams      int // the viewer's teams
	vars       map[string]any
	queries    []string
	created    []map[string]any
}

func newFakeLinear(t *testing.T) (*fakeLinear, *httptest.Server) {
	f := &fakeLinear{scope: "read", teams: 1}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != linearTestID || r.Form.Get("client_secret") != linearTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "ln-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "lin_oauth_0", "refresh_token": "lin_refresh_0", "expires_in": 86399, "scope": f.scope, "token_type": "Bearer"})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "lin_refresh_"+strconv.Itoa(f.rotateNext) {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			// apps made before December 2023 answer the scopes as an array
			writeJSON(w, 200, map[string]any{"access_token": "lin_oauth_" + n, "refresh_token": "lin_refresh_" + n, "expires_in": 86399, "scope": strings.Fields(f.scope), "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		_ = r.ParseForm()
		if !ok || id != linearTestID || secret != linearTestSecret {
			w.WriteHeader(401)
			return
		}
		if !strings.HasPrefix(r.Form.Get("token"), "lin_") {
			w.WriteHeader(400) // already revoked
			return
		}
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token_type_hint")+":"+r.Form.Get("token"))
		f.mu.Unlock()
		w.WriteHeader(200)
	})
	gqlErr := func(w http.ResponseWriter, status int, code string) {
		writeJSON(w, status, map[string]any{"errors": []map[string]any{{"message": code, "extensions": map[string]any{"code": code}}}})
	}
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			gqlErr(w, 400, "FORBIDDEN")
			return
		case tok == "limited":
			w.Header().Set("Retry-After", "30")
			gqlErr(w, 429, "RATELIMITED")
			return
		case tok == "busy":
			writeJSON(w, 503, nil)
			return
		case tok == "dead":
			gqlErr(w, 400, "AUTHENTICATION_ERROR")
			return
		case tok == "gone":
			w.WriteHeader(401)
			return
		case !strings.HasPrefix(tok, "lin_oauth_"):
			gqlErr(w, 400, "AUTHENTICATION_ERROR")
			return
		}
		var b struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || r.Header.Get("Content-Type") != "application/json" {
			gqlErr(w, 400, "GRAPHQL_PARSE_FAILED")
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.vars = b.Variables
		f.queries = append(f.queries, b.Query)
		node := func(id string) map[string]any { return map[string]any{"id": id} }
		switch {
		case strings.Contains(b.Query, "assignedIssues"):
			writeJSON(w, 200, map[string]any{"data": map[string]any{"viewer": map[string]any{"assignedIssues": map[string]any{"nodes": []map[string]any{
				{"identifier": "ENG-12", "title": "ログインを直す", "url": "https://linear.app/acme/issue/ENG-12/x", "dueDate": "2026-11-20", "state": map[string]any{"name": "In Progress"}},
				{"identifier": "ENG-3", "title": "期限なし", "url": "https://evil.example/x", "dueDate": nil, "state": nil},
			}}}}})
		case strings.Contains(b.Query, "viewer { teams"):
			nodes := []any{}
			for i := range f.teams {
				nodes = append(nodes, node(linearTestTeam[:35]+strconv.Itoa(i)))
			}
			writeJSON(w, 200, map[string]any{"data": map[string]any{"viewer": map[string]any{"teams": map[string]any{"nodes": nodes}}}})
		case strings.Contains(b.Query, "teams(first: 1"):
			nodes := []any{}
			if b.Variables["key"] == "ENG" {
				nodes = append(nodes, node(linearTestTeam))
			}
			writeJSON(w, 200, map[string]any{"data": map[string]any{"teams": map[string]any{"nodes": nodes}}})
		case strings.Contains(b.Query, "issueCreate"):
			in, _ := b.Variables["input"].(map[string]any)
			if title, _ := in["title"].(string); title == "invalid" {
				gqlErr(w, 400, "INVALID_INPUT")
				return
			}
			f.created = append(f.created, in)
			n := strconv.Itoa(len(f.created) + 99)
			writeJSON(w, 200, map[string]any{"data": map[string]any{"issueCreate": map[string]any{"success": true, "issue": map[string]any{
				"id": "abc", "identifier": "ENG-" + n, "title": in["title"], "url": "https://linear.app/acme/issue/ENG-" + n + "/y", "state": map[string]any{"name": "Backlog"},
			}}}})
		default:
			gqlErr(w, 400, "GRAPHQL_VALIDATION_FAILED")
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeLinear) wire(srv *httptest.Server) linearWire {
	return linearWire{AuthURL: srv.URL + "/oauth/authorize", TokenURL: srv.URL + "/oauth/token", RevokeURL: srv.URL + "/oauth/revoke",
		GraphQL: srv.URL + "/graphql", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }}
}

func useLinear(t *testing.T) (*env, *fakeLinear) {
	f, srv := newFakeLinear(t)
	old := linearWireDefault
	linearWireDefault = f.wire(srv)
	t.Cleanup(func() { linearWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Linear] = true
		c.Secrets = map[string]Secrets{Linear: {ClientID: linearTestID, ClientSecret: linearTestSecret}}
	}})
	return e, f
}

func (e *env) connectLinear(f *fakeLinear, scopes []string, want string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Linear, scopes)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth/authorize?") || q.Get("scope") != want || q.Get("code_challenge_method") != "S256" || q.Get("actor") != "user" ||
		q.Get("client_id") != linearTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/linear" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Linear, q.Get("state"), "ln-code"); back != "tailorit://connected?provider=linear&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE, read) → myIssues → createIssue needs issues:create (409,
// connect again asking for it, read kept) → refresh (rotated) → refused
// refresh needs a reconnect → revoke.
func TestLinearFlow(t *testing.T) {
	e, f := useLinear(t)
	ctx := context.Background()
	e.connectLinear(f, nil, "read")

	res, err := e.call(e.req(Linear, "myIssues", map[string]string{"limit": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "ログインを直す", Subtitle: "ENG-12 · In Progress", Value: "2026-11-20", URL: "https://linear.app/acme/issue/ENG-12/x"}) ||
		rows[1] != (Row{Title: "期限なし", Subtitle: "ENG-3"}) {
		t.Fatalf("rows %+v", rows)
	}
	filter, _ := json.Marshal(f.vars["filter"])
	if f.vars["first"] != float64(10) || string(filter) != `{"state":{"type":{"nin":["completed","canceled"]}}}` {
		t.Fatalf("vars %v", f.vars)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// read-only grant: creating is 409 and gives the run back
	if _, err := e.call(e.req(Linear, "createIssue", map[string]string{"title": "x"})); !errors.Is(err, ErrNotConnected) || e.runs() != 1 {
		t.Fatalf("create without scope: %v runs=%d", err, e.runs())
	}
	f.scope = "read issues:create"
	e.connectLinear(f, []string{linearScopeCreate}, "read,issues:create")
	res, err = e.call(e.req(Linear, "createIssue", map[string]string{"title": "牛乳を買う", "description": "2 本"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "ENG-100" || res.Output.Item.Title != "牛乳を買う" || res.Output.Item.URL != "https://linear.app/acme/issue/ENG-100/y" {
		t.Fatalf("created %+v", res.Output)
	}
	if in := f.created[0]; in["teamId"] != linearTestTeam[:35]+"0" || in["description"] != "2 本" {
		t.Fatalf("input %v", in)
	}
	if _, err := e.call(e.req(Linear, "myIssues", map[string]string{"state": "all"})); err != nil || f.vars["filter"] != nil {
		t.Fatalf("reads after the write connect: %v %v", err, f.vars)
	}

	// expired → refreshed with the rotated refresh token (scopes as an array)
	tok, _ := e.svc.Conns.Token(ctx, user, Linear)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Linear, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Linear, "myIssues", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Linear)
	if err != nil || tok.AccessToken != "lin_oauth_1" || tok.RefreshToken != "lin_refresh_1" || f.refreshes != 1 || !linearCanCreate(&tok) {
		t.Fatalf("refreshed: %v refreshes=%d scopes=%v", err, f.refreshes, tok.Scopes)
	}

	// an old (rotated away) refresh token is invalid_grant → error
	if err := e.svc.Conns.Save(ctx, user, Linear, Token{AccessToken: "lin_oauth_0", RefreshToken: "lin_refresh_0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Linear, "myIssues", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Linear); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect and cut: the refresh token is revoked at Linear
	e.connectLinear(f, nil, "read")
	if err := e.svc.Revoke(ctx, user, Linear); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Linear); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "refresh_token:lin_refresh_0" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

func linearTestClient(t *testing.T) (*linear, *fakeLinear) {
	f, srv := newFakeLinear(t)
	c, err := newLinear(Secrets{ClientID: linearTestID, ClientSecret: linearTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

// team: a key is looked up, an id is used as is, none is the only team
// (more than one: a bad request, nothing created).
func TestLinearCreateTeam(t *testing.T) {
	c, f := linearTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "lin_oauth_0", Scopes: []string{"read", "issues:create"}}
	create := func(a map[string]string) (Output, error) {
		return c.createIssue(ctx, Input{Token: tok, RequestID: "r", Args: a})
	}
	if _, err := create(map[string]string{"title": "a", "team": "eng"}); err != nil || f.created[0]["teamId"] != linearTestTeam {
		t.Fatalf("by key: %v %v", err, f.created)
	}
	upper := strings.ToUpper(linearTestTeam)
	if _, err := create(map[string]string{"title": "b", "team": upper}); err != nil || f.created[1]["teamId"] != linearTestTeam || len(f.queries) != 3 {
		t.Fatalf("by id: %v %v queries=%d", err, f.created, len(f.queries))
	}
	if _, err := create(map[string]string{"title": "c", "team": "OPS"}); Reason(err) != ReasonBadRequest {
		t.Fatalf("no such team: %v", err)
	}
	f.teams = 2
	if _, err := create(map[string]string{"title": "d"}); Reason(err) != ReasonBadRequest {
		t.Fatalf("two teams: %v", err)
	}
	f.teams = 0
	if _, err := create(map[string]string{"title": "e"}); Reason(err) != ReasonBadRequest {
		t.Fatalf("no team: %v", err)
	}
	if _, err := create(map[string]string{"title": "invalid", "team": linearTestTeam}); Reason(err) != ReasonBadRequest {
		t.Fatalf("INVALID_INPUT: %v", err)
	}
	if len(f.created) != 2 {
		t.Fatalf("created %v", f.created)
	}
	// write covers issues:create; read only does not
	if _, err := c.createIssue(ctx, Input{Token: &Token{AccessToken: "lin_oauth_0", Scopes: []string{"read", "write"}}, Args: map[string]string{"title": "f", "team": linearTestTeam}}); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	if _, err := c.createIssue(ctx, Input{Token: &Token{AccessToken: "lin_oauth_0", Scopes: []string{"read"}}, Args: map[string]string{"title": "g"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("read only: %v", err)
	}
}

// FORBIDDEN / 403 → connect again, AUTHENTICATION_ERROR / 401 → the
// connection is an error, 429 RATELIMITED → a retryable 429 (pauses the
// provider), 5xx → upstream.
func TestLinearErrors(t *testing.T) {
	c, _ := linearTestClient(t)
	ctx := context.Background()
	in := func(tok string) Input { return Input{Token: &Token{AccessToken: tok}} }
	if _, err := c.myIssues(ctx, in("noscope")); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("FORBIDDEN: %v", err)
	}
	for _, tok := range []string{"dead", "gone"} {
		if _, err := c.myIssues(ctx, in(tok)); !errors.Is(err, ErrInvalidGrant) {
			t.Fatalf("%s: %v", tok, err)
		}
	}
	_, err := c.myIssues(ctx, in("limited"))
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Status != 429 || !pe.Retryable || Reason(err) != ReasonUpstream || !upstreamLimited(err) {
		t.Fatalf("429: %v", err)
	}
	if _, err := c.myIssues(ctx, in("busy")); !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := c.myIssues(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "lin_oauth_0"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/linear"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	// already revoked (400) is done; without a refresh token the access token is revoked
	if err := c.Revoke(ctx, Token{AccessToken: "x"}); err != nil {
		t.Fatalf("400: %v", err)
	}
	if err := c.Revoke(ctx, Token{}); err != nil {
		t.Fatal(err)
	}
}

func TestLinearCheck(t *testing.T) {
	p, _ := Lookup(Linear)
	bad := map[string][]map[string]string{
		"myIssues": {{"state": "done"}, {"state": "Open"}, {"limit": "0"}, {"limit": "51"}, {"limit": "x"}, {"assignee": "me"}},
		"createIssue": {
			{}, {"title": "  "}, {"title": strings.Repeat("あ", 256)}, {"title": "a\nb"}, {"title": "a\x00"},
			{"title": "x", "description": strings.Repeat("d", 10001)},
			{"title": "x", "team": "ENG-1"}, {"title": "x", "team": "../x"}, {"title": "x", "team": "TOOLONGKEY"},
			{"title": "x", "teamId": linearTestTeam},
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
		"myIssues":    {"state": "started", "limit": "50"},
		"createIssue": {"title": "牛乳を買う", "description": "1 行目\n2 行目", "team": "ENG"},
	} {
		if _, err := p.CheckArgs(op, a); err != nil {
			t.Errorf("%s: %v", op, err)
		}
	}
	c, f := linearTestClient(t)
	if _, err := c.myIssues(context.Background(), Input{Token: &Token{AccessToken: "lin_oauth_0"}, Args: map[string]string{"state": "completed"}}); err != nil {
		t.Fatal(err)
	}
	if filter, _ := json.Marshal(f.vars["filter"]); string(filter) != `{"state":{"type":{"eq":"completed"}}}` {
		t.Fatalf("filter %s", filter)
	}
}

// Only createIssue writes (issues:create, never write / admin); the real
// endpoints are on the allowlist and nothing else; off by default and never
// in the review build.
func TestLinearDefinition(t *testing.T) {
	p, ok := Lookup(Linear)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "LINEAR" {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write != (o.Name == "createIssue") {
			t.Errorf("%s: write = %v", o.Name, o.Write)
		}
		for _, s := range o.Scopes {
			if s == linearScopeWrite || s == "admin" {
				t.Errorf("%s asks for a wide scope %s", o.Name, s)
			}
		}
	}
	w := linearWireDefault
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.GraphQL} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	c, err := newLinear(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.hc.Get("https://linear.app/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientSecret: "b"}); err == nil {
		t.Fatal("built without a client id")
	}
	cfg, _ := ConfigFromEnv(func(string) string { return "" })
	if cfg.Enabled[Linear] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
