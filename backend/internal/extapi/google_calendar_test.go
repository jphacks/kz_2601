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

// fakeGoogle is Google's token / revoke endpoints and the Calendar API on
// httptest (no live calls).
type fakeGoogle struct {
	t   *testing.T
	url string

	mu        sync.Mutex
	challenge string // from the consent URL
	refreshOK bool
	revoked   []string
	created   map[string]map[string]any
	lastQuery url.Values
}

const (
	gcalTestID     = "client-id"
	gcalTestSecret = "client-secret"
)

func newFakeGoogle(t *testing.T) (*fakeGoogle, *httptest.Server) {
	f := &fakeGoogle{t: t, refreshOK: true, created: map[string]map[string]any{}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != gcalTestID || r.Form.Get("client_secret") != gcalTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || !strings.HasSuffix(r.Form.Get("redirect_uri"), "/oauth/callback/googleCalendar") {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600, "scope": gcalScopeRead, "token_type": "Bearer"})
		case "refresh_token":
			if !f.refreshOK || r.Form.Get("refresh_token") != "refresh-1" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant", "error_description": "Token has been expired or revoked."})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "access-2", "expires_in": 3600, "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.revoked = append(f.revoked, r.Form.Get("token"))
		f.mu.Unlock()
		if r.Form.Get("token") == "gone" {
			writeJSON(w, 400, map[string]string{"error": "invalid_token"})
			return
		}
		w.WriteHeader(200)
	})
	events := func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "access-1", "access-2", "wide":
		case "narrow":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": 403, "status": "PERMISSION_DENIED",
				"details": []map[string]string{{"reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT"}}}})
			return
		case "busy":
			writeJSON(w, 503, map[string]any{"error": map[string]any{"code": 503, "status": "UNAVAILABLE"}})
			return
		default:
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": 401, "status": "UNAUTHENTICATED"}})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodGet {
			f.lastQuery = r.URL.Query()
			writeJSON(w, 200, map[string]any{"items": []map[string]any{
				{"id": "e1", "summary": "会議", "location": "本社", "htmlLink": "https://www.google.com/calendar/event?eid=e1", "start": map[string]string{"dateTime": "2026-11-12T10:00:00+09:00"}},
				{"id": "e2", "htmlLink": "javascript:alert(1)", "start": map[string]string{"date": "2026-11-13"}},
			}})
			return
		}
		var ev map[string]any
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil || r.URL.Query().Get("sendUpdates") != "none" {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": 400}})
			return
		}
		id, _ := ev["id"].(string)
		if _, dup := f.created[id]; dup {
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": 409, "errors": []map[string]string{{"reason": "duplicate"}}}})
			return
		}
		f.created[id] = ev
		ev["htmlLink"] = "https://www.google.com/calendar/event?eid=" + id
		writeJSON(w, 200, ev)
	}
	mux.HandleFunc("GET /calendar/v3/calendars/primary/events", events)
	mux.HandleFunc("POST /calendar/v3/calendars/primary/events", events)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeGoogle) wire(srv *httptest.Server) gcalWire {
	return gcalWire{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", RevokeURL: srv.URL + "/revoke",
		APIBase: srv.URL + "/calendar/v3", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, gcalJST) }}
}

// useGoogleCalendar points the provider at the fake and builds a Service
// with it switched on.
func useGoogleCalendar(t *testing.T) (*env, *fakeGoogle) {
	f, srv := newFakeGoogle(t)
	old := googleCalendarWire
	googleCalendarWire = f.wire(srv)
	t.Cleanup(func() { googleCalendarWire = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[GoogleCalendar] = true
		c.Secrets = map[string]Secrets{GoogleCalendar: {ClientID: gcalTestID, ClientSecret: gcalTestSecret}}
	}})
	return e, f
}

func (e *env) connectGoogle(f *fakeGoogle) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, GoogleCalendar, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/auth?") || q.Get("scope") != gcalScopeRead || q.Get("code_challenge_method") != "S256" ||
		q.Get("access_type") != "offline" || q.Get("include_granted_scopes") != "true" || q.Get("client_id") != gcalTestID ||
		q.Get("redirect_uri") != "https://runner.example/oauth/callback/googleCalendar" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, GoogleCalendar, q.Get("state"), "good-code"); back != "tailorit://connected?provider=googleCalendar&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE) → list → a write without its scope is 409 (connect again
