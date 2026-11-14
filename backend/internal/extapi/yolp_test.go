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

// 偽の鍵(gitleaks に掛からない値)。
var yolpTestKey = strings.Repeat("y", 32)

// yolpServer は map.yahooapis.jp の代わり(本物は呼ばない)。
type yolpServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value // 最後の要求の URI
	ua     atomic.Value // 最後の要求の User-Agent
	status int
	body   string
}

func newYOLPServer(t *testing.T) *yolpServer {
	t.Helper()
	s := &yolpServer{status: http.StatusOK}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.RequestURI())
		s.ua.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		body := s.body
		if body == "" {
			body = `{"ResultInfo":{"Count":3,"Total":3,"Start":1,"Status":200},"Feature":[
				{"Id":"1","Name":"兼六園","Geometry":{"Type":"point","Coordinates":"136.6625,36.5621"},
				 "Property":{"Address":"石川県金沢市兼六町1","Tel1":"076-000-0000","Genre":[{"Code":"0303001","Name":"庭園"}]}},
				{"Id":"2","Name":"  ","Property":{"Address":"名前なし"}},
				{"Id":"3","Name":"金沢城公園","Geometry":{"Type":"point","Coordinates":"bad"},"Property":{"Address":"石川県金沢市丸の内1-1"}},
				{"Id":"4","Name":"四つ目","Property":{}}]}`
		}
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *yolpServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *yolpServer) ops(t *testing.T) map[string]Operation {
	t.Helper()
	ops, err := newYOLP(yolpTestKey, s.URL, s.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestYOLPRegistered(t *testing.T) {
	p, ok := Lookup(YOLP)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://map.yahooapis.jp"}) {
		t.Fatalf("yolp %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.yolp") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL != 10*time.Minute {
			t.Fatalf("%s: read ops, cached 10 minutes", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_YOLP_ENABLED": "1", "EXT_YOLP_API_KEY": yolpTestKey}[k]
	})
	if err != nil || !cfg.Enabled[YOLP] || cfg.Secrets[YOLP].APIKey != yolpTestKey {
		t.Fatalf("EXT_YOLP_*: %v %v", cfg.Enabled, err)
	}
	if off, err := ConfigFromEnv(func(string) string { return "" }); err != nil || off.Enabled[YOLP] {
		t.Fatalf("on by default: %v", err)
	}
	// アプリ ID が無ければ起動で止まる
	if _, err := p.New(Secrets{}); err == nil || !strings.Contains(err.Error(), "EXT_YOLP_API_KEY") {
		t.Fatalf("no key: %v", err)
	}
}

func TestYOLPLocalSearch(t *testing.T) {
	s := newYOLPServer(t)
	out, err := s.ops(t)["localSearch"].Do(context.Background(), Input{Args: map[string]string{"query": "庭園", "lat": "36.562123", "lng": "136.662456", "dist": "5", "limit": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/search/local/V1/localSearch?dist=5&lat=36.5621&lon=136.6625&output=json&query=%E5%BA%AD%E5%9C%92&results=3&sort=dist" {
		t.Fatalf("request %s", s.uri())
	}
	// アプリ ID はヘッダーだけ(URL に入れない)
	if ua, _ := s.ua.Load().(string); ua != "Yahoo AppID: "+yolpTestKey || strings.Contains(s.uri(), yolpTestKey) {
		t.Fatalf("user agent %q / uri %s", ua, s.uri())
	}
	// 名前の無い行は飛ばす・電話番号は出さない・limit で切る
	want := []Row{
		{Title: "兼六園", Subtitle: "石川県金沢市兼六町1", Value: "庭園"},
		{Title: "金沢城公園", Subtitle: "石川県金沢市丸の内1-1"},
		{Title: "四つ目"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	if len(out.Cells) != 3 || !slices.Equal(out.Cells[0], []string{"兼六園", "石川県金沢市兼六町1", "庭園", "36.5621", "136.6625"}) ||
		!slices.Equal(out.Cells[1], []string{"金沢城公園", "石川県金沢市丸の内1-1", "", "", ""}) {
		t.Fatalf("cells %v", out.Cells)
	}
	// 点が無ければ lat / lon / dist / sort を付けない
	out, err = s.ops(t)["localSearch"].Do(context.Background(), Input{Args: map[string]string{"query": "金沢 カフェ", "limit": "2"}})
	if err != nil || len(out.Rows) != 2 {
		t.Fatalf("no point: %+v %v", out.Rows, err)
	}
	if u := s.uri(); strings.Contains(u, "lat=") || strings.Contains(u, "sort=") || !strings.Contains(u, "results=2") {
		t.Fatalf("request %s", u)
	}
}

func TestYOLPCheck(t *testing.T) {
	p, _ := Lookup(YOLP)
	for _, c := range []struct {
		args map[string]string
		ok   bool
	}{
		{map[string]string{"query": "ラーメン"}, true},
		{map[string]string{"query": "ラーメン", "lat": "36.56", "lng": "136.65", "dist": "20", "limit": "20"}, true},
		{nil, false},
		{map[string]string{"query": "  "}, false},
		{map[string]string{"query": "a\nb"}, false},
		{map[string]string{"query": strings.Repeat("あ", 101)}, false},
		{map[string]string{"query": "https://evil.example/x"}, false},
		{map[string]string{"query": "x", "lat": "36.56"}, false},
		{map[string]string{"query": "x", "lat": "91", "lng": "0"}, false},
		{map[string]string{"query": "x", "dist": "3"}, false},
		{map[string]string{"query": "x", "lat": "36.56", "lng": "136.65", "dist": "0"}, false},
		{map[string]string{"query": "x", "lat": "36.56", "lng": "136.65", "dist": "21"}, false},
		{map[string]string{"query": "x", "limit": "0"}, false},
		{map[string]string{"query": "x", "limit": "21"}, false},
		{map[string]string{"query": "x", "limit": "x"}, false},
		{map[string]string{"q": "x"}, false},
	} {
		_, err := p.CheckArgs("localSearch", c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%v: %v", c.args, err)
		}
	}
}

func TestYOLPFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusBadRequest, `{"Error":{"Message":"bad"}}`, func(err error) bool { return errors.Is(err, ErrUnknown) && Reason(err) == ReasonBadRequest }},
		{http.StatusUnauthorized, `{"Error":{"Message":"appid"}}`, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && !pe.Retryable && Reason(err) == ReasonUpstream && !strings.Contains(err.Error(), yolpTestKey)
		}},
		{http.StatusForbidden, `{"Error":{"Message":"appid"}}`, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && !pe.Retryable && Reason(err) == ReasonUpstream
		}},
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && !pe.Billable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && upstreamLimited(err)
		}},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"Error":{"Message":"x"}}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
	}
	for _, c := range cases {
		s := newYOLPServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops(t)["localSearch"].Do(context.Background(), Input{Args: map[string]string{"query": "x"}})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
	// 0 件は 0 行(失敗ではない)
	s := newYOLPServer(t)
	s.body = `{"ResultInfo":{"Count":0,"Total":0},"Feature":[]}`
	if out, err := s.ops(t)["localSearch"].Do(context.Background(), Input{Args: map[string]string{"query": "x"}}); err != nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %+v %v", out, err)
	}
}

// 本物のクライアントは map.yahooapis.jp にしか届かない。
func TestYOLPEgress(t *testing.T) {
	s := newYOLPServer(t)
	real, err := newYOLP(yolpTestKey, s.URL, NewHTTPClient(yolpHosts, time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_, err = real["localSearch"].Do(context.Background(), Input{Args: map[string]string{"query": "x"}})
	if !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
}

// Service 経由: 同じ引数は 10 分キャッシュ(`runs` は 1 回だけ)。
func TestYOLPCachedCall(t *testing.T) {
	s := newYOLPServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[YOLP] = true }})
	e.svc.ops[YOLP] = s.ops(t)
	args := map[string]string{"query": "庭園"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(YOLP, "localSearch", args))
		if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(10 * time.Minute)
	if _, err := e.call(e.req(YOLP, "localSearch", args)); err != nil || s.hits.Load() != 2 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	if _, err := e.call(e.req(YOLP, "localSearch", map[string]string{"query": "x", "dist": "3"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 2 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
}
