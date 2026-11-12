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
	withingsTestID     = "wi-client"
	withingsTestSecret = "wi-secret"
)

// fakeWithings is Withings' OAuth and measure API on httptest (no live
// calls). Withings answers HTTP 200 with its own status in the body.
type fakeWithings struct {
	url string

	mu         sync.Mutex
	refreshes  int
	rotateNext int
	calls      int
	lastForm   url.Values
	groups     []map[string]any
	pageSize   int // 0 = all on one page
}

var withingsTestNow = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

func newFakeWithings(t *testing.T) (*fakeWithings, *httptest.Server) {
	day := func(d int) int64 { return withingsTestNow.AddDate(0, 0, -d).Unix() }
	m := func(v int64, typ, unit int) map[string]any {
		return map[string]any{"value": v, "type": typ, "unit": unit}
	}
	f := &fakeWithings{groups: []map[string]any{
		// oldest first on purpose: the provider sorts
		{"grpid": 1, "date": day(3), "attrib": 0, "category": 1, "measures": []any{m(65750, 1, -3), m(213, 6, -1)}},
		{"grpid": 2, "date": day(2), "attrib": 1, "category": 1, "measures": []any{m(80000, 1, -3)}}, // someone else's?
		{"grpid": 3, "date": day(1), "attrib": 2, "category": 1, "measures": []any{m(655, 1, -1)}},   // entered by hand
		{"grpid": 4, "date": day(1) + 60, "attrib": 0, "category": 1, "measures": []any{m(128, 10, 0), m(82, 9, 0), m(65, 11, 0)}},
		{"grpid": 5, "date": day(0) - 60, "attrib": 0, "category": 1, "measures": []any{m(121, 10, 0), m(79, 9, 0)}},
		{"grpid": 6, "date": day(0) - 30, "attrib": 0, "category": 1, "measures": []any{m(70, 11, 0)}},    // pulse only
		{"grpid": 7, "date": day(0) - 10, "attrib": 0, "category": 2, "measures": []any{m(60000, 1, -3)}}, // an objective
	}}
	mux := http.NewServeMux()
	answer := func(w http.ResponseWriter, status int, body any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "body": body})
	}
	mux.HandleFunc("POST /v2/oauth2", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("action") != "requesttoken" || r.Form.Get("client_id") != withingsTestID || r.Form.Get("client_secret") != withingsTestSecret {
			answer(w, 503, map[string]any{})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "wi-code" || r.Form.Get("redirect_uri") != "https://runner.example/oauth/callback/withings" {
				answer(w, 503, map[string]any{})
				return
			}
			answer(w, 0, map[string]any{"userid": "363", "access_token": "wi-access-0", "refresh_token": "wi-refresh-0", "expires_in": 10800, "scope": "user.metrics", "token_type": "Bearer"})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "wi-refresh-"+strconv.Itoa(f.rotateNext) {
				answer(w, 503, map[string]any{})
				return
			}
			f.rotateNext++
			f.refreshes++
			n := strconv.Itoa(f.rotateNext)
			answer(w, 0, map[string]any{"userid": "363", "access_token": "wi-access-" + n, "refresh_token": "wi-refresh-" + n, "expires_in": 10800, "scope": "user.metrics", "token_type": "Bearer"})
		default:
			answer(w, 503, map[string]any{})
		}
	})
	mux.HandleFunc("POST /measure", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); {
		case tok == "busy":
			answer(w, 601, map[string]any{})
			return
		case tok == "down":
			w.WriteHeader(http.StatusBadGateway)
			return
		case tok == "odd":
			answer(w, 2554, map[string]any{})
			return
		case !strings.HasPrefix(tok, "wi-access-"):
			answer(w, 401, map[string]any{})
			return
		}
		if r.Form.Get("action") != "getmeas" || r.Form.Get("category") != "1" {
			answer(w, 503, map[string]any{})
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		f.lastForm = r.Form
		types := strings.Split(r.Form.Get("meastypes"), ",")
		start, _ := strconv.ParseInt(r.Form.Get("startdate"), 10, 64)
		end, _ := strconv.ParseInt(r.Form.Get("enddate"), 10, 64)
		var out []map[string]any
		for _, g := range f.groups {
			d := g["date"].(int64)
			if d < start || d > end {
				continue
			}
			var ms []any
			for _, x := range g["measures"].([]any) {
				if slices.Contains(types, strconv.Itoa(x.(map[string]any)["type"].(int))) {
					ms = append(ms, x)
				}
			}
			if len(ms) > 0 {
				c := map[string]any{}
				for k, v := range g {
					c[k] = v
				}
				c["measures"] = ms
				out = append(out, c)
			}
		}
		off, _ := strconv.Atoi(r.Form.Get("offset"))
		more, next := 0, 0
		if f.pageSize > 0 {
			out = out[min(off, len(out)):]
			if len(out) > f.pageSize {
				out, more, next = out[:f.pageSize], 1, off+f.pageSize
			}
		}
		answer(w, 0, map[string]any{"updatetime": end, "timezone": "Asia/Tokyo", "measuregrps": out, "more": more, "offset": next})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeWithings) wire(srv *httptest.Server) withingsWire {
	return withingsWire{AuthURL: srv.URL + "/oauth2_user/authorize2", TokenURL: srv.URL + "/v2/oauth2", APIBase: srv.URL,
		Client: srv.Client(), Now: func() time.Time { return withingsTestNow }}
}

