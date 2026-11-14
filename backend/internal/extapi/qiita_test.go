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

// qiitaServer は qiita.com の代わり(本物は呼ばない)。
type qiitaServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value
	status int
	body   string
}

const qiitaBody = `[
{"title":" Go の context ","url":"https://qiita.com/gopher/items/abc123?utm=1","likes_count":42,"created_at":"2026-11-10T23:30:00+00:00","body":"long...","rendered_body":"<p>long</p>","user":{"id":"gopher","name":"G"}},
{"title":"二つ目","url":"https://qiita.com/someone/items/def456","likes_count":0,"created_at":"2026-01-02T09:00:00+09:00","user":{"id":"someone"}},
{"title":"","url":"https://qiita.com/x/items/1","likes_count":1,"created_at":"2026-01-02T09:00:00+09:00","user":{"id":"x"}},
{"title":"外のリンク","url":"https://evil.example.com/items/2","likes_count":1,"created_at":"2026-01-02T09:00:00+09:00","user":{"id":"x"}},
{"title":"http","url":"http://qiita.com/x/items/3","likes_count":1,"created_at":"bad","user":{"id":"x"}},
{"title":"日付なし","url":"https://qiita.com/x/items/4","likes_count":-3,"created_at":"bad","user":{"id":"x"}}
]`

func newQiitaServer(t *testing.T) *qiitaServer {
	t.Helper()
	s := &qiitaServer{status: http.StatusOK, body: qiitaBody}
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

func (s *qiitaServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *qiitaServer) ops() map[string]Operation { return newQiita(s.URL, s.Client()) }

func TestQiitaRegistered(t *testing.T) {
	p, ok := Lookup(Qiita)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.GlobalDaily != qiitaGlobalDaily || !slices.Equal(p.Hosts, []string{"https://qiita.com"}) {
		t.Fatalf("qiita %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.qiita") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	if len(p.Ops) != 2 || p.Ops[0].Name != "search" || p.Ops[1].Name != "userItems" {
		t.Fatalf("ops %+v", p.Ops)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 10*time.Minute {
			t.Fatalf("op %+v", o)
		}
	}
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || cfg.Enabled[Qiita] || cfg.Limits[Qiita].GlobalDaily != qiitaGlobalDaily {
		t.Fatalf("default: %v %v %+v", cfg.Enabled, err, cfg.Limits[Qiita])
	}
	cfg, err = ConfigFromEnv(func(k string) string { return map[string]string{"EXT_QIITA_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[Qiita] {
		t.Fatalf("EXT_QIITA_ENABLED: %v %v", cfg.Enabled, err)
	}
}

var qiitaWant = []Row{
	{Title: "Go の context", Subtitle: "gopher・2026/11/11", Value: "42", URL: "https://qiita.com/gopher/items/abc123"},
	{Title: "二つ目", Subtitle: "someone・2026/01/02", Value: "0", URL: "https://qiita.com/someone/items/def456"},
	{Title: "日付なし", Subtitle: "x", Value: "0", URL: "https://qiita.com/x/items/4"},
}

func TestQiitaSearch(t *testing.T) {
	s := newQiitaServer(t)
	out, err := s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": " tag:Go context ", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/api/v2/items?page=1&per_page=5&query=tag%3AGo+context" {
		t.Fatalf("request %s", s.uri())
	}
	if !slices.Equal(out.Rows, qiitaWant) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// 本文は行に出ない
	for _, r := range out.Rows {
		if strings.Contains(r.Title+r.Subtitle+r.Value+r.URL, "long") {
			t.Fatalf("body in a row: %+v", r)
		}
	}
	// limit は行を切る・既定は 10
	out, err = s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": "a", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("limit 1: %+v %v", out.Rows, err)
	}
	if _, err := s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": "a"}}); err != nil || !strings.Contains(s.uri(), "per_page=10") {
		t.Fatalf("default limit: %s %v", s.uri(), err)
	}
	// 空の答えは行 0(エラーでない)
	s.body = `[]`
	out, err = s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": "a"}})
	if err != nil || out.Rows == nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %+v %v", out.Rows, err)
	}
}

func TestQiitaUserItems(t *testing.T) {
	s := newQiitaServer(t)
	out, err := s.ops()["userItems"].Do(context.Background(), Input{Args: map[string]string{"userId": "gopher"}})
	if err != nil || s.uri() != "/api/v2/users/gopher/items?page=1&per_page=10" || !slices.Equal(out.Rows, qiitaWant) {
		t.Fatalf("userItems: %s %+v %v", s.uri(), out.Rows, err)
	}
	// 無い人は 400 bad_request(呼んだ後)
	s.status, s.body = http.StatusNotFound, `{"message":"Not found","type":"not_found"}`
	if _, err := s.ops()["userItems"].Do(context.Background(), Input{Args: map[string]string{"userId": "nobody"}}); !errors.Is(err, ErrUnknown) || Reason(err) != ReasonBadRequest {
		t.Fatalf("404: %v", err)
	}
}

func TestQiitaCheck(t *testing.T) {
	p, _ := Lookup(Qiita)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"search", map[string]string{"q": "Go"}, true},
		{"search", map[string]string{"q": "tag:Go title:context", "limit": "20"}, true},
		{"search", map[string]string{"q": strings.Repeat("あ", 100)}, true},
		{"search", nil, false},
		{"search", map[string]string{"q": "  "}, false},
		{"search", map[string]string{"q": strings.Repeat("あ", 101)}, false},
		{"search", map[string]string{"q": "a\nb"}, false},
		{"search", map[string]string{"q": "a", "limit": "0"}, false},
		{"search", map[string]string{"q": "a", "limit": "21"}, false},
		{"search", map[string]string{"q": "a", "limit": "x"}, false},
		{"search", map[string]string{"q": "https://qiita.com/x"}, false},
		{"search", map[string]string{"q": "a", "page": "2"}, false},
		{"userItems", map[string]string{"userId": "gopher"}, true},
		{"userItems", map[string]string{"userId": "a_b-1", "limit": "1"}, true},
		{"userItems", nil, false},
		{"userItems", map[string]string{"userId": ""}, false},
		{"userItems", map[string]string{"userId": "a/b"}, false},
		{"userItems", map[string]string{"userId": "日本語"}, false},
		{"userItems", map[string]string{"userId": strings.Repeat("a", 33)}, false},
		{"userItems", map[string]string{"userId": "a", "limit": "21"}, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%s %v: %v", c.op, c.args, err)
		}
	}
}

func TestQiitaFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && !pe.Billable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, `{"message":"Rate limit exceeded"}`, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && upstreamLimited(err)
		}},
		{http.StatusForbidden, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && !pe.Retryable }},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"title":"an object"}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `[{"title":"x","url":"https://qiita.com/x/items/1","body":"` + strings.Repeat("x", qiitaMaxBody) + `"}]`,
			func(err error) bool { return Reason(err) == ReasonUpstream }},
	}
	for _, c := range cases {
		s := newQiitaServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": "x"}})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
}

// 本物のクライアントは qiita.com にしか届かない。
func TestQiitaEgress(t *testing.T) {
	s := newQiitaServer(t)
	args := map[string]string{"q": "x"}
	real := newQiita(s.URL, NewHTTPClient(qiitaHosts, time.Second))
	_, err := real["search"].Do(context.Background(), Input{Args: args})
	if !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
	other := newQiita("https://example.com", NewHTTPClient(qiitaHosts, time.Second))
	if _, err := other["search"].Do(context.Background(), Input{Args: args}); !errors.Is(err, ErrEgress) {
		t.Fatalf("another host: %v", err)
	}
}

// Service を通す: 1 回目は `runs` 1・同じ引数は 10 分キャッシュ・だめな引数は届かない。
func TestQiitaCachedCall(t *testing.T) {
	s := newQiitaServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[Qiita] = true }})
	if !e.svc.Enabled(Qiita) {
		t.Fatal("qiita not on")
	}
	e.svc.ops[Qiita] = s.ops()
	args := map[string]string{"q": "Go"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Qiita, "search", args))
		if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(10 * time.Minute)
	if _, err := e.call(e.req(Qiita, "search", args)); err != nil || s.hits.Load() != 2 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	if _, err := e.call(e.req(Qiita, "search", map[string]string{"q": "a\nb"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 2 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
}
