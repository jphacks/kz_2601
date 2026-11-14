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

// zipServer は zipcloud.ibsnet.co.jp の代わり(本物は呼ばない)。
type zipServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value
	status int
	body   string
}

const zipBody = `{"message":null,"results":[{"address1":"東京都","address2":"千代田区","address3":"千代田","kana1":"ﾄｳｷｮｳﾄ","kana2":"ﾁﾖﾀﾞｸ","kana3":"ﾁﾖﾀﾞ","prefcode":"13","zipcode":"1000001"}],"status":200}`

func newZipServer(t *testing.T) *zipServer {
	t.Helper()
	s := &zipServer{status: http.StatusOK, body: zipBody}
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

func (s *zipServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *zipServer) ops() map[string]Operation { return newZipcloud(s.URL, s.Client()) }

func TestZipcloudRegistered(t *testing.T) {
	p, ok := Lookup(Zipcloud)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://zipcloud.ibsnet.co.jp"}) {
		t.Fatalf("zipcloud %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.zipcloud") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	if len(p.Ops) != 1 || p.Ops[0].Name != "address" || p.Ops[0].Write || p.Ops[0].CacheTTL != 24*time.Hour {
		t.Fatalf("ops %+v", p.Ops)
	}
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || cfg.Enabled[Zipcloud] {
		t.Fatalf("default: %v %v", cfg.Enabled, err)
	}
	cfg, err = ConfigFromEnv(func(k string) string { return map[string]string{"EXT_ZIPCLOUD_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[Zipcloud] {
		t.Fatalf("EXT_ZIPCLOUD_ENABLED: %v %v", cfg.Enabled, err)
	}
}

func TestZipcloudAddress(t *testing.T) {
	s := newZipServer(t)
	out, err := s.ops()["address"].Do(context.Background(), Input{Args: map[string]string{"zipcode": "100-0001"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/api/search?limit=20&zipcode=1000001" {
		t.Fatalf("request %s", s.uri())
	}
	want := Row{Title: "東京都千代田区千代田", Subtitle: "ﾄｳｷｮｳﾄ ﾁﾖﾀﾞｸ ﾁﾖﾀﾞ", Value: "〒100-0001"}
	if len(out.Rows) != 1 || out.Rows[0] != want || out.Item == nil || *out.Item != want {
		t.Fatalf("rows %+v item %+v", out.Rows, out.Item)
	}
	// 町域が複数: 行が複数・Item は最初。番号の無い行は引数の番号で補う
	s.body = `{"status":200,"results":[{"address1":"A","address2":"B","address3":"C","zipcode":"9999999"},{"address1":"A","address2":"B","address3":"D","zipcode":""},{"address1":"","address2":"","address3":""}]}`
	out, err = s.ops()["address"].Do(context.Background(), Input{Args: map[string]string{"zipcode": "9999999"}})
	if err != nil || len(out.Rows) != 2 || out.Item.Title != "ABC" || out.Rows[1].Value != "〒999-9999" || out.Rows[0].Subtitle != "" {
		t.Fatalf("several: %+v %v", out.Rows, err)
	}
}

func TestZipcloudCheck(t *testing.T) {
	p, _ := Lookup(Zipcloud)
	for _, c := range []struct {
		args map[string]string
		ok   bool
	}{
		{map[string]string{"zipcode": "1000001"}, true},
		{map[string]string{"zipcode": "100-0001"}, true},
		{map[string]string{"zipcode": " 1000001 "}, true},
		{nil, false},
		{map[string]string{"zipcode": ""}, false},
		{map[string]string{"zipcode": "100000"}, false},
		{map[string]string{"zipcode": "10000011"}, false},
		{map[string]string{"zipcode": "１００００１"}, false},
		{map[string]string{"zipcode": "1000-001"}, false},
		{map[string]string{"zipcode": "abc1234"}, false},
		{map[string]string{"zipcode": "1000001", "limit": "1"}, false},
	} {
		_, err := p.CheckArgs("address", c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%v: %v", c.args, err)
		}
	}
}

func TestZipcloudFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		// 無い番号: results が null
		{http.StatusOK, `{"message":null,"results":null,"status":200}`, func(err error) bool { return errors.Is(err, ErrUnknown) && Reason(err) == ReasonBadRequest }},
		{http.StatusOK, `{"message":"必須パラメータが指定されていません。","results":null,"status":400}`, func(err error) bool { return Reason(err) == ReasonBadRequest }},
		{http.StatusOK, `{"message":"内部エラー","results":null,"status":500}`, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && Reason(err) == ReasonUpstream
		}},
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && !pe.Billable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && pe.Retryable }},
		{http.StatusForbidden, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && !pe.Retryable }},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"status":200,"results":[],"pad":"` + strings.Repeat("x", zipcloudMaxBody) + `"}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
	}
	for _, c := range cases {
		s := newZipServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops()["address"].Do(context.Background(), Input{Args: map[string]string{"zipcode": "1000001"}})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
}

// 本物のクライアントは zipcloud.ibsnet.co.jp にしか届かない。
func TestZipcloudEgress(t *testing.T) {
	s := newZipServer(t)
	args := map[string]string{"zipcode": "1000001"}
	real := newZipcloud(s.URL, NewHTTPClient(zipcloudHosts, time.Second))
	_, err := real["address"].Do(context.Background(), Input{Args: args})
	if !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
	other := newZipcloud("https://example.com", NewHTTPClient(zipcloudHosts, time.Second))
	if _, err := other["address"].Do(context.Background(), Input{Args: args}); !errors.Is(err, ErrEgress) {
		t.Fatalf("another host: %v", err)
	}
}

// Service を通す: 1 回目は `runs` 1・同じ番号は 1 日キャッシュ・だめな引数は届かない。
func TestZipcloudCachedCall(t *testing.T) {
	s := newZipServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[Zipcloud] = true }})
	if !e.svc.Enabled(Zipcloud) {
		t.Fatal("zipcloud not on")
	}
	e.svc.ops[Zipcloud] = s.ops()
	args := map[string]string{"zipcode": "1000001"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Zipcloud, "address", args))
		if err != nil || len(res.Output.Rows) != 1 || res.Output.Item == nil || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(24 * time.Hour)
	if _, err := e.call(e.req(Zipcloud, "address", args)); err != nil || s.hits.Load() != 2 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	if _, err := e.call(e.req(Zipcloud, "address", map[string]string{"zipcode": "123"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 2 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
}
