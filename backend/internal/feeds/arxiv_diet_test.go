package feeds

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Fixtures are real responses recorded on 2026-11-08 (JST), cut to the
// first 4 entries / records:
//   - arxiv_api_cs_AI.xml: https://export.arxiv.org/api/query?search_query=cat:cs.AI&sortBy=submittedDate&sortOrder=descending&max_results=20
//   - kokkai_speech_kodomo.json: https://kokkai.ndl.go.jp/api/speech?any=こども家庭庁&maximumRecords=20&recordPacking=json

// source is a fake arXiv + 国会会議録 that records each request's query.
type source struct {
	srv *httptest.Server
	mu  sync.Mutex
	got []url.Values
}

func newSource(t *testing.T) *source {
	s := &source{}
	arxiv, diet := fixture(t, "arxiv_api_cs_AI.xml"), fixture(t, "kokkai_speech_kodomo.json")
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.got = append(s.got, r.URL.Query())
		s.mu.Unlock()
		switch r.URL.Path {
		case "/arxiv/query":
			_, _ = w.Write(arxiv)
		case "/kokkai/speech":
			_, _ = w.Write(diet)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *source) requests() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values{}, s.got...)
}

func (s *source) svc(c *clock, o Options) *Service {
	o.Client, o.Now = s.srv.Client(), c.now
	o.Endpoints = &Endpoints{Arxiv: s.srv.URL + "/arxiv/query", Diet: s.srv.URL + "/kokkai/speech"}
	svc := New(o)
	svc.sleep = func(_ context.Context, d time.Duration) bool { c.add(d); return true }
	return svc
}

func TestArxiv(t *testing.T) {
	src := newSource(t)
	s := src.svc(&clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedArxiv, FeedID: "cs.AI"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "arXiv cs.AI(人工知能)の新着" || res.Source.Name != "arXiv.org" ||
		res.Source.URL != "https://arxiv.org/list/cs.AI/recent" || !strings.Contains(res.Source.License, "CC0") {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	if len(res.Items) != 4 {
		t.Fatalf("items = %d", len(res.Items))
	}
	first, third := res.Items[0], res.Items[2]
	if first.Title != "SwarmEvolve: Latent Agent Swarms for Evolutionary Discovery and Optimization" ||
		first.URL != "https://arxiv.org/abs/2611.08483v1" || first.Date != "2026-11-06T03:59:29+09:00" ||
		first.Subtitle != "Qile Jiang, George Em Karniadakis" {
		t.Errorf("first = %+v", first)
	}
	if third.Subtitle != "Linrong Cai, Taiming Lu, Haozhe Chen, ほか" {
		t.Errorf("authors = %q", third.Subtitle)
	}
	q := src.requests()[0]
	if q.Get("search_query") != "cat:cs.AI" || q.Get("sortBy") != "submittedDate" || q.Get("sortOrder") != "descending" || q.Get("max_results") != "20" {
		t.Errorf("query = %v", q)
	}
}

func TestDiet(t *testing.T) {
	src := newSource(t)
	s := src.svc(&clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: "　こども家庭庁  "}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "国会会議録「こども家庭庁」" || res.Source.URL != "https://kokkai.ndl.go.jp/" {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	var got []string
	for _, it := range res.Items {
		got = append(got, it.Date+" | "+it.Title+" | "+it.Subtitle+" | "+it.URL)
	}
	// The order-0 header records (会議録情報) are not speeches.
	want := []string{
		"2026-10-23 | 中司宏(日本維新の会) | 参議院 こども・子育て・若者活躍に関する特別委員会 第2号 | https://kokkai.ndl.go.jp/txt/122215386X00220261023/3",
		"2026-07-22 | 舟山康江(国民民主党・新緑風会) | 参議院 こども・子育て・若者活躍に関する特別委員会 第5号 | https://kokkai.ndl.go.jp/txt/122115386X00520260722/2",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	q := src.requests()[0]
	if q.Get("any") != "こども家庭庁" || q.Get("maximumRecords") != "20" || q.Get("recordPacking") != "json" || len(q) != 3 {
		t.Errorf("query = %v", q)
	}
	// Same words after normalizing = same cache entry.
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: "こども家庭庁"}, 5); err != nil || len(src.requests()) != 1 {
		t.Errorf("second get: %v, %d requests", err, len(src.requests()))
	}
}

func TestParseDietRejects(t *testing.T) {
	for _, bad := range []string{`{"message":"検索条件を指定してください"}`, `[]`, `<html>`} {
		if _, err := parseDiet([]byte(bad)); err == nil {
			t.Errorf("%s should fail", bad)
		}
	}
	if items, err := parseDiet([]byte(`{"numberOfRecords":0}`)); err != nil || len(items) != 0 {
		t.Errorf("no hits = %v %v", items, err)
	}
	if _, err := parseArxiv([]byte(`<rss><channel/></rss>`)); err == nil {
		t.Error("non-Atom arxiv body should fail")
	}
}

func TestArxivDietRejectedBeforeNetwork(t *testing.T) {
	src := newSource(t)
	s := src.svc(&clock{t: testNow}, Options{})
	for _, q := range []Query{
		{Provider: dsl.FeedArxiv},
		{Provider: dsl.FeedArxiv, FeedID: "math.QA"},       // not allowlisted
		{Provider: dsl.FeedArxiv, FeedID: "cs.AI+cs.CL"},   // no combining
		{Provider: dsl.FeedArxiv, FeedID: "cs.AI", Q: "x"}, // q is diet only
		{Provider: dsl.FeedArxiv, FeedID: "cs.AI", Region: "130000"},
		{Provider: dsl.FeedDiet},
		{Provider: dsl.FeedDiet, Q: "   "},
		{Provider: dsl.FeedDiet, Q: strings.Repeat("あ", 41)},
		{Provider: dsl.FeedDiet, Q: "予算\n委員会"},
		{Provider: dsl.FeedDiet, Q: "予算\u200b"},
		{Provider: dsl.FeedDiet, Q: "予算\x00"},
		{Provider: dsl.FeedDiet, Q: "予算", FeedID: "cs.AI"},
		{Provider: dsl.FeedWeather, Region: "130000", Q: "x"},
	} {
		if _, err := s.Get(context.Background(), q, 5); !errors.Is(err, ErrUnknown) {
			t.Errorf("%+v: err = %v, want ErrUnknown", q, err)
		}
	}
	if n := len(src.requests()); n != 0 {
		t.Errorf("rejected queries reached the source %d times", n)
	}
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: strings.Repeat("あ", 40)}, 5); err != nil {
		t.Errorf("40 characters: %v", err)
	}
}