// with it) → refresh → a refused refresh needs a reconnect → revoke.
func TestGoogleCalendarFlow(t *testing.T) {
	e, f := useGoogleCalendar(t)
	ctx := context.Background()
	if _, err := e.call(e.req(GoogleCalendar, "listEvents", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("before connecting: %v", err)
	}
	e.connectGoogle(f)
	c, _, _ := e.mem.Get(ctx, user, GoogleCalendar)
	if c.Status != StatusActive || len(c.Scopes) != 1 || c.Scopes[0] != gcalScopeRead {
		t.Fatalf("connection %+v", c)
	}

	res, err := e.call(e.req(GoogleCalendar, "listEvents", map[string]string{"timeMin": "2026-11-12", "timeMax": "2026-11-14", "q": "会議"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 2 || rows[0].Title != "会議" || rows[0].Subtitle != "本社" || rows[0].Value != "2026-11-12T10:00:00+09:00" ||
		rows[0].URL == "" || rows[1].Title != gcalNoTitle || rows[1].Value != "2026-11-13" || rows[1].URL != "" || res.Charge.Coins != 0 {
		t.Fatalf("rows %+v", rows)
	}
	if q := f.lastQuery; q.Get("timeMin") != "2026-11-12T00:00:00+09:00" || q.Get("timeMax") != "2026-11-14T00:00:00+09:00" ||
		q.Get("singleEvents") != "true" || q.Get("q") != "会議" {
		t.Fatalf("query %v", q)
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// the read-only grant cannot write: 409, the run given back
	_, err = e.call(e.req(GoogleCalendar, "createEvent", map[string]string{"title": "x", "start": "2026-11-12T10:00:00+09:00"}))
	if !errors.Is(err, ErrNotConnected) || e.runs() != 1 {
		t.Fatalf("write without scope: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, GoogleCalendar); c.Status != StatusActive {
		t.Fatal("a missing scope must not break the connection")
	}

	// expired → refreshed (Google keeps the refresh token)
	if err := e.svc.Conns.Save(ctx, user, GoogleCalendar, Token{AccessToken: "access-1", RefreshToken: "refresh-1",
		Expiry: e.c.t.Add(-time.Minute), Scopes: []string{gcalScopeRead}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GoogleCalendar, "listEvents", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err := e.svc.Conns.Token(ctx, user, GoogleCalendar)
	if err != nil || tok.AccessToken != "access-2" || tok.RefreshToken != "refresh-1" || len(tok.Scopes) != 1 {
		t.Fatalf("refreshed %v", err)
	}

	// refresh refused (revoked at Google) → error: the person reconnects
	f.mu.Lock()
	f.refreshOK = false
	f.mu.Unlock()
	if err := e.svc.Conns.Save(ctx, user, GoogleCalendar, Token{AccessToken: "access-2", RefreshToken: "refresh-1", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GoogleCalendar, "listEvents", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, GoogleCalendar); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect, then cut: the refresh token is revoked at Google
	e.connectGoogle(f)
	if err := e.svc.Revoke(ctx, user, GoogleCalendar); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, GoogleCalendar); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "refresh-1" {
		t.Fatalf("revoked %v", f.revoked)
	}
}

// A 401 on a call (token revoked at Google) turns the connection to error.
func TestGoogleCalendarRevokedAtProvider(t *testing.T) {
	e, f := useGoogleCalendar(t)
	e.connectGoogle(f)
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, GoogleCalendar, Token{AccessToken: "dead", RefreshToken: "refresh-1", Expiry: e.c.t.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(GoogleCalendar, "listEvents", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, GoogleCalendar); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func gcalTestClient(t *testing.T) (*googleCalendar, *fakeGoogle) {
	f, srv := newFakeGoogle(t)
	g, err := newGoogleCalendar(Secrets{ClientID: gcalTestID, ClientSecret: gcalTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return g, f
}

func TestGoogleCalendarCreateEvent(t *testing.T) {
	g, f := gcalTestClient(t)
	ctx := context.Background()
	wide := &Token{AccessToken: "wide", Scopes: []string{gcalScopeWrite}}

	out, err := g.createEvent(ctx, Input{Token: wide, RequestID: "r1", Args: map[string]string{"title": "打ち合わせ", "start": "2026-11-12T10:00:00+09:00", "location": "会議室"}})
	if err != nil {
		t.Fatal(err)
	}
	id := gcalEventID("r1")
	ev := f.created[id]
	if out.ResultID != id || out.Item.Title != "打ち合わせ" || ev["summary"] != "打ち合わせ" || ev["location"] != "会議室" ||
		ev["end"].(map[string]any)["dateTime"] != "2026-11-12T11:00:00+09:00" {
		t.Fatalf("out %+v ev %v", out, ev)
	}
	// the same request again makes no second event
	again, err := g.createEvent(ctx, Input{Token: wide, RequestID: "r1", Args: map[string]string{"title": "打ち合わせ", "start": "2026-11-12T10:00:00+09:00"}})
	if err != nil || again.ResultID != id || len(f.created) != 1 {
		t.Fatalf("retry %+v %v (%d events)", again, err, len(f.created))
	}
	// all day: Google's end is exclusive
	if _, err := g.createEvent(ctx, Input{Token: wide, RequestID: "r2", Args: map[string]string{"title": "旅行", "start": "2026-11-20", "end": "2026-11-22"}}); err != nil {
		t.Fatal(err)
	}
	ev = f.created[gcalEventID("r2")]
	if ev["start"].(map[string]any)["date"] != "2026-11-20" || ev["end"].(map[string]any)["date"] != "2026-11-23" {
		t.Fatalf("all day %v", ev)
	}
	if !validGcalID(id) {
		t.Fatalf("bad event id %q", id)
	}
}

func validGcalID(id string) bool {
	if len(id) < 5 || len(id) > 1024 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'v') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func TestGoogleCalendarArgs(t *testing.T) {
	g, f := gcalTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "wide"}
	bad := []map[string]string{
		{"start": "2026-11-12T10:00:00+09:00"}, // no title
		{"title": "x"},                         // no start
		{"title": "x", "start": "tomorrow"},    // not a time
		{"title": "x", "start": "2026-11-12T10:00:00+09:00", "end": "2026-11-12T09:00:00+09:00"}, // end before start
		{"title": "x", "start": "2026-11-12", "end": "2026-11-12T09:00:00+09:00"},                // date and time mixed
		{"title": "x", "start": "2026-11-12", "end": "2028-11-12"},                               // over a year
		{"title": strings.Repeat("あ", 501), "start": "2026-11-12"},                               // too long
		{"title": "a\x00b", "start": "2026-11-12"},                                               // control char
		{"title": "x", "start": "2026-11-12", "description": strings.Repeat("d", 8001)},          // too long
	}
	for i, a := range bad {
		if _, err := g.createEvent(ctx, Input{Token: tok, RequestID: "x", Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("createEvent case %d: %v", i, err)
		}
	}
	badList := []map[string]string{
		{"timeMin": "2026-11-14", "timeMax": "2026-11-12"},
		{"timeMin": "2026-01-01", "timeMax": "2027-06-01"},
		{"maxResults": "0"},
		{"maxResults": "51"},
		{"maxResults": "x"},
		{"timeMin": "11/12"},
		{"q": strings.Repeat("q", 257)},
	}
	for i, a := range badList {
		if _, err := g.listEvents(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("listEvents case %d: %v", i, err)
		}
	}
	if len(f.created) != 0 || f.lastQuery != nil {
		t.Fatal("a bad argument reached Google")
	}
	// no args: now → 7 days, 20 rows
	if _, err := g.listEvents(ctx, Input{Token: tok}); err != nil {
		t.Fatal(err)
	}
	if q := f.lastQuery; q.Get("timeMin") != "2026-11-11T12:00:00+09:00" || q.Get("timeMax") != "2026-11-18T12:00:00+09:00" || q.Get("maxResults") != "20" {
		t.Fatalf("default query %v", q)
	}
}

func TestGoogleCalendarErrors(t *testing.T) {
	g, _ := gcalTestClient(t)
	ctx := context.Background()
	_, err := g.listEvents(ctx, Input{Token: &Token{AccessToken: "narrow"}})
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403 scope: %v", err)
	}
	_, err = g.listEvents(ctx, Input{Token: &Token{AccessToken: "busy"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || pe.Billable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := g.listEvents(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := g.listEvents(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	// an already revoked token is done
	if err := g.Revoke(ctx, Token{RefreshToken: "gone"}); err != nil {
		t.Fatalf("revoke gone: %v", err)
	}
	if _, err := g.Refresh(ctx, Token{AccessToken: "a"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a refresh token: %v", err)
	}
	if _, err := g.Exchange(ctx, "bad-code", "v", "https://runner.example/oauth/callback/googleCalendar"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// without the client id / secret the provider stays off; off by default
// and never in the review build.
func TestGoogleCalendarDefinition(t *testing.T) {
	p, ok := Lookup(GoogleCalendar)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "GOOGLE_CALENDAR" {
		t.Fatalf("%+v", p)
	}
	w := googleCalendarWire
	for _, raw := range []string{w.TokenURL, w.RevokeURL, w.APIBase + "/calendars/primary/events"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, o := range p.Ops {
		for _, s := range o.Scopes {
			if s == "https://www.googleapis.com/auth/calendar" || s == "https://www.googleapis.com/auth/calendar.events" {
				t.Errorf("%s asks for a wide scope %s", o.Name, s)
			}
		}
	}
	g, err := newGoogleCalendar(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.hc.Get("https://evil.example.com/")
	if !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil {
		t.Fatal("built without a secret")
	}
	c, _ := ConfigFromEnv(func(string) string { return "" })
	if c.Enabled[GoogleCalendar] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
