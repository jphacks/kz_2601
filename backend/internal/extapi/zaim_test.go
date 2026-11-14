package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/extapi/oauth1"
)

// fakeZaim is Zaim's OAuth 1.0a token endpoints and the home API on
// httptest (no live calls). Every request's signature is checked as Zaim
// would check it.
type fakeZaim struct {
	t   *testing.T
	url string

	mu        sync.Mutex
	callback  string // oauth_callback of the request token
	verifier  string // the one the access token step takes
	lastPath  string
	lastForm  url.Values
	lastQuery url.Values // of /v2/home/money
	payments  int
	noGenres  bool
}

const (
	zaimTestKey    = "zaim-consumer-key"
	zaimTestSecret = "zaim-consumer-secret"
	zaimRequestTok = "req-token"
	zaimRequestSec = "req-secret"
	zaimAccessTok  = "access-token"
	zaimAccessSec  = "access-secret"
)

// tokenSecrets are the token secrets the fake knows (an unknown token is 401).
var zaimTokenSecrets = map[string]string{zaimRequestTok: zaimRequestSec, zaimAccessTok: zaimAccessSec,
	"narrow": "s", "busy": "s", "notfound": "s"}

func newFakeZaim(t *testing.T) (*fakeZaim, *httptest.Server) {
	f := &fakeZaim{t: t, verifier: "good-verifier"}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, status int, v string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(v))
	}
	// verify checks the header's signature and returns the protocol params.
	verify := func(w http.ResponseWriter, r *http.Request) (url.Values, bool) {
		params, ok := oauth1.ParseHeader(r.Header.Get("Authorization"))
		if !ok {
			w.WriteHeader(401)
			return nil, false
		}
		var sig string
		var protocol []oauth1.Param
		p := url.Values{}
		for _, x := range params {
			p.Set(x.Key, x.Value)
			if x.Key == "oauth_signature" {
				sig = x.Value
				continue
			}
			protocol = append(protocol, x)
		}
		body := ""
		if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
			b, _ := ReadBody(r.Body)
			body = string(b)
		}
		u := *r.URL
		u.Scheme, u.Host = "http", r.Host
		all, err := oauth1.Collect(&u, body, protocol)
		if err != nil {
			w.WriteHeader(400)
			return nil, false
		}
		tok := p.Get("oauth_token")
		secret, known := zaimTokenSecrets[tok]
		if (tok != "" && !known) || p.Get("oauth_consumer_key") != zaimTestKey || p.Get("oauth_signature_method") != "HMAC-SHA1" ||
			p.Get("oauth_nonce") == "" || p.Get("oauth_timestamp") == "" ||
			sig != oauth1.SignHMACSHA1(oauth1.BaseString(r.Method, &u, all), zaimTestSecret, secret) {
			writeJSON(w, 401, `{"error":{"message":"bad signature"}}`)
			return nil, false
		}
		f.mu.Lock()
		f.lastPath = r.URL.Path
		f.lastForm, _ = url.ParseQuery(body)
		f.mu.Unlock()
		return p, true
	}
	mux.HandleFunc("POST /v2/auth/request", func(w http.ResponseWriter, r *http.Request) {
		p, ok := verify(w, r)
		if !ok {
			return
		}
		if p.Get("oauth_token") != "" || p.Get("oauth_callback") == "" {
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		f.callback = p.Get("oauth_callback")
		f.mu.Unlock()
		_, _ = w.Write([]byte(url.Values{"oauth_token": {zaimRequestTok}, "oauth_token_secret": {zaimRequestSec}, "oauth_callback_confirmed": {"true"}}.Encode()))
	})
	mux.HandleFunc("POST /v2/auth/access", func(w http.ResponseWriter, r *http.Request) {
		p, ok := verify(w, r)
		if !ok {
			return
		}
		f.mu.Lock()
		want := f.verifier
		f.mu.Unlock()
		if p.Get("oauth_token") != zaimRequestTok || p.Get("oauth_verifier") != want {
			writeJSON(w, 401, `{"error":{"message":"bad verifier"}}`)
			return
		}
		_, _ = w.Write([]byte(url.Values{"oauth_token": {zaimAccessTok}, "oauth_token_secret": {zaimAccessSec}}.Encode()))
	})
	// api checks the token's state before the signature (the error cases).
	api := func(h func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			p, ok := verify(w, r)
			if !ok {
				return
			}
			switch p.Get("oauth_token") {
			case zaimAccessTok:
				h(w, r)
			case "narrow":
				writeJSON(w, 403, `{"error":{"message":"forbidden"}}`)
			case "busy":
				writeJSON(w, 503, `{"error":{"message":"maintenance"}}`)
			case "notfound":
				writeJSON(w, 404, `{"error":{"message":"not found"}}`)
			default:
				writeJSON(w, 401, `{"error":{"message":"unauthorized"}}`)
			}
		}
	}
	mux.HandleFunc("GET /v2/home/money", api(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mapping") != "1" {
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		f.lastQuery = r.URL.Query()
		f.mu.Unlock()
		writeJSON(w, 200, `{"money":[
			{"id":1,"mode":"payment","date":"2026-11-10","category_id":101,"genre_id":10101,"amount":480,"name":"コーヒー","place":"カフェ","comment":"","currency_code":"JPY"},
			{"id":3,"mode":"income","date":"2026-11-12","category_id":11,"genre_id":1101,"amount":250000,"name":"","place":"","comment":"11月","currency_code":"JPY"},
			{"id":2,"mode":"payment","date":"2026-11-12","category_id":101,"genre_id":10102,"amount":1200,"name":"","place":"スーパー","comment":"","currency_code":"JPY"}
		],"requested":1700000000}`)
	}))
	mux.HandleFunc("GET /v2/home/category", api(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"categories":[{"id":101,"mode":"payment","name":"食費","sort":1,"active":1},{"id":102,"mode":"payment","name":"交通","sort":2,"active":1},
			{"id":11,"mode":"income","name":"給与所得","sort":1,"active":1},{"id":103,"mode":"payment","name":"古い","sort":3,"active":-1}]}`)
	}))
	mux.HandleFunc("GET /v2/home/genre", api(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		none := f.noGenres
		f.mu.Unlock()
		if none {
			writeJSON(w, 200, `{"genres":[]}`)
			return
		}
		writeJSON(w, 200, `{"genres":[{"id":10102,"category_id":101,"name":"食料品","sort":2,"active":1},{"id":10101,"category_id":101,"name":"カフェ","sort":1,"active":1},
			{"id":10201,"category_id":102,"name":"電車","sort":1,"active":1},{"id":1101,"category_id":11,"name":"給与","sort":1,"active":1},{"id":10301,"category_id":103,"name":"古い","sort":1,"active":1}]}`)
	}))
	mux.HandleFunc("POST /v2/home/money/payment", api(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		form := f.lastForm
		f.payments++
		f.mu.Unlock()
		if form.Get("mapping") != "1" || form.Get("category_id") == "" || form.Get("genre_id") == "" || form.Get("amount") == "" || form.Get("date") == "" {
			writeJSON(w, 400, `{"error":{"message":"missing"}}`)
			return
		}
		writeJSON(w, 200, `{"stamp":1,"money":{"id":777,"modified":"2026-11-11 12:00:00"},"requested":1700000000}`)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f, srv
}

func (f *fakeZaim) wire(srv *httptest.Server) zaimWire {
	return zaimWire{RequestURL: srv.URL + "/v2/auth/request", AuthURL: srv.URL + "/users/auth", AccessURL: srv.URL + "/v2/auth/access",
		APIBase: srv.URL + "/v2", Client: srv.Client(), Now: func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, gcalJST) }}
}

func useZaim(t *testing.T) (*env, *fakeZaim) {
	f, srv := newFakeZaim(t)
	old := zaimProdWire
	zaimProdWire = f.wire(srv)
	t.Cleanup(func() { zaimProdWire = old })
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Zaim] = true
		c.Secrets = map[string]Secrets{Zaim: {ClientID: zaimTestKey, ClientSecret: zaimTestSecret}}
	}})
	return e, f
}

// connectZaim: Start gets the request token (the state rides on the
// callback URL), the browser opens auth.zaim.net, the callback brings
// oauth_token + oauth_verifier through the runner's route.
func (e *env) connectZaim(f *fakeZaim) {
	e.t.Helper()
	ctx := context.Background()
	auth, err := e.svc.Start(ctx, user, Zaim, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	if auth != f.url+"/users/auth?oauth_token="+zaimRequestTok {
		e.t.Fatalf("consent url %s", auth)
	}
	cb, err := url.Parse(f.callback)
	if err != nil || !strings.HasPrefix(f.callback, "https://runner.example/oauth/callback/zaim?state=") || cb.Query().Get("state") == "" {
		e.t.Fatalf("oauth_callback %q", f.callback)
	}
	mux := http.NewServeMux()
	e.svc.Routes(mux)
	rec := httptest.NewRecorder()
	q := url.Values{"state": {cb.Query().Get("state")}, "oauth_token": {zaimRequestTok}, "oauth_verifier": {"good-verifier"}}
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/callback/zaim?"+q.Encode(), nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "tailorit://connected?provider=zaim&status=ok" {
		e.t.Fatalf("callback → %d %s %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

// connect (OAuth 1.0a) → money → addPayment → a 401 needs a reconnect →
// a used state fails → disconnect deletes the token (no revoke endpoint).
func TestZaimFlow(t *testing.T) {
	e, f := useZaim(t)
	ctx := context.Background()
	if _, err := e.call(e.req(Zaim, "money", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 0 {
		t.Fatalf("before connecting: %v", err)
	}
	e.connectZaim(f)
	c, _, _ := e.mem.Get(ctx, user, Zaim)
	if c.Status != StatusActive || len(c.Scopes) != 0 {
		t.Fatalf("connection %+v", c)
	}
	tok, err := e.svc.Conns.Token(ctx, user, Zaim)
	if err != nil || tok.AccessToken != zaimAccessTok || tok.RefreshToken != zaimAccessSec || !tok.Expiry.IsZero() {
		t.Fatalf("token %v", err)
	}

	res, err := e.call(e.req(Zaim, "money", map[string]string{"limit": "50", "startDate": "2026-11-01", "endDate": "2026-11-30"}))
	if err != nil {
		t.Fatal(err)
	}
	rows := res.Output.Rows
	// newest first; an item without a name shows its category/genre; income says so
	if len(rows) != 3 || rows[0].Title != "給与所得/給与" || rows[0].Subtitle != "2026-11-12・収入" || rows[0].Value != "250,000 円" ||
		rows[1].Title != "食費/食料品" || rows[1].Subtitle != "2026-11-12・スーパー" || rows[1].Value != "1,200 円" ||
		rows[2].Title != "コーヒー" || rows[2].Subtitle != "2026-11-10・カフェ" || rows[2].Value != "480 円" || rows[2].URL != "" ||
		len(res.Output.Cells) != 3 || res.Output.Cells[1][3] != "食料品" || res.Charge.Coins != 0 {
		t.Fatalf("rows %+v cells %v", rows, res.Output.Cells)
	}
	if q := f.lastQuery; q.Get("limit") != "50" || q.Get("start_date") != "2026-11-01" || q.Get("end_date") != "2026-11-30" || q.Get("mapping") != "1" {
		t.Fatalf("query %v", q)
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d (free: one `runs` a call)", e.runs())
	}

	out, err := e.call(e.req(Zaim, "addPayment", map[string]string{"amount": "1500", "genreId": "10201", "place": "駅"}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Output.ResultID != "777" || out.Output.Item == nil || out.Output.Item.Title != "交通/電車" || out.Output.Item.Subtitle != "2026-11-11・駅" ||
		out.Output.Item.Value != "1,500 円" || f.lastForm.Get("category_id") != "102" || f.lastForm.Get("date") != "2026-11-11" || f.payments != 1 {
		t.Fatalf("addPayment %+v form %v", out.Output, f.lastForm)
	}

	// the token no longer taken (removed at Zaim) → error: the person reconnects
	if err := e.svc.Conns.Save(ctx, user, Zaim, Token{AccessToken: "dead", RefreshToken: "s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.call(e.req(Zaim, "money", nil)); !errors.Is(err, ErrNotConnected) || e.runs() != 2 {
		t.Fatalf("401: %v runs=%d", err, e.runs())
	}
	if c, _, _ := e.mem.Get(ctx, user, Zaim); c.Status != StatusError {
		t.Fatalf("status %q, want error", c.Status)
	}

	// a wrong verifier, then reconnect and cut: no revoke endpoint, the token is deleted
	auth, err := e.svc.Start(ctx, user, Zaim, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = auth
	cb, _ := url.Parse(f.callback)
	if back := e.svc.Callback(ctx, Zaim, cb.Query().Get("state"), "bad-verifier"); back != "tailorit://connected?provider=zaim&status=error" {
		t.Fatalf("bad verifier → %s", back)
	}
	if back := e.svc.Callback(ctx, Zaim, cb.Query().Get("state"), "good-verifier"); back != "tailorit://connected?provider=zaim&status=error" {
		t.Fatalf("used state → %s", back)
	}
	e.connectZaim(f)
	if err := e.svc.Revoke(ctx, user, Zaim); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := e.mem.Get(ctx, user, Zaim); c.Status != StatusRevoked || len(c.Sealed) != 0 {
		t.Fatalf("after revoke %+v", c)
	}
}

func zaimTestClient(t *testing.T) (*zaim, *fakeZaim) {
	f, srv := newFakeZaim(t)
	z, err := newZaim(Secrets{ClientID: zaimTestKey, ClientSecret: zaimTestSecret}, f.wire(srv))
	if err != nil {
		t.Fatal(err)
	}
	return z, f
}

func TestZaimAddPayment(t *testing.T) {
	z, f := zaimTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: zaimAccessTok, RefreshToken: zaimAccessSec}

	// a category alone takes its first genre (sort order, not list order)
	out, err := z.addPayment(ctx, Input{Token: tok, RequestID: "r1", Args: map[string]string{"amount": "480", "categoryId": "101", "date": "2026-11-01", "comment": "朝"}})
	if err != nil || out.ResultID != "777" || out.Item.Title != "食費/カフェ" || f.lastForm.Get("genre_id") != "10101" || f.lastForm.Get("comment") != "朝" || f.lastForm.Get("date") != "2026-11-01" {
		t.Fatalf("category alone: %+v %v form %v", out, err, f.lastForm)
	}
	// ids the person does not have, or that do not go together: 400 after the lookup
	bad := []map[string]string{
		{"amount": "1", "genreId": "99999"},
		{"amount": "1", "categoryId": "99999"},
		{"amount": "1", "categoryId": "11"},                      // an income category
		{"amount": "1", "categoryId": "103"},                     // inactive
		{"amount": "1", "genreId": "10301"},                      // its category is inactive / a non-payment one
		{"amount": "1", "categoryId": "102", "genreId": "10101"}, // genre of another category
	}
	for i, a := range bad {
		_, err := z.addPayment(ctx, Input{Token: tok, RequestID: "x", Args: a})
		if !errors.Is(err, ErrUnknown) || errors.Is(err, ErrBadArgs) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	f.mu.Lock()
	f.noGenres = true
	f.mu.Unlock()
	if _, err := z.addPayment(ctx, Input{Token: tok, RequestID: "x", Args: map[string]string{"amount": "1", "categoryId": "101"}}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("no genre: %v", err)
	}
	if f.payments != 1 {
		t.Fatalf("%d payments were sent", f.payments)
	}
}

func TestZaimArgs(t *testing.T) {
	z, f := zaimTestClient(t)
	ctx := context.Background()
	tok := &Token{AccessToken: zaimAccessTok, RefreshToken: zaimAccessSec}
	bad := []map[string]string{
		{"categoryId": "101"},                                   // no amount
		{"amount": "0", "categoryId": "101"},                    // not positive
		{"amount": "12.5", "categoryId": "101"},                 // not whole yen
		{"amount": "100000001", "categoryId": "101"},            // too big
		{"amount": "100"},                                       // neither id
		{"amount": "100", "genreId": "abc"},                     // not an id
		{"amount": "100", "categoryId": "0101"},                 // leading zero
		{"amount": "100", "categoryId": "101", "date": "11/01"}, // not a date
		{"amount": "100", "categoryId": "101", "place": strings.Repeat("あ", 101)},
		{"amount": "100", "categoryId": "101", "comment": "a\x00b"},
	}
	for i, a := range bad {
		if _, err := z.addPayment(ctx, Input{Token: tok, RequestID: "x", Args: a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("addPayment case %d: %v", i, err)
		}
		if err := checkZaimAddPayment(a); err == nil {
			t.Errorf("Check passed case %d", i)
		}
	}
	badList := []map[string]string{
		{"limit": "0"}, {"limit": "101"}, {"limit": "x"},
		{"startDate": "2026/11/01"},
		{"startDate": "2026-11-30", "endDate": "2026-11-01"},
		{"startDate": "2025-01-01", "endDate": "2026-06-01"},
		{"mode": "expense"},
	}
	for i, a := range badList {
		if _, err := z.money(ctx, Input{Token: tok, Args: a}); !errors.Is(err, ErrBadArgs) {
			t.Errorf("money case %d: %v", i, err)
		}
		if err := checkZaimMoney(a); err == nil {
			t.Errorf("Check passed list case %d", i)
		}
	}
	if f.lastPath != "" || f.payments != 0 {
		t.Fatal("a bad argument reached Zaim")
	}
	// defaults: 20, every mode, date order
	if _, err := z.money(ctx, Input{Token: tok}); err != nil {
		t.Fatal(err)
	}
	if q := f.lastQuery; q.Get("limit") != "20" || q.Get("order") != "date" || q.Get("mapping") != "1" || q.Has("mode") || q.Has("start_date") {
		t.Fatalf("default query %v", q)
	}
	if got := zaimAmount(-1234567, "JPY"); got != "-1,234,567 円" {
		t.Fatalf("amount %q", got)
	}
	if got := zaimAmount(12, "USD"); got != "12 USD" {
		t.Fatalf("amount %q", got)
	}
}

func TestZaimErrors(t *testing.T) {
	z, _ := zaimTestClient(t)
	ctx := context.Background()
	if _, err := z.money(ctx, Input{Token: &Token{AccessToken: "narrow", RefreshToken: "s"}}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("403: %v", err)
	}
	_, err := z.money(ctx, Input{Token: &Token{AccessToken: "busy", RefreshToken: "s"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable || pe.Billable || Reason(err) != ReasonUpstream {
		t.Fatalf("503: %v", err)
	}
	if _, err := z.money(ctx, Input{Token: &Token{AccessToken: "notfound", RefreshToken: "s"}}); !errors.Is(err, ErrUnknown) || errors.Is(err, ErrBadArgs) {
		t.Fatalf("404: %v", err)
	}
	if _, err := z.money(ctx, Input{Token: &Token{AccessToken: "dead", RefreshToken: "s"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("401: %v", err)
	}
	// a wrong token secret signs wrongly: Zaim's 401
	if _, err := z.money(ctx, Input{Token: &Token{AccessToken: zaimAccessTok, RefreshToken: "wrong"}}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad secret: %v", err)
	}
	if _, err := z.money(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("no token: %v", err)
	}
	if err := z.Revoke(ctx, Token{AccessToken: "a"}); !errors.Is(err, ErrNoRevoke) {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := z.Refresh(ctx, Token{AccessToken: "a"}); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("refresh: %v", err)
	}
	if _, err := z.Exchange1(ctx, zaimRequestTok, zaimRequestSec, "bad"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("bad verifier: %v", err)
	}
	if _, err := z.Exchange1(ctx, "", "", ""); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := z.Exchange(ctx, "c", "v", "r"); err == nil {
		t.Fatal("Exchange is not OAuth 1.0a")
	}
}

// The real endpoints are on the allowlist (auth.zaim.net is the browser's
// only); nothing else is reachable; without the consumer key / secret the
// provider stays off; no scopes; off by default and never in the review
// build.
func TestZaimDefinition(t *testing.T) {
	p, ok := Lookup(Zaim)
	if !ok || !p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "ZAIM" || len(p.Hosts) != 1 {
		t.Fatalf("%+v", p)
	}
	w := zaimProdWire
	for _, raw := range []string{w.RequestURL, w.AccessURL, w.APIBase + "/home/money", w.APIBase + "/home/money/payment"} {
		u, _ := url.Parse(raw)
		if !AllowedURL(p.Hosts, u) {
			t.Errorf("%s not on the allowlist", raw)
		}
	}
	if u, _ := url.Parse(w.AuthURL); AllowedURL(p.Hosts, u) {
		t.Error("the consent page is reachable from the runner")
	}
	for _, o := range p.Ops {
		if len(o.Scopes) != 0 || o.CacheTTL != 0 {
			t.Errorf("%s: scopes %v cache %v", o.Name, o.Scopes, o.CacheTTL)
		}
	}
	if o, _ := p.Op("addPayment"); !o.Write {
		t.Error("addPayment is not a write")
	}
	z, err := newZaim(Secrets{ClientID: "a", ClientSecret: "b"}, w)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := any(z).(OAuth1Client); !ok {
		t.Fatal("not an OAuth1Client")
	}
	if _, err = z.hc.Get("https://evil.example.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
	if _, err := p.New(Secrets{ClientID: "a"}); err == nil {
		t.Fatal("built without a secret")
	}
	c, _ := ConfigFromEnv(func(string) string { return "" })
	if c.Enabled[Zaim] {
		t.Fatal("on by default")
	}
	for _, f := range ReviewBuildFeatures() {
		if f == p.FeatureKey() {
			t.Fatal("in the review build")
		}
	}
}
