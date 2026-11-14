package extapi

import (
	"context"
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
	stravaTestID     = "st-client"
	stravaTestSecret = "st-secret"
)

// stravaTestExpiry is the first token's expires_at (6 hours after the test clock).
var stravaTestExpiry = time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

// fakeStrava is Strava's OAuth and API v3 on httptest (no live calls).
type fakeStrava struct {
	url string

	mu         sync.Mutex
	refreshes  int
	rotateNext int
	deauthed   []string
	lastPath   string
	lastQuery  url.Values
	paths      []string
}

func newFakeStrava(t *testing.T) (*fakeStrava, *httptest.Server) {
	f := &fakeStrava{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != stravaTestID || r.Form.Get("client_secret") != stravaTestSecret {
			writeJSON(w, 401, map[string]any{"message": "Authorization Error"})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "st-code" || r.Form.Get("code_verifier") != "" {
				writeJSON(w, 400, map[string]any{"message": "Bad Request", "errors": []map[string]string{{"resource": "AuthorizationCode", "field": "code", "code": "invalid"}}})
				return
			}
			writeJSON(w, 200, map[string]any{"token_type": "Bearer", "expires_at": stravaTestExpiry.Unix(), "expires_in": 21600,
				"refresh_token": "st-refresh-0", "access_token": "st-access-0", "athlete": map[string]any{"id": 12345}})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "st-refresh-"+strconv.Itoa(f.rotateNext) {
				writeJSON(w, 400, map[string]any{"message": "Bad Request", "errors": []map[string]string{{"resource": "RefreshToken", "field": "refresh_token", "code": "invalid"}}})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			writeJSON(w, 200, map[string]any{"token_type": "Bearer", "expires_at": stravaTestExpiry.Add(6 * time.Hour).Unix(), "expires_in": 21600, "refresh_token": "st-refresh-" + n, "access_token": "st-access-" + n})
		default:
			writeJSON(w, 400, map[string]any{"message": "Bad Request"})
		}
	})
	mux.HandleFunc("POST /oauth/deauthorize", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		tok := r.Form.Get("access_token")
		if !strings.HasPrefix(tok, "st-access-") {
			writeJSON(w, 401, map[string]any{"message": "Authorization Error"})
			return
		}
		f.mu.Lock()
		f.deauthed = append(f.deauthed, tok)
		f.mu.Unlock()
		writeJSON(w, 200, map[string]any{"access_token": tok})
	})
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "noscope":
			writeJSON(w, 401, map[string]any{"message": "Authorization Error", "errors": []map[string]string{{"resource": "AccessToken", "field": "activity:read_permission", "code": "missing"}}})
			return false
		case tok == "busy":
			writeJSON(w, 429, map[string]any{"message": "Rate Limit Exceeded"})
			return false
		case tok == "down":
			writeJSON(w, 503, map[string]any{"message": "Service Unavailable"})
			return false
		case !strings.HasPrefix(tok, "st-access-"):
			writeJSON(w, 401, map[string]any{"message": "Authorization Error", "errors": []map[string]string{{"resource": "AccessToken", "field": "access_token", "code": "invalid"}}})
			return false
		}
		f.mu.Lock()
		f.lastPath, f.lastQuery = r.URL.Path, r.URL.Query()
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		return true
	}
	mux.HandleFunc("GET /api/v3/athlete/activities", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, 200, []map[string]any{
			{"id": 9876543210, "name": "朝ラン", "distance": 10523.4, "moving_time": 3125, "type": "Run", "sport_type": "Run", "start_date": "2026-11-10T22:30:00Z", "total_elevation_gain": 42.0},
			{"id": 9876543211, "name": "通勤", "distance": 7000, "moving_time": 1500, "type": "Ride", "sport_type": "EBikeRide", "start_date": "2026-11-09T23:05:00Z"},
			{"id": 9876543212, "name": "ピラティス", "distance": 0, "moving_time": 2700, "type": "Workout", "sport_type": "Pilates", "start_date": "bad"},
			{"id": 0, "name": ""},
		})
	})
	mux.HandleFunc("GET /api/v3/athlete", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		writeJSON(w, 200, map[string]any{"id": 12345, "firstname": "太郎"})
	})
	mux.HandleFunc("GET /api/v3/athletes/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		if r.PathValue("id") != "12345" {
			writeJSON(w, 404, map[string]any{"message": "Record Not Found"})
			return
		}
		writeJSON(w, 200, map[string]any{
			"recent_ride_totals": map[string]any{"count": 3, "distance": 120340.5, "moving_time": 18600, "elevation_gain": 1203.4},
			"recent_run_totals":  map[string]any{"count": 5, "distance": 42195, "moving_time": 12000, "elevation_gain": 0},
			"recent_swim_totals": map[string]any{"count": 0, "distance": 0, "moving_time": 0, "elevation_gain": 0},
			"ytd_run_totals":     map[string]any{"count": 99},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeStrava) wire(srv *httptest.Server) stravaWire {
	return stravaWire{AuthURL: srv.URL + "/oauth/authorize", TokenURL: srv.URL + "/oauth/token", DeauthURL: srv.URL + "/oauth/deauthorize",
		APIBase: srv.URL + "/api/v3", ActivityLinks: "https://www.strava.com/activities/", Client: srv.Client(),
		Now: func() time.Time { return time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC) }} // the env clock (12:00 JST)
}