func useWithings(t *testing.T) (*env, *fakeWithings) {
	f, srv := newFakeWithings(t)
	old := withingsWireDefault
	withingsWireDefault = f.wire(srv)
	t.Cleanup(func() { withingsWireDefault = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Withings] = true
		c.Secrets = map[string]Secrets{Withings: {ClientID: withingsTestID, ClientSecret: withingsTestSecret}}
	}})
	return e, f
}

func (e *env) connectWithings(f *fakeWithings) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Withings, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if !strings.HasPrefix(auth, f.url+"/oauth2_user/authorize2?") || q.Get("scope") != "user.metrics" || q.Get("response_type") != "code" ||
		q.Get("client_id") != withingsTestID || q.Get("redirect_uri") != "https://runner.example/oauth/callback/withings" ||
		q.Get("client_secret") != "" || q.Get("code_challenge") != "" || q.Get("state") == "" {
		e.t.Fatalf("consent url %s", auth)
	}
	f.mu.Lock()
	f.rotateNext = 0
	f.mu.Unlock()
	if back := e.svc.Callback(ctx, Withings, q.Get("state"), "wi-code"); back != "tailorit://connected?provider=withings&status=ok" {
		e.t.Fatalf("callback → %s", back)
	}
}

// connect → weight / bloodPressure → refresh (rotated) → a refused refresh
// needs a reconnect → cut (no revoke at Withings; tokens deleted).
func TestWithingsFlow(t *testing.T) {
	e, f := useWithings(t)
	ctx := context.Background()
	e.connectWithings(f)

	res, err := e.call(e.req(Withings, "weight", nil))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	// newest first; attrib 1 (ambiguous) and category 2 (objective) left out
	want := []Row{
		{Title: "2026/11/10 21:00", Value: "65.5 kg"},
		{Title: "2026/11/8 21:00", Subtitle: "体脂肪率 21.3%", Value: "65.75 kg"},
	}
	if len(rows) != len(want) || rows[0] != want[0] || rows[1] != want[1] {
		t.Fatalf("rows %+v", rows)
	}
	// display only: no Cells for the scheduled import (#1712)
	if res.Output.Cells != nil {
		t.Fatalf("cells %v", res.Output.Cells)
	}
	if f.lastForm.Get("meastypes") != "1,6" || f.lastForm.Get("enddate") != strconv.FormatInt(withingsTestNow.Unix(), 10) ||
		f.lastForm.Get("startdate") != strconv.FormatInt(withingsTestNow.AddDate(0, 0, -30).Unix(), 10) {
		t.Fatalf("form %v", f.lastForm)
	}
	if e.runs() != 1 || res.Charge.Coins != 0 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	res, err = e.call(e.req(Withings, "bloodPressure", map[string]string{"days": "7", "limit": "5"}))
	if err != nil {
		t.Fatal(err)
	}
	rows = res.Output.Rows
	if len(rows) != 2 || rows[0] != (Row{Title: "2026/11/11 20:59", Value: "121/79 mmHg"}) ||
		rows[1] != (Row{Title: "2026/11/10 21:01", Subtitle: "脈拍 65", Value: "128/82 mmHg"}) {
		t.Fatalf("bp rows %+v", rows)
	}
	if res.Output.Cells != nil {
		t.Fatalf("bp cells %v", res.Output.Cells)
	}
	if f.lastForm.Get("meastypes") != "9,10,11" {
		t.Fatalf("form %v", f.lastForm)
	}

	// not cached: the same call reaches Withings again
	before := f.calls
	if _, err := e.call(e.req(Withings, "bloodPressure", map[string]string{"days": "7", "limit": "5"})); err != nil || f.calls != before+1 {
		t.Fatalf("second call: %v calls %d → %d", err, before, f.calls)
	}

	// expired → refreshed with the rotated refresh token
	tok, _ := e.svc.Conns.Token(ctx, user, Withings)
	tok.Expiry = e.c.t.Add(-time.Minute)
	if err := e.svc.Conns.Save(ctx, user, Withings, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Withings, "weight", nil)); err != nil {
		t.Fatal(err)
	}
	tok, err = e.svc.Conns.Token(ctx, user, Withings)
	if err != nil || tok.AccessToken != "wi-access-1" || tok.RefreshToken != "wi-refresh-1" || f.refreshes != 1 {
		t.Fatalf("refreshed: %v refreshes=%d", err, f.refreshes)
	}

	// an old (rotated away) refresh token → error
	if err := e.svc.Conns.Save(ctx, user, Withings, Token{AccessToken: "wi-access-0", RefreshToken: "wi-refresh-0", Expiry: e.c.t.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Withings, "weight", nil)); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("refused refresh: %v", err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Withings); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// reconnect and cut: Withings has no plain revoke; the tokens go
	e.connectWithings(f)
	if err := e.svc.Revoke(ctx, user, Withings); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Withings); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
}

