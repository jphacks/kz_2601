package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fxServer is a stand-in for api.frankfurter.app (no live calls).
type fxServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value // the last request URI
	status int
	body   string
}

func newFXServer(t *testing.T) *fxServer {
	t.Helper()
	s := &fxServer{status: http.StatusOK,
		body: `{"amount":10.0,"base":"USD","date":"2026-11-10","rates":{"EUR":9.2051,"JPY":1532.1,"GBP":7.71}}`}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *fxServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *fxServer) ops() map[string]Operation { return newFrankfurter(s.URL, s.Client()) }

func TestFrankfurterRegistered(t *testing.T) {
	p, ok := Lookup(Frankfurter)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://api.frankfurter.app"}) {
		t.Fatalf("frankfurter %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.frankfurter") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL <= 0 {
			t.Fatalf("%s: read ops, cached", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string { return map[string]string{"EXT_FRANKFURTER_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[Frankfurter] {
		t.Fatalf("EXT_FRANKFURTER_ENABLED: %v %v", cfg.Enabled, err)
	}
}

func TestFrankfurterLatest(t *testing.T) {
	s := newFXServer(t)
	out, err := s.ops()["latest"].Do(context.Background(), Input{Args: map[string]string{"from": "USD", "to": "JPY,EUR", "amount": "10"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/latest?amount=10&from=USD&to=JPY%2CEUR" {
		t.Fatalf("request %s", s.uri())
	}
	want := []Row{{Title: "JPY", Subtitle: "2026-11-10・10 USD", Value: "1532.1"}, {Title: "EUR", Subtitle: "2026-11-10・10 USD", Value: "9.2051"}}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// no to: every currency, by code
	out, err = s.ops()["latest"].Do(context.Background(), Input{})
	if err != nil || s.uri() != "/latest" || len(out.Rows) != 3 || out.Rows[0].Title != "EUR" || out.Rows[1].Title != "GBP" {
		t.Fatalf("all: %s %+v %v", s.uri(), out.Rows, err)
	}
}

func TestFrankfurterHistorical(t *testing.T) {
	s := newFXServer(t)
	out, err := s.ops()["historical"].Do(context.Background(), Input{Args: map[string]string{"date": "2020-01-02", "from": "USD", "to": "JPY"}})
	if err != nil || s.uri() != "/2020-01-02?from=USD&to=JPY" || len(out.Rows) != 1 {
		t.Fatalf("historical: %s %+v %v", s.uri(), out.Rows, err)
	}
}

func TestFrankfurterCheck(t *testing.T) {
	p, _ := Lookup(Frankfurter)
	future := time.Now().UTC().Add(72 * time.Hour).Format(time.DateOnly)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"latest", nil, true},
		{"latest", map[string]string{"from": "USD", "to": "JPY,EUR", "amount": "12.5"}, true},
		{"latest", map[string]string{"from": "usd"}, false},
		{"latest", map[string]string{"from": "USDX"}, false},
		{"latest", map[string]string{"to": "JPY,,EUR"}, false},
		{"latest", map[string]string{"to": "JPY,JPY"}, false},
		{"latest", map[string]string{"to": "AAA,BBB,CCC,DDD,EEE,FFF,GGG,HHH,III,JJJ,KKK"}, false},
		{"latest", map[string]string{"amount": "0"}, false},
		{"latest", map[string]string{"amount": "-1"}, false},
		{"latest", map[string]string{"amount": "1e9"}, false},
		{"latest", map[string]string{"date": "2020-01-02"}, false},
		{"historical", map[string]string{"date": "2020-01-02", "to": "JPY"}, true},
		{"historical", nil, false},
		{"historical", map[string]string{"date": "2020-1-2"}, false},
		{"historical", map[string]string{"date": "2020-02-30"}, false},
		{"historical", map[string]string{"date": "1998-12-31"}, false},
		{"historical", map[string]string{"date": future}, false},
		{"historical", map[string]string{"date": "2020-01-02", "from": "jpy"}, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%s %v: %v", c.op, c.args, err)
		}
	}
}

func TestFrankfurterFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusNotFound, `{"message":"not found"}`, func(err error) bool { return errors.Is(err, ErrUnknown) }},
		{http.StatusUnprocessableEntity, `{}`, func(err error) bool { return Reason(err) == ReasonBadRequest }},
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && !pe.Billable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && pe.Retryable }},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"amount":1,"base":"<b>","date":"2026-11-10","rates":{}}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"amount":1,"base":"USD","date":"2026-11-10","rates":{"JPY":1},"pad":"` + strings.Repeat("x", frankfurterMaxBody) + `"}`,
			func(err error) bool { return Reason(err) == ReasonUpstream }},
	}
	for _, c := range cases {
		s := newFXServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops()["latest"].Do(context.Background(), Input{})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
	// odd entries are dropped, not shown
	s := newFXServer(t)
	s.body = `{"amount":1,"base":"USD","date":"2026-11-10","rates":{"JPY":151.2,"jpy":1,"XX":2,"EUR":null}}`
	out, err := s.ops()["latest"].Do(context.Background(), Input{})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "JPY" {
		t.Fatalf("odd entries: %+v %v", out.Rows, err)
	}
}

// The real client reaches api.frankfurter.app only: another host, and the
// local test server, are refused before a connection is made.
func TestFrankfurterEgress(t *testing.T) {
	s := newFXServer(t)
	real := newFrankfurter(s.URL, NewHTTPClient(frankfurterHosts, time.Second))
	_, err := real["latest"].Do(context.Background(), Input{})
	if !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
	other := newFrankfurter("https://example.com", NewHTTPClient(frankfurterHosts, time.Second))
	if _, err := other["latest"].Do(context.Background(), Input{}); !errors.Is(err, ErrEgress) {
		t.Fatalf("another host: %v", err)
	}
}

// Through the Service: one `runs` for the first call, none for the same
// arguments again (cache), one more for other arguments; a creator without
// Pro gets no cached answer either.
func TestFrankfurterCachedCall(t *testing.T) {
	s := newFXServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[Frankfurter] = true }})
	if !e.svc.Enabled(Frankfurter) {
		t.Fatal("frankfurter not on")
	}
	e.svc.ops[Frankfurter] = s.ops()
	args := map[string]string{"from": "USD", "to": "JPY"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Frankfurter, "latest", args))
		if err != nil || len(res.Output.Rows) != 1 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	if !strings.Contains(e.logs.String(), `"billing":"cache"`) {
		t.Fatal("no cache log line")
	}
	if _, err := e.call(e.req(Frankfurter, "latest", map[string]string{"from": "USD", "to": "EUR"})); err != nil || s.hits.Load() != 2 || e.runs() != 2 {
		t.Fatalf("other args: %v hits=%d runs=%d", err, s.hits.Load(), e.runs())
	}
	// the hour passes: asked again
	e.c.t = e.c.t.Add(time.Hour)
	if _, err := e.call(e.req(Frankfurter, "latest", args)); err != nil || s.hits.Load() != 3 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	// bad arguments count nothing and never reach the provider
	if _, err := e.call(e.req(Frankfurter, "latest", map[string]string{"to": "jpy"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 3 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
	// Pro only, even from the cache
	r := e.req(Frankfurter, "latest", args)
	r.CreatorUID = "someone-without-pro"
	if _, err := e.call(r); !errors.Is(err, ErrProRequired) {
		t.Fatalf("cache without Pro: %v", err)
	}
}