func useStrava(t *testing.T) (*env, *fakeStrava) {
	f, srv := newFakeStrava(t)
	old := stravaWireDefault
	stravaWireDefault = f.wire(srv)
	t.Cleanup(func() { stravaWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Strava] = true
		c.Secrets = map[string]Secrets{Strava: {ClientID: stravaTestID, ClientSecret: stravaTestSecret}}
	}})
	return e, f
}

// connectStrava connects user; want is the scope param (comma separated).
func (e *env) connectStrava(f *fakeStrava, scopes []string, want string) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Strava, scopes)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth/authorize?") || q.Get("scope") != want || q.Get("response_type") != "code" || q.Get("approval_prompt") != "auto" ||
		q.Get("client_id") != stravaTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/strava" || q.Get("client_secret") != "" || q.Has("code_challenge") {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Strava, q.Get("state"), "st-code"); back != "tailorit://connected?provider=strava&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

func TestStravaRegistered(t *testing.T) {
	p, ok := Lookup(Strava)
	if !ok || p.Billing != BillingFree || !p.NeedsOAuth || p.Fake || p.NameJa == "" || p.GlobalDaily != 0 ||
		!slices.Equal(p.Hosts, []string{"https://www.strava.com"}) {
		t.Fatalf("strava %+v", p)
	}
	if slices.Contains(ReviewBuildFeatures(), "externalCall.strava") {
		t.Fatal("an OAuth provider is not in the review build")
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL != 0 || !slices.Equal(op.Scopes, []string{"activity:read"}) {
			t.Fatalf("%s: read ops, never cached, activity:read", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_STRAVA_ENABLED": "1", "EXT_STRAVA_CLIENT_ID": "id", "EXT_STRAVA_CLIENT_SECRET": "s", "EXT_GLOBAL_DAILY_STRAVA": "900"}[k]
	})
	if err != nil || !cfg.Enabled[Strava] || cfg.Secrets[Strava].ClientSecret != "s" || cfg.Limits[Strava].GlobalDaily != 900 {
		t.Fatalf("env: %v %v", cfg.Enabled, err)
	}
	if _, err := newStrava(Secrets{ClientID: "id"}, stravaWireDefault); err == nil {
		t.Fatal("no client secret")
	}
	if StravaAttribution != "Powered by Strava" {
		t.Fatal(StravaAttribution)
	}
}

// connect (read,activity:read) → activities → stats (two requests) →
// refresh (rotated, expires_at) → refused refresh → revoke (deauthorize).
func TestStravaFlow(t *testing.T) {
	e, f := useStrava(t)
	ctx := context.Background()
	e.connectStrava(f, nil, "read,activity:read")
	tok, err := e.svc.Conns.Token(ctx, user, Strava)
	if err != nil || !slices.Equal(tok.Scopes, []string{"activity:read"}) || !tok.Expiry.Equal(stravaTestExpiry) {
		t.Fatalf("token after connect: %+v %v", tok, err)
	}

	res, err := e.call(e.req(Strava, "activities", map[string]string{"limit": "10"}))
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Title: "朝ラン", Subtitle: "ラン・11/11 07:30", Value: "10.52 km / 52分", URL: "https://www.strava.com/activities/9876543210"},
		{Title: "通勤", Subtitle: "E バイク・11/10 08:05", Value: "7 km / 25分", URL: "https://www.strava.com/activities/9876543211"},
		{Title: "ピラティス", Subtitle: "Pilates", Value: "0 km / 45分", URL: "https://www.strava.com/activities/9876543212"},
	}
	if !slices.Equal(res.Output.Rows, want) {
		t.Fatalf("rows %+v", res.Output.Rows)
	}
	if f.lastPath != "/api/v3/athlete/activities" || f.lastQuery.Get("per_page") != "10" {
		t.Fatalf("request %s %v", f.lastPath, f.lastQuery)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}
	if _, err := e.call(e.req(Strava, "activities", nil)); err != nil || f.lastQuery.Has("after") || f.lastQuery.Get("per_page") != "20" {
		t.Fatalf("defaults: %v %v", err, f.lastQuery)
	}
	// after (JST day) is applied to the newest ones here, not sent (Strava
	// answers oldest first with it): only 朝ラン (11/11 07:30 JST) is on or after 11/11
	res, err = e.call(e.req(Strava, "activities", map[string]string{"after": "2026-11-11"}))
	if err != nil || f.lastQuery.Has("after") || len(res.Output.Rows) != 1 || res.Output.Rows[0] != want[0] {
		t.Fatalf("after: %v %v %+v", err, f.lastQuery, res.Output.Rows)
	}

	f.paths = nil
	res, err = e.call(e.req(Strava, "stats", nil))
	if err != nil {
		t.Fatal(err)
	}
	want = []Row{
		{Title: "最近 4 週間のライド", Subtitle: "3 回・獲得標高 1203 m", Value: "120.34 km / 5時間10分"},
		{Title: "最近 4 週間のラン", Subtitle: "5 回", Value: "42.2 km / 3時間20分"},
		{Title: "最近 4 週間のスイム", Subtitle: "0 回", Value: "0 km / 0分"},
	}
	if !slices.Equal(res.Output.Rows, want) || !slices.Equal(f.paths, []string{"/api/v3/athlete", "/api/v3/athletes/12345/stats"}) {
		t.Fatalf("stats %+v %v", res.Output.Rows, f.paths)
	}

	// expired → refreshed with the rotated refresh token
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Strava, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Strava, "activities", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Strava)
	if err != nil || tok.AccessToken != "st-access-1" || tok.RefreshToken != "st-refresh-1" || f.refreshes != 1 || !slices.Equal(tok.Scopes, []string{"activity:read"}) {
		t.Fatalf("refreshed: %v %+v refreshes=%d", err, tok, f.refreshes)
	}

	// a rotated-away refresh token is invalid_grant → error
	if err := e.svc.Conns.Save(ctx, user, Strava, Token{AccessToken: "st-access-0", RefreshToken: "st-refresh-0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Strava, "activities", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("invalid_grant: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Strava); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect and cut: deauthorized at Strava
	e.connectStrava(f, nil, "read,activity:read")
	if err := e.svc.Revoke(ctx, user, Strava); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Strava); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
	if !slices.Equal(f.deauthed, []string{"st-access-0"}) {
		t.Fatalf("deauthorized %v", f.deauthed)
	}
}

