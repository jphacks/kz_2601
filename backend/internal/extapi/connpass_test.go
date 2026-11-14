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

// cpServer は connpass.com の代わり(本物は呼ばない)。
type cpServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value
	key    atomic.Value // 最後の X-API-Key
	status int
	body   string
}

// 2026-11-11 12:00 JST(newEnv の時計)から見て: 終わった・今日・来週・月をまたぐ・時刻なし・外のリンク
const cpBody = `{"results_returned":6,"results_available":6,"results_start":1,"events":[
{"id":3,"title":"来週の勉強会","url":"https://connpass.com/event/3/?utm=x","started_at":"2026-11-18T19:00:00+09:00","ended_at":"2026-11-18T21:00:00+09:00","place":"金沢","accepted":12,"limit":30,"description":"<p>long</p>"},
{"id":1,"title":"終わった会","url":"https://connpass.com/event/1/","started_at":"2026-11-01T10:00:00+09:00","ended_at":"2026-11-01T12:00:00+09:00","place":"東京","accepted":5,"limit":10},
{"id":2,"title":" 今日の LT ","url":"https://kanazawa.connpass.com/event/2/","started_at":"2026-11-11T19:00:00+09:00","ended_at":"2026-11-11T20:30:00+09:00","place":"オンライン","accepted":40,"limit":null},
{"id":4,"title":"合宿","url":"https://connpass.com/event/4/","started_at":"2026-11-28T13:00:00+09:00","ended_at":"2026-11-29T12:00:00+09:00","place":"","accepted":8,"limit":0},
{"id":5,"title":"時刻なし","url":"https://connpass.com/event/5/","started_at":"","accepted":1},
{"id":6,"title":"外のリンク","url":"https://evil.example.com/event/6/","started_at":"2026-11-20T19:00:00+09:00","accepted":1}
]}`

func newCPServer(t *testing.T) *cpServer {
	t.Helper()
	s := &cpServer{status: http.StatusOK, body: cpBody}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.RequestURI())
		s.key.Store(r.Header.Get("X-API-Key"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *cpServer) uri() string { v, _ := s.last.Load().(string); return v }

var cpNow = func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, connpassJST) }

