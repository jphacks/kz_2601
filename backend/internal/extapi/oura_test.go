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
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	ouraTestID     = "oura-client"
	ouraTestSecret = "oura-secret"
)

// fakeOura is Oura's OAuth and API v2 on httptest (no live calls). The
// health values are made up.
type fakeOura struct {
	url string

	mu        sync.Mutex
	challenge string
	refreshes int
	refresh   string // the one refresh token Oura takes now (single use)
	revoked   []string
	queries   []url.Values
	paths     []string
	status    int // answer the API with this status when set
}

func newFakeOura(t *testing.T) (*fakeOura, *httptest.Server) {
	f := &fakeOura{refresh: "oura-refresh-0"}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != ouraTestID || r.Form.Get("client_secret") != ouraTestSecret {
			writeJSON(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "oura-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.refresh = "oura-refresh-0"
			writeJSON(w, 200, map[string]any{"token_type": "bearer", "access_token": "oura-access-0", "refresh_token": f.refresh, "expires_in": 86400})
		case "refresh_token":
			if r.Form.Get("refresh_token") != f.refresh {
				writeJSON(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			f.refreshes++
			n := strconv.Itoa(f.refreshes)
			f.refresh = "oura-refresh-" + n
			writeJSON(w, 200, map[string]any{"token_type": "bearer", "access_token": "oura-access-" + n, "refresh_token": f.refresh, "expires_in": 86400})
		default:
			writeJSON(w, 400, map[string]string{"error": "unsupported_grant_type"})
		}
	})
	mux.HandleFunc("GET /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.revoked = append(f.revoked, r.URL.Query().Get("access_token"))
		w.WriteHeader(200)
	})
	pages := map[string][]map[string]any{
		// unordered, a day outside the range, a null score, paged
		"/v2/usercollection/daily_sleep": {
			{"data": []map[string]any{
				{"day": "2026-11-10", "score": 82, "contributors": map[string]any{"total_sleep": 91, "efficiency": 88, "deep_sleep": 77, "rem_sleep": 66}},
				{"day": "2026-11-12", "score": 70},
			}, "next_token": "page2"},
			{"data": []map[string]any{
				{"day": "2026-11-09", "score": nil, "contributors": map[string]any{"total_sleep": nil, "efficiency": 51}},
			}, "next_token": nil},
		},
		"/v2/usercollection/daily_readiness": {
			{"data": []map[string]any{
				{"day": "2026-11-11", "score": 79, "temperature_deviation": -0.27, "contributors": map[string]any{"hrv_balance": 74, "resting_heart_rate": 93, "recovery_index": 100}},
			}},
		},
		"/v2/usercollection/daily_activity": {
			{"data": []map[string]any{
				{"day": "2026-11-11", "score": 85, "steps": 98765, "active_calories": 432},
			}},
		},
	}
	api := func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+f.access() {
			writeJSON(w, 401, map[string]string{"detail": "unauthorized"})
			return
		}
		if f.status != 0 {
			writeJSON(w, f.status, map[string]string{"detail": "no"})
			return
		}
		f.paths = append(f.paths, r.URL.Path)
		f.queries = append(f.queries, r.URL.Query())
		p := pages[r.URL.Path]
		i := 0
		if r.URL.Query().Get("next_token") == "page2" {
			i = 1
		}
		writeJSON(w, 200, p[i])
	}
	for path := range pages {
		mux.HandleFunc("GET "+path, api)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

// access is the access token that goes with the current refresh token.
func (f *fakeOura) access() string {
	return "oura-access-" + strings.TrimPrefix(f.refresh, "oura-refresh-")
}

func (f *fakeOura) wire(srv *httptest.Server) ouraWire {
	return ouraWire{AuthURL: srv.URL + "/oauth/authorize", TokenURL: srv.URL + "/oauth/token", RevokeURL: srv.URL + "/oauth/revoke",
		APIBase: srv.URL + "/v2/usercollection", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) }} // 21:00 JST
}

func useOura(t *testing.T) (*env, *fakeOura) {
	f, srv := newFakeOura(t)
	old := ouraWireDefault
	ouraWireDefault = f.wire(srv)
	t.Cleanup(func() { ouraWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Oura] = true
		c.Secrets = map[string]Secrets{Oura: {ClientID: ouraTestID, ClientSecret: ouraTestSecret}}
	}})
	return e, f
}

