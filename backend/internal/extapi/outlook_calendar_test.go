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

// fakeMicrosoft is Microsoft identity's token endpoint and Graph's calendar
// on httptest (no live calls).
type fakeMicrosoft struct {
	url string

	mu           sync.Mutex
	challenge    string // from the consent URL
	refreshOK    bool
	refreshScope string
	created      map[string]map[string]any // by transactionId
	lastQuery    url.Values
	lastPrefer   string
}

const (
	outlookTestID     = "ms-client-id"
	outlookTestSecret = "ms-client-secret"
)

func newFakeMicrosoft(t *testing.T) (*fakeMicrosoft, *httptest.Server) {
	f := &fakeMicrosoft{refreshOK: true, created: map[string]map[string]any{}}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /common/oauth2/v2.0/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != outlookTestID || r.Form.Get("client_secret") != outlookTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || !strings.HasSuffix(r.Form.Get("redirect_uri"), "/oauth/callback/outlookCalendar") {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			// Microsoft may answer short scope names
			writeJSON(w, 200, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "expires_in": 3600,
				"scope": "Calendars.Read offline_access openid", "token_type": "Bearer"})
		case "refresh_token":
			f.refreshScope = r.Form.Get("scope")
			if !f.refreshOK || r.Form.Get("refresh_token") != "refresh-1" {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant", "error_description": "AADSTS70000"})
				return
			}
			writeJSON(w, 200, map[string]any{"access_token": "access-2", "refresh_token": "refresh-2", "expires_in": 3600,
				"scope": "https://graph.microsoft.com/Calendars.Read", "token_type": "Bearer"})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		switch strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") {
		case "access-1", "access-2", "wide":
			return true
		case "narrow":
			writeJSON(w, 403, map[string]any{"error": map[string]any{"code": "ErrorAccessDenied", "message": "Access is denied."}})
		case "busy":
			writeJSON(w, 503, map[string]any{"error": map[string]any{"code": "ServiceNotAvailable"}})
		case "nomailbox":
			writeJSON(w, 404, map[string]any{"error": map[string]any{"code": "MailboxNotEnabledForRESTAPI"}})
		default:
			writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "InvalidAuthenticationToken"}})
		}
		return false
	}
	mux.HandleFunc("GET /v1.0/me/calendarView", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		f.lastQuery = r.URL.Query()
		f.lastPrefer = r.Header.Get("Prefer")
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"value": []map[string]any{
			{"id": "e1", "subject": "会議", "location": map[string]string{"displayName": "本社"},
				"webLink": "https://outlook.live.com/owa/?itemid=e1&exvsurl=1&path=/calendar/item",
				"start":   map[string]string{"dateTime": "2026-11-12T10:00:00.0000000", "timeZone": "Tokyo Standard Time"}},
			{"id": "e2", "isAllDay": true, "webLink": "javascript:alert(1)",
				"start": map[string]string{"dateTime": "2026-11-13T00:00:00.0000000", "timeZone": "Tokyo Standard Time"}},
			{"id": "e3", "subject": "ランチ", "start": map[string]string{"dateTime": "2026-11-13T03:00:00.0000000", "timeZone": "UTC"}},
		}})
	})
	mux.HandleFunc("POST /v1.0/me/events", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var ev map[string]any
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			writeJSON(w, 400, map[string]any{"error": map[string]any{"code": "BadRequest"}})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		tx, _ := ev["transactionId"].(string)
		if prev, dup := f.created[tx]; dup && tx != "" {
			// Graph keeps the first event of a transactionId
			writeJSON(w, 201, prev)
			return
		}
		ev["id"] = "ev-" + tx[:min(len(tx), 16)]
		ev["webLink"] = "https://outlook.live.com/owa/?itemid=" + ev["id"].(string)
		f.created[tx] = ev
		writeJSON(w, 201, ev)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeMicrosoft) wire(srv *httptest.Server) outlookWire {
	return outlookWire{AuthURL: srv.URL + "/common/oauth2/v2.0/authorize", TokenURL: srv.URL + "/common/oauth2/v2.0/token",
		APIBase: srv.URL + "/v1.0", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, gcalJST) }}
}

