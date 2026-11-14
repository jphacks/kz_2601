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

// wikiServer は ja/en.wikipedia.org の代わり(本物は呼ばない)。
type wikiServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value // 最後の要求の URI
	ua     atomic.Value
	status int
	body   string
}

func newWikiServer(t *testing.T) *wikiServer {
	t.Helper()
	s := &wikiServer{status: http.StatusOK}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.RequestURI())
		s.ua.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		body := s.body
		if body == "" {
			if strings.HasPrefix(r.URL.Path, "/api/rest_v1/page/summary/") {
				body = `{"type":"standard","title":"金沢市","description":"石川県の県庁所在地","extract":"` + strings.Repeat("あ", 350) + `",` +
					`"content_urls":{"desktop":{"page":"` + s.URL + `/wiki/%E9%87%91%E6%B2%A2%E5%B8%82"}}}`
			} else {
				body = `["金沢",["金沢市","金沢駅","金沢城"],["石川県の市","",""],["` + s.URL + `/wiki/A","https://evil.example/B","` + s.URL + `/wiki/C"]]`
			}
		}
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *wikiServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *wikiServer) ops() map[string]Operation {
	return newWikipedia(map[string]string{"ja": s.URL, "en": s.URL}, s.Client())
}

func TestWikipediaRegistered(t *testing.T) {
	p, ok := Lookup(Wikipedia)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://ja.wikipedia.org", "https://en.wikipedia.org"}) {
		t.Fatalf("wikipedia %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.wikipedia") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL != time.Hour {
			t.Fatalf("%s: read ops, cached 1 hour", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string { return map[string]string{"EXT_WIKIPEDIA_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[Wikipedia] {
		t.Fatalf("EXT_WIKIPEDIA_ENABLED: %v %v", cfg.Enabled, err)
	}
	if off, err := ConfigFromEnv(func(string) string { return "" }); err != nil || off.Enabled[Wikipedia] {
		t.Fatalf("on by default: %v", err)
	}
}

func TestWikipediaSummary(t *testing.T) {
	s := newWikiServer(t)
	out, err := s.ops()["summary"].Do(context.Background(), Input{Args: map[string]string{"title": "金沢 市"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/api/rest_v1/page/summary/%E9%87%91%E6%B2%A2_%E5%B8%82" {
		t.Fatalf("request %s", s.uri())
	}
	if ua, _ := s.ua.Load().(string); ua != "TailorIt/1.0 (https://tailor-it.app)" {
		t.Fatalf("user agent %q", ua)
	}
	if len(out.Rows) != 1 || out.Item == nil || *out.Item != out.Rows[0] {
		t.Fatalf("rows %+v item %+v", out.Rows, out.Item)
	}
	r := out.Rows[0]
	if r.Title != "金沢市" || r.Subtitle != "石川県の県庁所在地" || len([]rune(r.Value)) != 301 || !strings.HasSuffix(r.Value, "…") ||
		r.URL != s.URL+"/wiki/%E9%87%91%E6%B2%A2%E5%B8%82" {
		t.Fatalf("row %+v", r)
	}
}

func TestWikipediaSearch(t *testing.T) {
	s := newWikiServer(t)
	out, err := s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": "金沢", "lang": "en", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/w/api.php?action=opensearch&format=json&limit=5&namespace=0&search=%E9%87%91%E6%B2%A2" {
		t.Fatalf("request %s", s.uri())
	}
	want := []Row{{Title: "金沢市", Subtitle: "石川県の市", URL: s.URL + "/wiki/A"}, {Title: "金沢駅"}, {Title: "金沢城", URL: s.URL + "/wiki/C"}}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	// limit より多く返っても切る
	out, err = s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": "金沢", "limit": "2"}})
	if err != nil || len(out.Rows) != 2 {
		t.Fatalf("limit: %+v %v", out.Rows, err)
	}
}

func TestWikipediaCheck(t *testing.T) {
	p, _ := Lookup(Wikipedia)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"summary", map[string]string{"title": "金沢市"}, true},
		{"summary", map[string]string{"title": "Kanazawa", "lang": "en"}, true},
		{"summary", nil, false},
		{"summary", map[string]string{"title": "  "}, false},
		{"summary", map[string]string{"title": "a\nb"}, false},
		{"summary", map[string]string{"title": strings.Repeat("あ", 201)}, false},
		{"summary", map[string]string{"title": "x", "lang": "fr"}, false},
		{"summary", map[string]string{"title": "x", "lang": "JA"}, false},
		{"summary", map[string]string{"title": "https://evil.example/x"}, false},
		{"summary", map[string]string{"title": ".."}, false},
		{"summary", map[string]string{"title": " . "}, false},
		{"summary", map[string]string{"title": "..."}, true},
		{"summary", map[string]string{"q": "x"}, false},
		{"search", map[string]string{"q": "金沢", "limit": "20"}, true},
		{"search", map[string]string{"q": ""}, false},
		{"search", map[string]string{"q": "x", "limit": "0"}, false},
		{"search", map[string]string{"q": "x", "limit": "21"}, false},
		{"search", map[string]string{"q": "x", "limit": "x"}, false},
		{"search", map[string]string{"q": "x", "lang": "de"}, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%s %v: %v", c.op, c.args, err)
		}
	}
}

func TestWikipediaFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusNotFound, `{"title":"Not found."}`, func(err error) bool { return errors.Is(err, ErrUnknown) && Reason(err) == ReasonBadRequest }},
		{http.StatusBadRequest, `{}`, func(err error) bool { return Reason(err) == ReasonBadRequest }},
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && !pe.Billable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && upstreamLimited(err)
		}},
		{http.StatusForbidden, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && !pe.Retryable && Reason(err) == ReasonUpstream
		}},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"description":"no title"}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"title":"x","extract":"` + strings.Repeat("x", wikipediaMaxBody) + `"}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
	}
	for _, c := range cases {
		s := newWikiServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops()["summary"].Do(context.Background(), Input{Args: map[string]string{"title": "x"}})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
	// search の形が違う
	s := newWikiServer(t)
	s.body = `["q",["a"]]`
	if _, err := s.ops()["search"].Do(context.Background(), Input{Args: map[string]string{"q": "q"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("short answer: %v", err)
	}
}

// 本物のクライアントは ja / en の wikipedia.org にしか届かない。
func TestWikipediaEgress(t *testing.T) {
	s := newWikiServer(t)
	real := newWikipedia(map[string]string{"ja": s.URL}, NewHTTPClient(wikipediaHosts, time.Second))
	_, err := real["summary"].Do(context.Background(), Input{Args: map[string]string{"title": "x"}})
	if !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
	other := newWikipedia(map[string]string{"ja": "https://fr.wikipedia.org"}, NewHTTPClient(wikipediaHosts, time.Second))
	if _, err := other["summary"].Do(context.Background(), Input{Args: map[string]string{"title": "x"}}); !errors.Is(err, ErrEgress) {
		t.Fatalf("another host: %v", err)
	}
}

// Service 経由: 同じ引数は 1 時間キャッシュ(`runs` は 1 回だけ)。
func TestWikipediaCachedCall(t *testing.T) {
	s := newWikiServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[Wikipedia] = true }})
	e.svc.ops[Wikipedia] = s.ops()
	args := map[string]string{"title": "金沢市"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Wikipedia, "summary", args))
		if err != nil || len(res.Output.Rows) != 1 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(time.Hour)
	if _, err := e.call(e.req(Wikipedia, "summary", args)); err != nil || s.hits.Load() != 2 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	if _, err := e.call(e.req(Wikipedia, "summary", map[string]string{"title": "x", "lang": "de"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 2 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
}
