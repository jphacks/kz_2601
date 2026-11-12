package extapi

import (
	"context"
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

// fakeSlack is Slack's oauth.v2.access, auth.revoke and Web API methods on
// httptest (no live calls).
type fakeSlack struct {
	url string

	mu        sync.Mutex
	scope     string // authed_user.scope of the next exchange
	rotating  bool   // the exchange gives an expiring token and a refresh token
	refreshes int
	revoked   []string
	profiles  []map[string]any
	posts     []map[string]any
	lastForm  url.Values
	lastQuery url.Values
	unread    *int
}

const (
	slackTestID     = "slack-client-id"
	slackTestSecret = "slack-client-secret"
	slackTestDM     = "D0123456789"
	slackTestChan   = "C0123456789"
)

func newFakeSlack(t *testing.T) (*fakeSlack, *httptest.Server) {
	three := 3
	f := &fakeSlack{scope: "im:read", unread: &three}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(w http.ResponseWriter, code string) {
		writeJSON(w, 200, map[string]any{"ok": false, "error": code})
	}
	mux.HandleFunc("POST /api/oauth.v2.access", func(w http.ResponseWriter, r *http.Request) {
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(slackTestID+":"+slackTestSecret))
		if r.Header.Get("Authorization") != want || r.ParseForm() != nil || r.PostForm.Get("client_secret") != "" {
			fail(w, "bad_client_secret")
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastForm = r.PostForm
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			if r.PostForm.Get("code") != "good-code" || !strings.HasSuffix(r.PostForm.Get("redirect_uri"), "/oauth/callback/slack") {
				fail(w, "invalid_code")
				return
			}
			u := map[string]any{"id": "U1", "scope": f.scope, "access_token": "xoxp-1", "token_type": "user"}
			if f.rotating {
				u["refresh_token"], u["expires_in"] = "xoxe-1", 43200
			}
			writeJSON(w, 200, map[string]any{"ok": true, "app_id": "A1", "team": map[string]string{"id": "T1"}, "authed_user": u})
		case "refresh_token":
			if r.PostForm.Get("refresh_token") != "xoxe-1" {
				fail(w, "invalid_refresh_token")
				return
			}
			f.refreshes++
			writeJSON(w, 200, map[string]any{"ok": true, "access_token": "xoxp-2", "refresh_token": "xoxe-2", "expires_in": 43200,
				"token_type": "user", "scope": f.scope})
		default:
			fail(w, "invalid_grant_type")
		}
	})
	// auth checks the bearer token; the special tokens give Slack's errors
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "xoxp-1", "xoxp-2":
			return true
		case "busy":
			w.Header().Set("Retry-After", "30")
			writeJSON(w, 429, map[string]any{"ok": false, "error": "ratelimited"})
		case "down":
			writeJSON(w, 503, map[string]any{"ok": false})
		case "broken":
			fail(w, "fatal_error")
		case "noscope":
			fail(w, "missing_scope")
		case "http403":
			writeJSON(w, 403, map[string]any{"ok": false})
		case "http401":
			writeJSON(w, 401, map[string]any{"ok": false})
		case "revoked":
			fail(w, "token_revoked")
		default:
			fail(w, "invalid_auth")
		}
		return false
	}
	mux.HandleFunc("POST /api/auth.revoke", func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		f.mu.Lock()
		f.revoked = append(f.revoked, tok)
		f.mu.Unlock()
		if !auth(w, r) {
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "revoked": true})
	})
	mux.HandleFunc("POST /api/users.profile.set", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var b map[string]map[string]any
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") || json.NewDecoder(r.Body).Decode(&b) != nil {
			fail(w, "invalid_form_data")
			return
		}
		if b["profile"]["status_emoji"] == ":nope:" {
			fail(w, "invalid_emoji_not_allowed")
			return
		}
		f.mu.Lock()
		f.profiles = append(f.profiles, b["profile"])
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true, "profile": b["profile"]})
	})
	mux.HandleFunc("POST /api/chat.postMessage", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var b map[string]any
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			fail(w, "invalid_form_data")
			return
		}
		switch b["channel"] {
		case slackTestChan, slackTestDM:
		case "C9999999999":
			fail(w, "not_in_channel")
			return
		default:
			fail(w, "channel_not_found")
			return
		}
		f.mu.Lock()
		f.posts = append(f.posts, b)
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true, "channel": b["channel"], "ts": "1731400000.000100",
			"message": map[string]any{"text": b["text"]}})
	})
	mux.HandleFunc("GET /api/conversations.info", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		f.lastQuery = r.URL.Query()
		unread := f.unread
		f.mu.Unlock()
		ch := map[string]any{"id": r.URL.Query().Get("channel"), "is_im": true, "last_read": "1731390000.000000"}
		if unread != nil {
			ch["unread_count"], ch["unread_count_display"] = *unread+1, *unread
		}
		writeJSON(w, 200, map[string]any{"ok": true, "channel": ch})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeSlack) wire(srv *httptest.Server) slackWire {
	return slackWire{AuthURL: srv.URL + "/oauth/v2/authorize", APIBase: srv.URL + "/api", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) }}
}