func (e *env) connectOura(f *fakeOura) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Oura, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth/authorize?") || q.Get("scope") != "daily" || q.Get("code_challenge_method") != "S256" ||
		q.Get("client_id") != ouraTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/oura" || q.Get("client_secret") != "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.challenge = q.Get("code_challenge")
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Oura, q.Get("state"), "oura-code"); back != "tailorit://connected?provider=oura&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect (PKCE, daily only) → the three ops → refresh (single use) → a
// refused refresh needs a reconnect → revoke. No value reaches the logs.
func TestOuraFlow(t *testing.T) {
	e, f := useOura(t)
	ctx := context.Background()
	e.connectOura(f)
	if c, _, _ := e.mem.Get(ctx, user, Oura); !slices.Equal(c.Scopes, []string{"daily"}) {
		t.Fatalf("scopes %v", c.Scopes)
	}

	res, err := e.call(e.req(Oura, "dailySleep", map[string]string{"from": "2026-11-05", "to": "2026-11-11"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	want := []Row{
		{Title: "2026-11-09", Subtitle: "効率 51"},
		{Title: "2026-11-10", Value: "82", Subtitle: "合計睡眠 91 · 効率 88 · 深い睡眠 77 · レム睡眠 66"},
	}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows %+v", rows)
	}
	if len(f.queries) != 2 || f.queries[0].Get("start_date") != "2026-11-05" || f.queries[0].Get("end_date") != "2026-11-12" ||
		f.queries[0].Get("fields") != "day,score,contributors" || f.queries[1].Get("next_token") != "page2" {
		t.Fatalf("requests %v", f.queries)
	}
	if res.Output.Cells != nil || e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("cells %v runs %d", res.Output.Cells, e.runs())
	}

	res, err = e.call(e.req(Oura, "dailyReadiness", nil)) // the last 7 days to today (JST)
	if err != nil {
		t.Fatal(err)
	}
	if q := f.queries[len(f.queries)-1]; q.Get("start_date") != "2026-11-05" || q.Get("end_date") != "2026-11-12" {
		t.Fatalf("default range %v", q)
	}
	if r := res.Output.Rows; len(r) != 1 || r[0] != (Row{Title: "2026-11-11", Value: "79", Subtitle: "体温 -0.3℃ · HRV バランス 74 · 安静時心拍 93 · 回復 100"}) {
		t.Fatalf("readiness %+v", r)
	}

	res, err = e.call(e.req(Oura, "dailyActivity", map[string]string{"from": "2026-11-11"}))
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Output.Rows; len(r) != 1 || r[0] != (Row{Title: "2026-11-11", Value: "85", Subtitle: "歩数 98765 · 活動 432 kcal"}) {
		t.Fatalf("activity %+v", r)
	}
	if f.paths[len(f.paths)-1] != "/v2/usercollection/daily_activity" || f.queries[len(f.queries)-1].Get("fields") != "day,score,steps,active_calories" {
		t.Fatalf("activity request %v %v", f.paths, f.queries)
	}

	// expired → refreshed; the used refresh token is gone
	tok, _ := e.svc.Conns.Token(ctx, user, Oura)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Oura, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Oura, "dailySleep", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Oura)
	if err != nil || tok.AccessToken != "oura-access-1" || tok.RefreshToken != "oura-refresh-1" || f.refreshes != 1 {
		t.Fatalf("refreshed: %v refreshes=%d", err, f.refreshes)
	}
	if err := e.svc.Conns.Save(ctx, user, Oura, Token{AccessToken: "oura-access-0", RefreshToken: "oura-refresh-0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Oura, "dailySleep", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("used refresh token: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Oura); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect and cut: the access token is revoked at Oura
	e.connectOura(f)
	if err := e.svc.Revoke(ctx, user, Oura); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Oura); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if len(f.revoked) != 1 || f.revoked[0] != "oura-access-0" {
		t.Fatalf("revoked %v", f.revoked)
	}

	logs := e.logs.String()
	for _, v := range []string{"98765", "432", "-0.27", "oura-access", "oura-refresh"} {
		if strings.Contains(logs, v) {
			t.Fatalf("logs carry %q: %s", v, logs)
		}
	}
}