func (s *cpServer) ops(t *testing.T) map[string]Operation {
	t.Helper()
	ops, err := newConnpass("test-api-key-0001", s.URL, s.Client(), cpNow)
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestConnpassRegistered(t *testing.T) {
	p, ok := Lookup(Connpass)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.GlobalDaily != connpassGlobalDaily || !slices.Equal(p.Hosts, []string{"https://connpass.com"}) {
		t.Fatalf("connpass %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.connpass") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	if len(p.Ops) != 1 || p.Ops[0].Name != "events" || p.Ops[0].Write || p.Ops[0].CacheTTL != 10*time.Minute {
		t.Fatalf("ops %+v", p.Ops)
	}
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || cfg.Enabled[Connpass] || cfg.Limits[Connpass].GlobalDaily != connpassGlobalDaily {
		t.Fatalf("default: %v %v %+v", cfg.Enabled, err, cfg.Limits[Connpass])
	}
	cfg, err = ConfigFromEnv(func(k string) string { return map[string]string{"EXT_CONNPASS_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[Connpass] {
		t.Fatalf("EXT_CONNPASS_ENABLED: %v %v", cfg.Enabled, err)
	}
	// 鍵が無いと op は作れない
	if _, err := p.New(Secrets{}); err == nil {
		t.Fatal("no key: want an error")
	}
	if _, err := p.New(Secrets{APIKey: "key with space"}); err == nil {
		t.Fatal("bad key: want an error")
	}
}

func TestConnpassEvents(t *testing.T) {
	s := newCPServer(t)
	out, err := s.ops(t)["events"].Do(context.Background(), Input{Args: map[string]string{"keyword": " Go ", "prefecture": "ishikawa,online", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/api/v2/events/?count=50&keyword=Go&order=2&prefecture=ishikawa%2Conline&ym=202611" {
		t.Fatalf("request %s", s.uri())
	}
	if k, _ := s.key.Load().(string); k != "test-api-key-0001" {
		t.Fatalf("X-API-Key %q", k)
	}
	// 終わった会・時刻なし・外のリンクは出ない。開催の順。
	want := []Row{
		{Title: "今日の LT", Subtitle: "11/11(水) 19:00〜20:30・オンライン", Value: "40", URL: "https://kanazawa.connpass.com/event/2/"},
		{Title: "来週の勉強会", Subtitle: "11/18(水) 19:00〜21:00・金沢", Value: "12/30", URL: "https://connpass.com/event/3/"},
		{Title: "合宿", Subtitle: "11/28(土) 13:00〜11/29 12:00", Value: "8", URL: "https://connpass.com/event/4/"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	for _, r := range out.Rows {
		if strings.Contains(r.Title+r.Subtitle+r.Value+r.URL, "long") {
			t.Fatalf("description in a row: %+v", r)
		}
	}
	// ym を渡すとその月を全部(終わった会も)・limit で切る・既定 10
	out, err = s.ops(t)["events"].Do(context.Background(), Input{Args: map[string]string{"ym": "202611", "nickname": "someone", "limit": "2"}})
	if err != nil || s.uri() != "/api/v2/events/?count=50&nickname=someone&order=2&ym=202611" || len(out.Rows) != 2 || out.Rows[0].Title != "終わった会" {
		t.Fatalf("ym: %s %+v %v", s.uri(), out.Rows, err)
	}
	out, err = s.ops(t)["events"].Do(context.Background(), Input{Args: map[string]string{"ym": "202611"}})
	if err != nil || len(out.Rows) != 4 {
		t.Fatalf("default limit: %+v %v", out.Rows, err)
	}
	// 空の答えは行 0(エラーでない)
	s.body = `{"results_returned":0,"results_available":0,"results_start":1,"events":[]}`
	out, err = s.ops(t)["events"].Do(context.Background(), Input{})
	if err != nil || out.Rows == nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %+v %v", out.Rows, err)
	}
}

func TestConnpassWhen(t *testing.T) {
	st := time.Date(2026, 11, 20, 19, 0, 0, 0, connpassJST)
	for _, c := range []struct {
		end    time.Time
		hasEnd bool
		want   string
	}{
		{st.Add(2 * time.Hour), true, "11/20(金) 19:00〜21:00"},
		{st, true, "11/20(金) 19:00"},
		{time.Time{}, false, "11/20(金) 19:00"},
		{st.Add(20 * time.Hour), true, "11/20(金) 19:00〜11/21 15:00"},
		{time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), true, "11/20(金) 19:00〜21:00"}, // UTC は日本時間に直す
	} {
		if got := connpassWhen(st, c.end, c.hasEnd); got != c.want {
			t.Errorf("%v %v: %q, want %q", c.end, c.hasEnd, got, c.want)
		}
	}
}

func TestConnpassCheck(t *testing.T) {
	p, _ := Lookup(Connpass)
	for _, c := range []struct {
		args map[string]string
		ok   bool
	}{
		{nil, true},
		{map[string]string{"keyword": "Go", "prefecture": "tokyo,online", "ym": "202612", "limit": "20", "nickname": "a_b-1"}, true},
		{map[string]string{"keyword": strings.Repeat("あ", 100)}, true},
		{map[string]string{"keyword": ""}, false},
		{map[string]string{"keyword": "  "}, false},
		{map[string]string{"keyword": strings.Repeat("あ", 101)}, false},
		{map[string]string{"keyword": "a\nb"}, false},
		{map[string]string{"keyword": "https://connpass.com/event/1/"}, false},
		{map[string]string{"nickname": "a b"}, false},
		{map[string]string{"nickname": strings.Repeat("a", 33)}, false},
		{map[string]string{"prefecture": "Tokyo"}, false},
		{map[string]string{"prefecture": "tokyo,tokyo"}, false},
		{map[string]string{"prefecture": "tokyo,"}, false},
		{map[string]string{"prefecture": "tokyo,osaka,kyoto,hyogo,nara,shiga"}, false},
		{map[string]string{"ym": "2026-11"}, false},
		{map[string]string{"ym": "202613"}, false},
		{map[string]string{"ym": "20261"}, false},
		{map[string]string{"limit": "0"}, false},
		{map[string]string{"limit": "21"}, false},
		{map[string]string{"limit": "x"}, false},
		{map[string]string{"count": "1"}, false},
	} {
		_, err := p.CheckArgs("events", c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%v: %v", c.args, err)
		}
	}
}

func TestConnpassFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && !pe.Billable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && upstreamLimited(err)
		}},
		{http.StatusUnauthorized, `{"detail":"bad key"}`, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && !pe.Retryable }},
		{http.StatusForbidden, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && !pe.Retryable }},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"events":[],"pad":"` + strings.Repeat("x", connpassMaxBody) + `"}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
	}
	for _, c := range cases {
		s := newCPServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops(t)["events"].Do(context.Background(), Input{})
		if err == nil || !c.check(err) || strings.Contains(err.Error(), "test-api-key") {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
}

// 本物のクライアントは connpass.com にしか届かない。
func TestConnpassEgress(t *testing.T) {
	s := newCPServer(t)
	real, err := newConnpass("test-api-key-0001", s.URL, NewHTTPClient(connpassHosts, time.Second), cpNow)
	if err != nil {
		t.Fatal(err)
	}
	_, err = real["events"].Do(context.Background(), Input{})
	if !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
	other, _ := newConnpass("test-api-key-0001", "https://example.com", NewHTTPClient(connpassHosts, time.Second), cpNow)
	if _, err := other["events"].Do(context.Background(), Input{}); !errors.Is(err, ErrEgress) {
		t.Fatalf("another host: %v", err)
	}
}

// Service を通す: 1 回目は `runs` 1・同じ引数は 10 分キャッシュ・だめな引数は届かない。
func TestConnpassCachedCall(t *testing.T) {
	s := newCPServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Connpass] = true
		c.Secrets = map[string]Secrets{Connpass: {APIKey: "test-api-key-0001"}}
	}})
	if !e.svc.Enabled(Connpass) {
		t.Fatal("connpass not on")
	}
	e.svc.ops[Connpass] = s.ops(t)
	args := map[string]string{"keyword": "Go"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Connpass, "events", args))
		if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(10 * time.Minute)
	if _, err := e.call(e.req(Connpass, "events", args)); err != nil || s.hits.Load() != 2 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	if _, err := e.call(e.req(Connpass, "events", map[string]string{"ym": "2026-11"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 2 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
}