// 401 turns the connection to error; a 401 naming a missing permission is
// 409 connect again; 429 / 503 are retryable upstream errors. None keeps the run.
func TestStravaRefused(t *testing.T) {
	e, f := useStrava(t)
	e.connectStrava(f, nil, "read,activity:read")
	ctx := context.Background()
	for _, c := range []struct {
		token  string
		reason string
		status string
	}{
		{"dead", ReasonNotConnected, StatusError},
		{"noscope", ReasonNotConnected, StatusActive},
		{"busy", ReasonUpstream, StatusActive},
		{"down", ReasonUpstream, StatusActive},
	} {
		if err := e.svc.Conns.Save(ctx, user, Strava, Token{AccessToken: c.token}); err != nil {
			t.Fatal(err)
		}
		_, err := e.call(e.req(Strava, "stats", nil))
		if Reason(err) != c.reason || e.runs() != 0 {
			t.Fatalf("%s: %v (%s) runs=%d", c.token, err, Reason(err), e.runs())
		}
		if conn, _, _ := e.mem.Get(ctx, user, Strava); conn.Status != c.status {
			t.Fatalf("%s: status %q, want %q", c.token, conn.Status, c.status)
		}
		if c.token == "busy" {
			var pe *ProviderError
			if !errors.As(err, &pe) || !pe.Retryable || pe.Status != 429 {
				t.Fatalf("429: %v", err)
			}
		}
	}
}

