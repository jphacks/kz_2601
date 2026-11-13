package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// cgServer is a stand-in for api.coingecko.com (no live calls).
type cgServer struct {
	*httptest.Server
	uri, key string
	status   int
	body     []byte
}

func newCGServer(t *testing.T, body []byte) *cgServer {
	t.Helper()
	s := &cgServer{status: http.StatusOK, body: body}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.uri, s.key = r.URL.RequestURI(), r.Header.Get("x-cg-demo-api-key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write(s.body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *cgServer) ops(t *testing.T, key string) map[string]Operation {
	t.Helper()
	ops, err := newCoinGecko(key, s.URL, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestCoinGeckoRegistered_2139(t *testing.T) {
	p, ok := Lookup(CoinGecko)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://api.coingecko.com"}) {
		t.Fatalf("coingecko %+v", p)
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL <= 0 {
			t.Fatalf("%s: read ops, cached", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || cfg.Enabled[CoinGecko] {
		t.Fatalf("on by default: %v %v", cfg.Enabled[CoinGecko], err)
	}
	if New(cfg, nil, nil, nil, nil, nil).Enabled(CoinGecko) {
		t.Fatal("callable by default")
	}
	cfg, err = ConfigFromEnv(func(k string) string { return map[string]string{"EXT_COINGECKO_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[CoinGecko] || !New(cfg, nil, nil, nil, nil, nil).Enabled(CoinGecko) {
		t.Fatalf("EXT_COINGECKO_ENABLED: %v %v", cfg.Enabled[CoinGecko], err)
	}
}

func TestCoinGeckoCheckArgs_2139(t *testing.T) {
	p, _ := Lookup(CoinGecko)
	good := []struct {
		op   string
		args map[string]string
	}{
		{"price", map[string]string{"ids": "bitcoin"}},
		{"price", map[string]string{"ids": "bitcoin,ethereum,shiba-inu", "vs": "usd"}},
		{"history", map[string]string{"id": "bitcoin"}},
		{"history", map[string]string{"id": "bitcoin", "vs": "jpy", "days": "90"}},
	}
	for _, c := range good {
		if _, err := p.CheckArgs(c.op, c.args); err != nil {
			t.Errorf("%s %v: %v", c.op, c.args, err)
		}
	}
	bad := []struct {
		op   string
		args map[string]string
	}{
		{"price", map[string]string{}},
		{"price", map[string]string{"ids": "Bitcoin"}},
		{"price", map[string]string{"ids": "bitcoin,bitcoin"}},
		{"price", map[string]string{"ids": "a,b,c,d,e,f,g,h,i,j,k"}},
		{"price", map[string]string{"ids": "bitcoin/../x"}},
		{"price", map[string]string{"ids": "bitcoin", "vs": "JPY"}},
		{"price", map[string]string{"ids": "bitcoin", "vs": "jpy,usd"}},
		{"history", map[string]string{}},
		{"history", map[string]string{"id": "-bitcoin"}},
		{"history", map[string]string{"id": "bitcoin", "days": "91"}},
		{"history", map[string]string{"id": "bitcoin", "days": "0"}},
		{"history", map[string]string{"id": "bitcoin", "other": "1"}},
	}
	for _, c := range bad {
		if _, err := p.CheckArgs(c.op, c.args); !errors.Is(err, ErrUnknown) {
			t.Errorf("%s %v: err %v, want bad_request", c.op, c.args, err)
		}
	}
}

func TestCoinGeckoPrice_2139(t *testing.T) {
	s := newCGServer(t, readTestdata(t, "coingecko", "price.json"))
	out, err := s.ops(t, "")["price"].Do(context.Background(), Input{Args: map[string]string{"ids": "ethereum,bitcoin,nocoin,shiba-inu"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri != "/api/v3/simple/price?ids=ethereum%2Cbitcoin%2Cnocoin%2Cshiba-inu&include_24hr_change=true&vs_currencies=jpy" || s.key != "" {
		t.Fatalf("request %s key %q", s.uri, s.key)
	}
	want := []Row{
		{Title: "ethereum", Subtitle: "24時間 -2.50%", Value: "512345.5 JPY"},
		{Title: "bitcoin", Subtitle: "24時間 +1.23%", Value: "15234567.89 JPY"},
		{Title: "shiba-inu", Value: "0.00123457 JPY"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
}

func TestCoinGeckoDemoKey_2139(t *testing.T) {
	s := newCGServer(t, readTestdata(t, "coingecko", "price.json"))
	if _, err := s.ops(t, "CG-demo123456")["price"].Do(context.Background(), Input{Args: map[string]string{"ids": "bitcoin"}}); err != nil {
		t.Fatal(err)
	}
	if s.key != "CG-demo123456" {
		t.Fatalf("key header %q", s.key)
	}
	if _, err := newCoinGecko("bad key!", s.URL, s.Client()); err == nil {
		t.Fatal("a malformed key was taken")
	}
}

func TestCoinGeckoPriceUnknown_2139(t *testing.T) {
	s := newCGServer(t, readTestdata(t, "coingecko", "price_empty.json"))
	_, err := s.ops(t, "")["price"].Do(context.Background(), Input{Args: map[string]string{"ids": "nocoin"}})
	if !errors.Is(err, ErrUnknown) {
		t.Fatalf("err %v, want bad_request", err)
	}
}

func TestCoinGeckoHistory_2139(t *testing.T) {
	s := newCGServer(t, readTestdata(t, "coingecko", "history.json"))
	out, err := s.ops(t, "")["history"].Do(context.Background(), Input{Args: map[string]string{"id": "bitcoin", "days": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri != "/api/v3/coins/bitcoin/market_chart?days=3&interval=daily&vs_currency=jpy" {
		t.Fatalf("request %s", s.uri)
	}
	want := []Row{ // newest first, one a JST day (the 11/14 "now" point wins)
		{Title: "2025/11/14", Value: "15234567.89 JPY"},
		{Title: "2025/11/13", Value: "15100000 JPY"},
		{Title: "2025/11/12", Value: "15000000.5 JPY"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// days=1 → at most 2 rows
	out, _ = s.ops(t, "")["history"].Do(context.Background(), Input{Args: map[string]string{"id": "bitcoin", "days": "1"}})
	if len(out.Rows) != 2 {
		t.Fatalf("days=1 rows %+v", out.Rows)
	}
}

func TestCoinGeckoErrors_2139(t *testing.T) {
	s := newCGServer(t, []byte(`{"error":"coin not found"}`))
	s.status = http.StatusNotFound
	if _, err := s.ops(t, "")["history"].Do(context.Background(), Input{Args: map[string]string{"id": "nocoin"}}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("404: %v, want bad_request", err)
	}
	s.status, s.body = http.StatusTooManyRequests, []byte(`{"status":{"error_code":429}}`)
	var pe *ProviderError
	if _, err := s.ops(t, "")["price"].Do(context.Background(), Input{Args: map[string]string{"ids": "bitcoin"}}); !errors.As(err, &pe) || !pe.Retryable || pe.Status != http.StatusTooManyRequests {
		t.Fatalf("429: %v", err)
	}
	s.status, s.body = http.StatusOK, []byte(`{"prices":[]}`)
	if _, err := s.ops(t, "")["history"].Do(context.Background(), Input{Args: map[string]string{"id": "bitcoin"}}); !errors.As(err, &pe) {
		t.Fatalf("empty history: %v", err)
	}
}