// arXiv and 国会会議録 ask for one request at a time, a few seconds apart.
func TestGapProviders(t *testing.T) {
	src := newSource(t)
	c := &clock{t: testNow}
	s := src.svc(c, Options{})
	start := c.now()
	for _, id := range []string{"cs.AI", "cs.CL", "cs.CV"} {
		if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedArxiv, FeedID: id}, 5); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if d := c.now().Sub(start); d != 6*time.Second {
		t.Errorf("3 categories took %s of waiting, want 6s (3 s apart)", d)
	}
	// diet has its own turn: no wait right after arxiv.
	before := c.now()
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: "予算"}, 5); err != nil || c.now() != before {
		t.Errorf("diet: %v, waited %s", err, c.now().Sub(before))
	}
	// A caller that cannot wait gets ErrBusy (503), not a burst.
	s.sleep = func(context.Context, time.Duration) bool { return false }
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedDiet, Q: "決算"}, 5); !errors.Is(err, ErrBusy) {
		t.Errorf("no wait: %v, want ErrBusy", err)
	}
	if n := len(src.requests()); n != 4 {
		t.Errorf("requests = %d, want 4", n)
	}
}

func TestNewProvidersSwitchOffOneByOne(t *testing.T) {
	src := newSource(t)
	env := map[string]string{"FEEDS_DISABLED": "arxiv, diet"}
	s := src.svc(&clock{t: testNow}, OptionsFromEnv(func(k string) string { return env[k] }))
	for _, q := range []Query{{Provider: dsl.FeedArxiv, FeedID: "cs.AI"}, {Provider: dsl.FeedDiet, Q: "予算"}} {
		if _, err := s.Get(context.Background(), q, 5); !errors.Is(err, ErrDisabled) || s.Enabled(q.Provider) {
			t.Errorf("%s: %v, want ErrDisabled", q.Provider, err)
		}
	}
	if !s.Enabled(dsl.FeedWeather) || len(src.requests()) != 0 {
		t.Errorf("weather enabled = %v, requests = %d", s.Enabled(dsl.FeedWeather), len(src.requests()))
	}
	env["FEEDS_DISABLED"] = "diet"
	s = src.svc(&clock{t: testNow}, OptionsFromEnv(func(k string) string { return env[k] }))
	if !s.Enabled(dsl.FeedArxiv) || s.Enabled(dsl.FeedDiet) {
		t.Error("diet alone should be off")
	}
}

// Free search words must not grow the cache without bound.
func TestCachePrune(t *testing.T) {
	s := New(Options{Endpoints: &Endpoints{}})
	now := testNow
	for i := range maxCacheEntries {
		at := now.Add(-time.Duration(i) * time.Second)
		s.cache[fmt.Sprint("k", i)] = &entry{items: []Item{}, fetchedAt: at, expires: at.Add(time.Hour)}
	}
	s.cache["old"] = &entry{fetchedAt: now.Add(-48 * time.Hour), expires: now.Add(-47 * time.Hour)}
	s.prune(now)
	if len(s.cache) != maxCacheEntries-1 {
		t.Fatalf("cache = %d, want %d", len(s.cache), maxCacheEntries-1)
	}
	if s.cache["old"] != nil || s.cache[fmt.Sprint("k", maxCacheEntries-1)] != nil || s.cache["k0"] == nil {
		t.Error("prune should drop the long-stale entry and then the oldest one")
	}
}

func TestArxivDietEndpoints(t *testing.T) {
	ep := DefaultEndpoints()
	if got := ep.arxivURL("cs.CL"); got != "https://export.arxiv.org/api/query?max_results=20&search_query=cat%3Acs.CL&sortBy=submittedDate&sortOrder=descending" {
		t.Errorf("arxiv url = %s", got)
	}
	if got := ep.dietURL("こども 予算&x=1"); got != "https://kokkai.ndl.go.jp/api/speech?any=%E3%81%93%E3%81%A9%E3%82%82+%E4%BA%88%E7%AE%97%26x%3D1&maximumRecords=20&recordPacking=json" {
		t.Errorf("diet url = %s", got)
	}
	for _, raw := range []string{ep.arxivURL("cs.AI"), ep.dietURL("予算")} {
		if !ep.allowedURL(raw) {
			t.Errorf("%s not allowed", raw)
		}
	}
	for id, name := range dsl.ArxivCategories {
		if name == "" || strings.ContainsAny(id, "+&?/ ") {
			t.Errorf("bad arxiv category %q", id)
		}
	}
}