func TestStravaCheck(t *testing.T) {
	p, _ := Lookup(Strava)
	future := time.Now().In(jst).AddDate(0, 0, 2).Format(time.DateOnly)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"activities", nil, true},
		{"activities", map[string]string{"limit": "50", "after": "2026-01-01"}, true},
		{"activities", map[string]string{"limit": "0"}, false},
		{"activities", map[string]string{"limit": "51"}, false},
		{"activities", map[string]string{"after": "2026/01/01"}, false},
		{"activities", map[string]string{"after": "2026-02-30"}, false},
		{"activities", map[string]string{"after": future}, false},
		{"activities", map[string]string{"page": "2"}, false},
		{"stats", nil, true},
		{"stats", map[string]string{"limit": "1"}, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%s %v: %v", c.op, c.args, err)
		}
	}
}

func TestStravaFormats(t *testing.T) {
	for sec, want := range map[int64]string{0: "0分", 59: "0分", 3125: "52分", 3600: "1時間00分", 18600: "5時間10分", -1: ""} {
		if got := stravaDuration(sec); got != want {
			t.Errorf("%d: %q, want %q", sec, got, want)
		}
	}
	for m, want := range map[float64]string{0: "0 km", 10523.4: "10.52 km", 42195: "42.2 km", 999.9: "1 km", -1: ""} {
		if got := stravaKm(m); got != want {
			t.Errorf("%v: %q, want %q", m, got, want)
		}
	}
	if stravaSport("", "Run") != "ラン" || stravaSport("TrailRun", "Run") != "トレイルラン" || stravaSport("Pickleball", "Workout") != "Pickleball" {
		t.Fatal("sport names")
	}
}

// The real client reaches www.strava.com only.
func TestStravaEgress(t *testing.T) {
	f, srv := newFakeStrava(t)
	w := f.wire(srv)
	w.Client = nil
	c, err := newStrava(Secrets{ClientID: stravaTestID, ClientSecret: stravaTestSecret}, w)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.stats(context.Background(), Input{Token: &Token{AccessToken: "st-access-0"}})
	if !errors.Is(err, ErrEgress) || f.lastPath != "" {
		t.Fatalf("local server reached: %v", err)
	}
}

// A transport failure (API and token endpoint) is the shared upstream error:
// retryable, 502, no token or secret in it.
func TestStravaTransportError(t *testing.T) {
	f, srv := newFakeStrava(t)
	c, err := newStrava(Secrets{ClientID: stravaTestID, ClientSecret: stravaTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	_, err = c.stats(context.Background(), Input{Token: &Token{AccessToken: "st-access-0"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream || StatusOf(Reason(err)) != http.StatusBadGateway {
		t.Fatalf("api transport: %v", err)
	}
	if strings.Contains(err.Error(), "st-access-0") {
		t.Fatalf("token in the error: %v", err)
	}
	_, err = c.Refresh(context.Background(), Token{AccessToken: "st-access-0", RefreshToken: "st-refresh-0"})
	if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("token transport: %v", err)
	}
	if strings.Contains(err.Error(), stravaTestSecret) || strings.Contains(err.Error(), "st-refresh-0") {
		t.Fatalf("secret in the error: %v", err)
	}
}

// The API Agreement: a person's data only to that person, the credit, and
// no AI until Strava confirms it.
func TestStravaTerms(t *testing.T) {
	p, _ := Lookup(Strava)
	if p.Terms.NoCharge || !p.Terms.NoAI || !p.Terms.ShowOwnDataOnly || p.Terms.Attribution != StravaAttribution {
		t.Fatalf("%+v", p.Terms)
	}
}