// 401 turns the connection to error; 403 (membership ended) and 429 are
// provider errors that give the run back.
func TestOuraErrors(t *testing.T) {
	e, f := useOura(t)
	ctx := context.Background()
	e.connectOura(f)
	for _, st := range []int{403, 429, 500} {
		f.mu.Lock()
		f.status = st
		f.mu.Unlock()
		_, err := e.call(e.req(Oura, "dailySleep", nil))
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Status != st || pe.Retryable != (st != 403) || Reason(err) != ReasonUpstream || strings.Contains(err.Error(), "no\"") {
			t.Fatalf("%d: %v (%s)", st, err, Reason(err))
		}
		if e.runs() != 0 {
			t.Fatalf("%d: runs %d", st, e.runs())
		}
	}
	f.mu.Lock()
	f.status = 0
	f.mu.Unlock()
	if err := e.svc.Conns.Save(ctx, user, Oura, Token{AccessToken: "dead"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Oura, "dailySleep", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Oura); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func TestOuraDates(t *testing.T) {
	now := time.Date(2026, 11, 11, 16, 0, 0, 0, time.UTC) // 2026-11-12 01:00 JST
	ok := []struct {
		args     map[string]string
		from, to string
	}{
		{nil, "2026-11-06", "2026-11-12"},
		{map[string]string{"to": "2026-11-01"}, "2026-10-26", "2026-11-01"},
		{map[string]string{"from": "2026-10-13", "to": "2026-11-12"}, "2026-10-13", "2026-11-12"}, // 31 days
		{map[string]string{"from": "2026-11-12", "to": "2026-11-12"}, "2026-11-12", "2026-11-12"},
	}
	for _, c := range ok {
		from, to, err := ouraDates(c.args, now)
		if err != nil || from.Format(time.DateOnly) != c.from || to.Format(time.DateOnly) != c.to {
			t.Errorf("%v: %v %v %v", c.args, from, to, err)
		}
	}
	bad := []map[string]string{
		{"to": "2026-11-13"},                         // after today (JST)
		{"from": "2026-10-12", "to": "2026-11-12"},   // 32 days
		{"from": "2026-11-10", "to": "2026-11-09"},   // from after to
		{"from": "11/10"},                            // not a date
		{"from": "2014-12-31", "to": "2015-01-01"},   // before 2015
		{"from": "2026-11-10T00:00:00", "to": "now"}, // not a date
	}
	for _, a := range bad {
		if _, _, err := ouraDates(a, now); !errors.Is(err, ErrUnknown) {
			t.Errorf("%v: %v", a, err)
		}
	}
}

// The registered provider: read only, daily scope only, no cache, only
// Oura's API host, and off unless EXT_OURA_ENABLED.
func TestOuraProvider(t *testing.T) {
	p, ok := Lookup(Oura)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || !slices.Equal(p.Hosts, []string{"https://api.ouraring.com"}) {
		t.Fatalf("provider %+v", p)
	}
	for _, name := range []string{"dailySleep", "dailyReadiness", "dailyActivity"} {
		o, ok := p.Op(name)
		if !ok || o.Write || o.CacheTTL != 0 || !slices.Equal(o.Scopes, []string{"daily"}) || !slices.Equal(o.Args, []string{"from", "to"}) {
			t.Fatalf("%s: %+v", name, o)
		}
		if _, err := p.CheckArgs(name, map[string]string{"from": "2026-11-01", "to": "2026-11-02"}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, a := range []map[string]string{{"day": "2026-11-01"}, {"from": "yesterday"}, {"from": "2020-01-01", "to": "2020-03-01"}} {
			if _, err := p.CheckArgs(name, a); !errors.Is(err, ErrUnknown) {
				t.Fatalf("%s %v: %v", name, a, err)
			}
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_OURA_CLIENT_ID" || k == "EXT_OURA_CLIENT_SECRET" {
			return "x"
		}
		return ""
	})
	if err != nil || cfg.Enabled[Oura] {
		t.Fatalf("on by default: %v %v", cfg.Enabled, err)
	}
	cfg, err = ConfigFromEnv(func(k string) string {
		if k == "EXT_OURA_ENABLED" {
			return "1"
		}
		return ""
	})
	if err != nil || !cfg.Enabled[Oura] {
		t.Fatalf("EXT_OURA_ENABLED: %v %v", cfg.Enabled, err)
	}
	if _, err := newOura(Secrets{}, ouraWireDefault); err == nil {
		t.Fatal("no client id / secret")
	}
}

// AuthURL sends daily only, whatever is asked (an empty scope is every
// scope at Oura).
func TestOuraAuthURLScope(t *testing.T) {
	c, err := newOura(Secrets{ClientID: ouraTestID, ClientSecret: ouraTestSecret}, ouraWireDefault)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range [][]string{nil, {"daily"}, {"daily", "personal", "heartrate"}} {
		u, _ := url.Parse(c.AuthURL("st", "ch", "https://runner.example/oauth/callback/oura", s))
		if u.Host != "cloud.ouraring.com" || u.Query().Get("scope") != "daily" {
			t.Fatalf("%v: %s", s, u)
		}
	}
}