func useSlack(t *testing.T) (*env, *fakeSlack) {
	f, srv := newFakeSlack(t)
	old := slackWireDefault
	slackWireDefault = f.wire(srv)
	t.Cleanup(func() { slackWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Slack] = true
		c.Secrets = map[string]Secrets{Slack: {ClientID: slackTestID, ClientSecret: slackTestSecret}}
	}})
	return e, f
}

// connectSlack connects user asking for scopes; want is the user_scope param.
func (e *env) connectSlack(f *fakeSlack, scopes []string, want string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Slack, scopes)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth/v2/authorize?") || q.Get("user_scope") != want || q.Get("scope") != "" ||
		q.Get("code_challenge") != "" || q.Get("client_id") != slackTestID || q.Get("client_secret") != "" ||
		q.Get("redirect_uri") != "https://runner.example/oauth/callback/slack" {
		e.t.Fatalf("consent url %s", auth)
	}
	if back := e.svc.Callback(ctx, Slack, q.Get("state"), "good-code"); back != "tailorit://connected?provider=slack&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (im:read) → unread → setStatus needs users.profile:write (409,
// connect again) → set / clear → postMessage needs chat:write → post →
// revoke.
func TestSlackFlow(t *testing.T) {
	e, f := useSlack(t)
	ctx := context.Background()
	e.connectSlack(f, nil, "im:read")

	res, err := e.call(e.req(Slack, "unread", map[string]string{"channel": slackTestDM}))
	if err != nil {
		t.Fatal(err)
	}
	if it := res.Output.Item; it == nil || it.Value != "3" || it.Subtitle != slackTestDM || f.lastQuery.Get("channel") != slackTestDM {
		t.Fatalf("unread %+v", res.Output)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// read-only grant: a write is 409 and gives the run back
	if _, err := e.call(e.req(Slack, "setStatus", map[string]string{"text": "集中", "emoji": ":tomato:"})); !errors.Is(err, ErrNotConnected) || e.runs() != 1 {
		t.Fatalf("status without scope: %v runs=%d", err, e.runs())
	}
	f.scope = "im:read,users.profile:write" // Slack adds the new scope to the token
	e.connectSlack(f, []string{slackScopeStatus}, "users.profile:write")
	res, err = e.call(e.req(Slack, "setStatus", map[string]string{"text": "集中", "emoji": ":tomato:", "minutes": "25"}))
	if err != nil {
		t.Fatal(err)
	}
	until := time.Date(2026, 11, 12, 12, 25, 0, 0, time.UTC)
	p := f.profiles[0]
	if p["status_text"] != "集中" || p["status_emoji"] != ":tomato:" || p["status_expiration"] != float64(until.Unix()) {
		t.Fatalf("profile %v", p)
	}
	if it := res.Output.Item; it.Title != "集中" || it.Value != until.Format(time.RFC3339) {
		t.Fatalf("status %+v", it)
	}
	// clearing sends both empty and no expiry
	if _, err := e.call(e.req(Slack, "setStatus", nil)); err != nil {
		t.Fatal(err)
	}
	if p := f.profiles[1]; p["status_text"] != "" || p["status_emoji"] != "" || p["status_expiration"] != float64(0) {
		t.Fatalf("clear %v", p)
	}

	if _, err := e.call(e.req(Slack, "postMessage", map[string]string{"channel": slackTestChan, "text": "x"})); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("post without scope: %v", err)
	}
	f.scope = "im:read,users.profile:write,chat:write"
	e.connectSlack(f, []string{slackScopePost}, "chat:write")
	res, err = e.call(e.req(Slack, "postMessage", map[string]string{"channel": slackTestChan, "text": "日報: できた\n詳細"}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Output.ResultID != "1731400000.000100" || res.Output.Item.Title != "日報: できた" || res.Output.Item.Subtitle != slackTestChan {
		t.Fatalf("post %+v", res.Output)
	}
	if b := f.posts[0]; b["text"] != "日報: できた\n詳細" || b["parse"] != "none" || b["link_names"] != false {
		t.Fatalf("post body %v", b)
	}
	// reads still work after the write connects
	if _, err := e.call(e.req(Slack, "unread", map[string]string{"channel": slackTestDM})); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.Revoke(ctx, user, Slack); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Slack); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "xoxp-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

// A rotating token (token rotation on) is refreshed with its one-use
// refresh token; a refused refresh needs a reconnect.
func TestSlackRotation(t *testing.T) {
	e, f := useSlack(t)
	ctx := context.Background()
	f.rotating = true
	e.connectSlack(f, nil, "im:read")
	tok, err := e.svc.Conns.Token(ctx, user, Slack)
	if err != nil || tok.RefreshToken != "xoxe-1" || !tok.Expiry.Equal(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("token: %v expiry %v", err, tok.Expiry)
	}
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Slack, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Slack, "unread", map[string]string{"channel": slackTestDM})); err != nil {
		t.Fatal(err)
	}
	tok, _ = e.svc.Conns.Token(ctx, user, Slack)
	if tok.AccessToken != "xoxp-2" || tok.RefreshToken != "xoxe-2" || f.refreshes != 1 || len(tok.Scopes) != 1 {
		t.Fatalf("refreshed %d scopes=%v", f.refreshes, tok.Scopes)
	}
	// the used refresh token is refused → connection error
	if err := e.svc.Conns.Save(ctx, user, Slack, Token{AccessToken: "old", RefreshToken: "xoxe-0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Slack, "unread", map[string]string{"channel": slackTestDM})); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_refresh_token: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Slack); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}
}

// A token Slack no longer takes turns the connection to error.
func TestSlackRevokedAtProvider(t *testing.T) {
	e, f := useSlack(t)
	e.connectSlack(f, nil, "im:read")
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, Slack, Token{AccessToken: "revoked"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Slack, "unread", map[string]string{"channel": slackTestDM})); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("token_revoked: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Slack); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func slackTestClient(t *testing.T) (*slack, *fakeSlack) {
	f, srv := newFakeSlack(t)
	c, err := newSlack(Secrets{ClientID: slackTestID, ClientSecret: slackTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestSlackArgs(t *testing.T) {
	c, f := slackTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "xoxp-1"}
	badStatus := []map[string]string{
		{"text": strings.Repeat("あ", 101)},
		{"text": "a\nb"}, // one line
		{"text": "a\x00"},
		{"emoji": "tomato"},
		{"emoji": ":to mato:"},
		{"emoji": ":a::b:"},
		{"text": "x", "minutes": "0"},
		{"text": "x", "minutes": "10081"},
		{"text": "x", "minutes": "1h"},
		{"minutes": "5"}, // clearing has no expiry
	}
	for i, a := range badStatus {
		if _, err := c.setStatus(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("setStatus case %d: %v", i, err)
		}
		if checkSlackStatus(a) == nil {
			t.Errorf("checkSlackStatus case %d passed", i)
		}
	}
	badPost := []map[string]string{
		{"text": "x"},
		{"channel": "general", "text": "x"}, // names are not taken (ids only)
		{"channel": "#general", "text": "x"},
		{"channel": "U0123456789", "text": "x"},
		{"channel": slackTestChan},
		{"channel": slackTestChan, "text": "  "},
		{"channel": slackTestChan, "text": strings.Repeat("あ", 4001)},
		{"channel": slackTestChan, "text": "<!channel> 見て"},
		{"channel": slackTestChan, "text": "<!subteam^S1> 見て"},
		{"channel": slackTestChan, "text": "a\x07b"},
	}
	for i, a := range badPost {
		if _, err := c.postMessage(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("postMessage case %d: %v", i, err)
		}
	}
	for i, a := range []map[string]string{{}, {"channel": slackTestChan}, {"channel": "d0123456789"}} {
		if _, err := c.unread(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("unread case %d: %v", i, err)
		}
	}
	if len(f.profiles) != 0 || len(f.posts) != 0 || f.lastQuery != nil {
		t.Fatal("a bad argument reached Slack")
	}
	// a skin tone and an emoji alone are fine
	if _, err := c.setStatus(ctx, Input{Token: tok, Args: map[string]string{"emoji": ":wave::skin-tone-3:"}}); err != nil {
		t.Fatal(err)
	}
	// CheckArgs refuses unknown names before the op
	p, _ := Lookup(Slack)
	if _, err := p.CheckArgs("postMessage", map[string]string{"channel": slackTestChan, "text": "x", "username": "bot"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown arg: %v", err)
	}
}

func TestSlackErrors(t *testing.T) {
	c, f := slackTestClient(t)
	ctx := context.Background()
	dm := map[string]string{"channel": slackTestDM}
	var pe *ProviderError
	for _, tc := range []struct {
		tok       string
		retryable bool
	}{{"busy", true}, {"down", true}, {"broken", true}} {
		_, err := c.unread(ctx, Input{Token: &Token{AccessToken: tc.tok}, Args: dm})
		if !errors.As(err, &pe) || pe.Retryable != tc.retryable || Reason(err) != ReasonUpstream {
			t.Errorf("%s: %v", tc.tok, err)
		}
	}
	if _, err := c.unread(ctx, Input{Token: &Token{AccessToken: "busy"}, Args: dm}); !upstreamLimited(err) {
		t.Errorf("429 should pause the provider: %v", err)
	}
	for _, tok := range []string{"noscope", "http403"} {
		if _, err := c.unread(ctx, Input{Token: &Token{AccessToken: tok}, Args: dm}); !errors.Is(err, ErrNotConnected) {
			t.Errorf("%s: %v", tok, err)
		}
	}
	for _, tok := range []string{"dead", "revoked", "http401"} {
		if _, err := c.unread(ctx, Input{Token: &Token{AccessToken: tok}, Args: dm}); !errors.Is(err, ErrInvalidGrant) {
			t.Errorf("%s: %v", tok, err)
		}
	}
	if _, err := c.unread(ctx, Input{Args: dm}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("no token: %v", err)
	}
	// Slack's request errors are bad_request
	tok := &Token{AccessToken: "xoxp-1"}
	for _, ch := range []string{"C9999999999", "C8888888888"} {
		_, err := c.postMessage(ctx, Input{Token: tok, Args: map[string]string{"channel": ch, "text": "x"}})
		if Reason(err) != ReasonBadRequest || !errors.As(err, &pe) || pe.Retryable {
			t.Errorf("%s: %v", ch, err)
		}
	}
	if _, err := c.setStatus(ctx, Input{Token: tok, Args: map[string]string{"emoji": ":nope:"}}); Reason(err) != ReasonBadRequest {
		t.Errorf("bad emoji: %v", err)
	}
	// a conversation without an unread count (not a DM) is bad_request
	f.unread = nil
	if _, err := c.unread(ctx, Input{Token: tok, Args: dm}); Reason(err) != ReasonBadRequest {
		t.Errorf("no count: %v", err)
	}
	// a write scope missing from a known grant is refused before Slack
	only := &Token{AccessToken: "xoxp-1", Scopes: []string{slackScopeIM}}
	if _, err := c.postMessage(ctx, Input{Token: only, Args: map[string]string{"channel": slackTestChan, "text": "x"}}); !errors.Is(err, ErrNotConnected) || len(f.posts) != 0 {
		t.Errorf("post with im:read only: %v", err)
	}
	// an odd error code does not reach logs as is
	if err := slackError("Bad Code\n"); !strings.Contains(err.Error(), "slack unknown") {
		t.Errorf("code: %v", err)
	}

	// OAuth: a bad code / refresh token is invalid grant; no refresh token
	// (no rotation) cannot be renewed
	if _, err := c.Exchange(ctx, "bad", "", "https://runner.example/oauth/callback/slack"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "xoxp-1"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("no refresh token: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{RefreshToken: "xoxe-0"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("used refresh token: %v", err)
	}
	// revoking a token Slack already refuses is done; an expired rotating
	// token is renewed first
	if err := c.Revoke(ctx, Token{AccessToken: "dead"}); err != nil {
		t.Fatalf("gone: %v", err)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "old", RefreshToken: "xoxe-1", Expiry: c.now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if got := f.revoked[len(f.revoked)-1]; got != "xoxp-2" {
		t.Fatalf("revoked %v", f.revoked)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "busy"}); Reason(err) != ReasonUpstream {
		t.Fatalf("revoke 429: %v", err)
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// minimal user scopes; without the client id / secret the provider stays
// off; off by default and never in the review build.
func TestSlackDefinition(t *testing.T) {
	p, ok := Lookup(Slack)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "SLACK" {
		t.Fatalf("%+v", p)
	}
	w := slackWireDefault
	for _, raw := range []string{w.APIBase + "/oauth.v2.access", w.APIBase + "/auth.revoke", w.APIBase + "/chat.postMessage"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	if len(p.Hosts) != 1 {
		t.Errorf("hosts %v", p.Hosts)
	}
	want := map[string]struct {
		scope string
		write bool
	}{"setStatus": {slackScopeStatus, true}, "postMessage": {slackScopePost, true}, "unread": {slackScopeIM, false}}
	for _, o := range p.Ops {
		wo, ok := want[o.Name]
		if !ok || len(o.Scopes) != 1 || o.Scopes[0] != wo.scope || o.Write != wo.write || o.CacheTTL != 0 {
			t.Errorf("%s: scopes %v write %v", o.Name, o.Scopes, o.Write)
		}
	}
	c, err := newSlack(Secrets{ClientID: "a", ClientSecret: "b"}, w)
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
	if cfg.Enabled[Slack] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