func useOutlookCalendar(t *testing.T) (*env, *fakeMicrosoft) {
	f, srv := newFakeMicrosoft(t)
	old := outlookCalendarWire
	outlookCalendarWire = f.wire(srv)
	t.Cleanup(func() { outlookCalendarWire = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[OutlookCalendar] = true
		c.Secrets = map[string]Secrets{OutlookCalendar: {ClientID: outlookTestID, ClientSecret: outlookTestSecret}}
	}})
	return e, f
}

func (e *env) connectOutlook(f *fakeMicrosoft) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, OutlookCalendar, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/common/oauth2/v2.0/authorize?") || q.Get("scope") != outlookScopeRead+" offline_access" ||
		q.Get("code_challenge_method") != "S256" || q.Get("client_id") != outlookTestID ||
		q.Get("redirect_uri") != "https://runner.example/oauth/callback/outlookCalendar" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, OutlookCalendar, q.Get("state"), "good-code"); back != "tailorit://connected?provider=outlookCalendar&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE) → list → a write without its scope is 409 → refresh
// (rotated) → a refused refresh needs a reconnect → disconnect deletes the
// token (Microsoft has no revoke).
func TestOutlookCalendarFlow(t *testing.T) {
	e, f := useOutlookCalendar(t)
	ctx := context.Background()
	if _, err := e.call(e.req(OutlookCalendar, "listEvents", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("before connecting: %v", err)
	}
	e.connectOutlook(f)
	c, _, _ := e.mem.Get(ctx, user, OutlookCalendar)
	if c.Status != StatusActive || len(c.Scopes) != 1 || c.Scopes[0] != outlookScopeRead {
		t.Fatalf("connection %+v", c)
	}

	res, err := e.call(e.req(OutlookCalendar, "listEvents", map[string]string{"timeMin": "2026-11-12", "timeMax": "2026-11-14"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	if len(rows) != 3 || rows[0].Title != "会議" || rows[0].Subtitle != "本社" || rows[0].Value != "2026-11-12T10:00:00+09:00" ||
		rows[0].URL == "" || rows[1].Title != gcalNoTitle || rows[1].Value != "2026-11-13" || rows[1].URL != "" ||
		rows[2].Value != "2026-11-13T12:00:00+09:00" || res.Charge.Coins != 0 {
		t.Fatalf("rows %+v", rows)
	}
	if q := f.lastQuery; q.Get("startDateTime") != "2026-11-12T00:00:00+09:00" || q.Get("endDateTime") != "2026-11-14T00:00:00+09:00" ||
		q.Get("$top") != "20" || q.Get("$orderby") != "start/dateTime" || f.lastPrefer != `outlook.timezone="Tokyo Standard Time"` {
		t.Fatalf("query %v prefer %q", q, f.lastPrefer)
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	// the read-only grant cannot write: 409, the run given back
	_, err = e.call(e.req(OutlookCalendar, "createEvent", map[string]string{"title": "x", "start": "2026-11-12T10:00:00+09:00"}))
	if !errors.Is(err, ErrNotConnected) || e.runs() != 1 {
		t.Fatalf("write without scope: %v runs=%d", err, e.runs())
	}

	// expired → refreshed with the granted scopes; the refresh token rotates
	if err := e.svc.Conns.Save(ctx, user, OutlookCalendar, Token{AccessToken: "access-1", RefreshToken: "refresh-1",
		Expiry: e.c.t.Add(-time.Minute), Scopes: []string{outlookScopeRead}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(OutlookCalendar, "listEvents", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err := e.svc.Conns.Token(ctx, user, OutlookCalendar)
	if err != nil || tok.AccessToken != "access-2" || tok.RefreshToken != "refresh-2" || f.refreshScope != outlookScopeRead+" offline_access" {
		t.Fatalf("refreshed %v scope %q", err, f.refreshScope)
	}

	// refresh refused → error: the person reconnects
	f.mu.Lock()
	f.refreshOK = false
	f.mu.Unlock()
	if err := e.svc.Conns.Save(ctx, user, OutlookCalendar, Token{AccessToken: "access-2", RefreshToken: "refresh-1", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(OutlookCalendar, "listEvents", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, OutlookCalendar); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect, then cut: no revoke endpoint, the token is deleted
	e.connectOutlook(f)
	if err := e.svc.Revoke(ctx, user, OutlookCalendar); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, OutlookCalendar); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
}

// A 401 on a call (consent removed at Microsoft) turns the connection to error.
func TestOutlookCalendarRevokedAtProvider(t *testing.T) {
	e, f := useOutlookCalendar(t)
	e.connectOutlook(f)
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, OutlookCalendar, Token{AccessToken: "dead", RefreshToken: "refresh-1", Expiry: e.c.t.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(OutlookCalendar, "listEvents", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, OutlookCalendar); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func outlookTestClient(t *testing.T) (*outlookCalendar, *fakeMicrosoft) {
	f, srv := newFakeMicrosoft(t)
	o, err := newOutlookCalendar(Secrets{ClientID: outlookTestID, ClientSecret: outlookTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return o, f
}

func TestOutlookCalendarCreateEvent(t *testing.T) {
	o, f := outlookTestClient(t)
	ctx := context.Background()
	wide := &Token{AccessToken: "wide", Scopes: []string{outlookScopeWrite}}

	out, err := o.createEvent(ctx, Input{Token: wide, RequestID: "r1", Args: map[string]string{
		"title": "打ち合わせ", "start": "2026-11-12T01:00:00Z", "location": "会議室", "description": "資料"}})
	if err != nil {
		t.Fatal(err)
	}
	tx := outlookTransactionID("r1")
	ev := f.created[tx]
	if out.ResultID == "" || out.Item.Title != "打ち合わせ" || out.Item.Value != "2026-11-12T10:00:00+09:00" || ev["subject"] != "打ち合わせ" ||
		ev["location"].(map[string]any)["displayName"] != "会議室" || ev["body"].(map[string]any)["content"] != "資料" || ev["isAllDay"] != false ||
		ev["start"].(map[string]any)["dateTime"] != "2026-11-12T10:00:00" || ev["start"].(map[string]any)["timeZone"] != "Tokyo Standard Time" ||
		ev["end"].(map[string]any)["dateTime"] != "2026-11-12T11:00:00" {
		t.Fatalf("out %+v ev %v", out, ev)
	}
	// the same request again makes no second event
	again, err := o.createEvent(ctx, Input{Token: wide, RequestID: "r1", Args: map[string]string{"title": "打ち合わせ", "start": "2026-11-12T10:00:00+09:00"}})
	if err != nil || again.ResultID != out.ResultID || len(f.created) != 1 {
		t.Fatalf("retry %+v %v (%d events)", again, err, len(f.created))
	}
	// all day: midnights, the end exclusive
	if _, err := o.createEvent(ctx, Input{Token: wide, RequestID: "r2", Args: map[string]string{"title": "旅行", "start": "2026-11-20", "end": "2026-11-22"}}); err != nil {
		t.Fatal(err)
	}
	ev = f.created[outlookTransactionID("r2")]
	if ev["isAllDay"] != true || ev["start"].(map[string]any)["dateTime"] != "2026-11-20T00:00:00" || ev["end"].(map[string]any)["dateTime"] != "2026-11-23T00:00:00" {
		t.Fatalf("all day %v", ev)
	}
	if len(tx) > 64 || outlookTransactionID("r1") != tx || outlookTransactionID("r2") == tx || outlookTransactionID("") != "" {
		t.Fatalf("transaction id %q", tx)
	}
}

func TestOutlookCalendarArgs(t *testing.T) {
	o, f := outlookTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "wide"}
	bad := []map[string]string{
		{"start": "2026-11-12T10:00:00+09:00"}, // no title
		{"title": "x"},                         // no start
		{"title": "x", "start": "tomorrow"},    // not a time
		{"title": "x", "start": "2026-11-12T10:00:00+09:00", "end": "2026-11-12T09:00:00+09:00"}, // end before start
		{"title": "x", "start": "2026-11-12", "end": "2026-11-12T09:00:00+09:00"},                // date and time mixed
		{"title": "x", "start": "2026-11-12", "end": "2028-11-12"},                               // over a year
		{"title": strings.Repeat("あ", 256), "start": "2026-11-12"},                               // too long
		{"title": "a\x00b", "start": "2026-11-12"},                                               // control char
		{"title": "x", "start": "2026-11-12", "location": strings.Repeat("l", 256)},              // too long
	}
	for i, a := range bad {
		if _, err := o.createEvent(ctx, Input{Token: tok, RequestID: "x", Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("createEvent case %d: %v", i, err)
		}
	}
	badList := []map[string]string{
		{"timeMin": "2026-11-14", "timeMax": "2026-11-12"},
		{"timeMin": "2026-01-01", "timeMax": "2027-06-01"},
		{"maxResults": "0"},
		{"maxResults": "51"},
		{"timeMin": "11/12"},
		{"q": strings.Repeat("q", 257)},
	}
	for i, a := range badList {
		if _, err := o.listEvents(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("listEvents case %d: %v", i, err)
		}
	}
	if len(f.created) != 0 || f.lastQuery != nil {
		t.Fatal("a bad argument reached Graph")
	}
	// no args: now → 7 days, 20 rows
	if _, err := o.listEvents(ctx, Input{Token: tok}); err != nil {
		t.Fatal(err)
	}
	if q := f.lastQuery; q.Get("startDateTime") != "2026-11-11T12:00:00+09:00" || q.Get("endDateTime") != "2026-11-18T12:00:00+09:00" || q.Get("$top") != "20" {
		t.Fatalf("default query %v", q)
	}
	// q filters subject / location (Graph's calendarView has no search)
	out, err := o.listEvents(ctx, Input{Token: tok, Args: map[string]string{"q": "本社", "maxResults": "5"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "会議" || f.lastQuery.Get("$top") != "50" {
		t.Fatalf("q: %+v %v", out.Rows, err)
	}
	out, err = o.listEvents(ctx, Input{Token: tok, Args: map[string]string{"maxResults": "2"}})
	if err != nil || len(out.Rows) != 2 {
		t.Fatalf("maxResults: %+v %v", out.Rows, err)
	}
}

func TestOutlookCalendarErrors(t *testing.T) {
	o, _ := outlookTestClient(t)
	ctx := context.Background()
	if _, err := o.listEvents(ctx, Input{Token: &Token{AccessToken: "narrow"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403 scope: %v", err)
	}
	_, err := o.listEvents(ctx, Input{Token: &Token{AccessToken: "busy"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || pe.Billable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := o.listEvents(ctx, Input{Token: &Token{AccessToken: "nomailbox"}}); !errors.As(err, &pe) || pe.Retryable || pe.Status != 404 {
		t.Fatalf("404: %v", err)
	}
	if _, err := o.listEvents(ctx, Input{Token: &Token{AccessToken: "dead"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	if _, err := o.listEvents(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := o.createEvent(ctx, Input{Token: &Token{AccessToken: "x", Scopes: []string{outlookScopeRead}}, Args: map[string]string{"title": "x", "start": "2026-11-12"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("read scope writes: %v", err)
	}
	if err := o.Revoke(ctx, Token{RefreshToken: "r"}); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := o.Refresh(ctx, Token{AccessToken: "a"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh without a refresh token: %v", err)
	}
	if _, err := o.Exchange(ctx, "bad-code", "v", "https://runner.example/oauth/callback/outlookCalendar"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	if got := outlookScopes("openid Calendars.ReadWrite https://graph.microsoft.com/Calendars.ReadWrite offline_access https://other.example/x"); len(got) != 1 || got[0] != outlookScopeWrite {
		t.Fatalf("scopes %v", got)
	}
}

// The real endpoints are on the allowlist (common tenant); nothing else is
// reachable; without the client id / secret the provider stays off; off by
// default and never in the review build.
func TestOutlookCalendarDefinition(t *testing.T) {
	p, ok := Lookup(OutlookCalendar)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "OUTLOOK_CALENDAR" {
		t.Fatalf("%+v", p)
	}
	w := outlookCalendarWire
	if !strings.Contains(w.TokenURL, "/common/") || !strings.Contains(w.AuthURL, "/common/") {
		t.Fatal("not the common tenant")
	}
	for _, raw := range []string{w.TokenURL, w.APIBase + "/me/calendarView", w.APIBase + "/me/events"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	for _, o := range p.Ops {
		for _, s := range o.Scopes {
			if s != outlookScopeRead && s != outlookScopeWrite {
				t.Errorf("%s asks for %s", o.Name, s)
			}
		}
	}
	o, err := newOutlookCalendar(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil {
		t.Fatal("built without a secret")
	}
	c, _ := ConfigFromEnv(func(string) string { return "" })
	if c.Enabled[OutlookCalendar] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