// A token Withings no longer takes (status 401 in a 200) turns the
// connection to error and gives the run back.
func TestWithingsRevokedAtProvider(t *testing.T) {
	e, f := useWithings(t)
	e.connectWithings(f)
	ctx := context.Background()
	if err := e.svc.Conns.Save(ctx, user, Withings, Token{AccessToken: "dead"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Withings, "weight", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Withings); c.Status != StatusError {
		t.Fatalf("status %q", c.Status)
	}
}

func withingsTestClient(t *testing.T) (*withings, *fakeWithings) {
	f, srv := newFakeWithings(t)
	c, err := newWithings(Secrets{ClientID: withingsTestID, ClientSecret: withingsTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestWithingsArgsAndErrors(t *testing.T) {
	c, f := withingsTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: "wi-access-0"}
	p, _ := Lookup(Withings)
	for i, a := range []map[string]string{
		{"days": "0"}, {"days": "91"}, {"days": "x"}, {"limit": "0"}, {"limit": "51"}, {"limit": "1.5"},
	} {
		if _, err := c.weight(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrUnknown) {
			t.Errorf("weight case %d: %v", i, err)
		}
		if _, err := p.CheckArgs("bloodPressure", a); !errors.Is(err, ErrUnknown) {
			t.Errorf("CheckArgs case %d: %v", i, err)
		}
	}
	if f.calls != 0 {
		t.Fatal("a bad argument reached Withings")
	}
	if _, err := p.CheckArgs("weight", map[string]string{"userid": "1"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown arg: %v", err)
	}
	if _, err := c.weight(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	var pe *ProviderError
	if _, err := c.weight(ctx, Input{Token: &Token{AccessToken: "busy"}}); !errors.As(err, &pe) || !pe.Retryable || pe.Status != 429 {
		t.Fatalf("601: %v", err)
	}
	if _, err := c.weight(ctx, Input{Token: &Token{AccessToken: "down"}}); !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
		t.Fatalf("502: %v", err)
	}
	if _, err := c.weight(ctx, Input{Token: &Token{AccessToken: "odd"}}); !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("2554: %v", err)
	}
	if _, err := c.Refresh(ctx, Token{AccessToken: "x"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("no refresh token: %v", err)
	}
	if _, err := c.Exchange(ctx, "bad", "v", "https://runner.example/oauth/callback/withings"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad code: %v", err)
	}
	if err := c.Revoke(ctx, Token{AccessToken: "wi-access-0"}); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
}

// more = 1: the next pages are read (bounded) and limit applies after sorting.
func TestWithingsPages(t *testing.T) {
	c, f := withingsTestClient(t)
	f.pageSize = 1
	out, err := c.weight(context.Background(), Input{Token: &Token{AccessToken: "wi-access-0"}, Args: map[string]string{"limit": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != withingsMaxPages || len(out.Rows) != 1 || out.Rows[0].Value != "65.5 kg" {
		t.Fatalf("calls %d rows %+v", f.calls, out.Rows)
	}
}

func TestWithingsDecimal(t *testing.T) {
	for _, tc := range []struct {
		v    int64
		u    int
		want string
	}{{65750, -3, "65.75"}, {655, -1, "65.5"}, {128, 0, "128"}, {5, -3, "0.005"}, {70000, -3, "70"}, {12, 2, "1200"}} {
		if got, ok := withingsDecimal(tc.v, tc.u); !ok || got != tc.want {
			t.Errorf("%d e%d = %q", tc.v, tc.u, got)
		}
	}
	if _, ok := withingsDecimal(-1, 0); ok {
		t.Error("negative")
	}
}

// The real endpoints are on the allowlist; nothing else is reachable;
// without the client id / secret it stays off; off by default, read only,
// never cached and never in the review build.
func TestWithingsDefinition(t *testing.T) {
	p, ok := Lookup(Withings)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "WITHINGS" {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 || len(o.Scopes) != 1 || o.Scopes[0] != withingsScopeMetrics {
			t.Errorf("%s: %+v", o.Name, o)
		}
	}
	w := withingsWireDefault
	for _, raw := range []string{w.TokenURL, w.APIBase + "/measure"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	if len(p.Hosts) != 1 {
		t.Errorf("hosts %v", p.Hosts)
	}
	c, err := newWithings(Secrets{ClientID: "a", ClientSecret: "b"}, w)
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
	if cfg.Enabled[Withings] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
