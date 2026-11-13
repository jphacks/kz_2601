package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// podServer is a stand-in for itunes.apple.com (no live calls).
type podServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value // the last request URI
	status int
	body   string
}

const podBody = `{"resultCount":3,"results":[
{"wrapperType":"track","kind":"podcast","collectionId":101,"collectionName":"テック雑談","artistName":"山田","feedUrl":"https://feeds.example.com/tech.xml",
 "collectionViewUrl":"https://podcasts.apple.com/jp/podcast/tech/id101?uo=4","artworkUrl600":"https://is1-ssl.mzstatic.com/image/thumb/a/600x600bb.jpg","artworkUrl100":"https://is1-ssl.mzstatic.com/image/thumb/a/100x100bb.jpg"},
{"wrapperType":"track","kind":"podcast","collectionId":102,"collectionName":"朝のニュース","artistName":"放送局",
 "collectionViewUrl":"https://podcasts.apple.com/jp/podcast/news/id102","artworkUrl100":"https://is2-ssl.mzstatic.com/image/thumb/b/100x100bb.jpg"},
{"wrapperType":"track","kind":"podcast","collectionId":103,"collectionName":"三つ目","artistName":"誰か","feedUrl":"https://feeds.example.com/3.xml",
 "collectionViewUrl":"https://podcasts.apple.com/jp/podcast/3/id103","artworkUrl600":"https://evil.example.com/a.jpg"}
]}`

func newPodServer(t *testing.T) *podServer {
	t.Helper()
	s := &podServer{status: http.StatusOK, body: podBody}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.RequestURI())
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *podServer) uri() string { v, _ := s.last.Load().(string); return v }

func (s *podServer) ops() map[string]Operation { return newPodcasts(s.URL, s.Client()) }

func TestPodcastsRegistered(t *testing.T) {
	p, ok := Lookup(Podcasts)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || !slices.Equal(p.Hosts, []string{"https://itunes.apple.com"}) {
		t.Fatalf("podcasts %+v", p)
	}
	if len(p.Ops) != 2 || p.Ops[0].Name != "searchPodcasts" || p.Ops[1].Name != "newEpisodes" {
		t.Fatalf("ops %+v", p.Ops)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 6*time.Hour {
			t.Fatalf("op %+v", o)
		}
	}
	// default off: only EXT_PODCASTS_ENABLED=1 turns it on
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || cfg.Enabled[Podcasts] {
		t.Fatalf("default: %v %v", cfg.Enabled, err)
	}
	cfg, err = ConfigFromEnv(func(k string) string { return map[string]string{"EXT_PODCASTS_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[Podcasts] {
		t.Fatalf("EXT_PODCASTS_ENABLED: %v %v", cfg.Enabled, err)
	}
}

func TestPodcastsSearch(t *testing.T) {
	s := newPodServer(t)
	out, err := s.ops()["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": " テック ", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/search?country=JP&entity=podcast&lang=ja_jp&limit=5&media=podcast&term=%E3%83%86%E3%83%83%E3%82%AF" {
		t.Fatalf("request %s", s.uri())
	}
	want := []Row{
		{Title: "テック雑談", Subtitle: "山田", Value: podcastsFeedYes, URL: "https://podcasts.apple.com/jp/podcast/tech/id101"},
		{Title: "朝のニュース", Subtitle: "放送局", Value: podcastsFeedNo, URL: "https://podcasts.apple.com/jp/podcast/news/id102"},
		{Title: "三つ目", Subtitle: "誰か", Value: podcastsFeedYes, URL: "https://podcasts.apple.com/jp/podcast/3/id103"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	cells := [][]string{
		{"テック雑談", "山田", podcastsFeedYes, "https://is1-ssl.mzstatic.com/image/thumb/a/600x600bb.jpg", "https://podcasts.apple.com/jp/podcast/tech/id101", "101"},
		{"朝のニュース", "放送局", podcastsFeedNo, "https://is2-ssl.mzstatic.com/image/thumb/b/100x100bb.jpg", "https://podcasts.apple.com/jp/podcast/news/id102", "102"},
		{"三つ目", "誰か", podcastsFeedYes, "", "https://podcasts.apple.com/jp/podcast/3/id103", "103"},
	}
	if !slices.EqualFunc(out.Cells, cells, slices.Equal) {
		t.Fatalf("cells %+v", out.Cells)
	}
	// the feed URL never leaves the provider
	for _, r := range out.Rows {
		if strings.Contains(r.Title+r.Subtitle+r.Value+r.URL, "feeds.example.com") {
			t.Fatalf("feed URL in a row: %+v", r)
		}
	}
	// limit cuts the rows; default limit 10
	out, err = s.ops()["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": "a", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 || len(out.Cells) != 1 {
		t.Fatalf("limit 1: %+v %v", out.Rows, err)
	}
	if _, err := s.ops()["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": "a"}}); err != nil || !strings.Contains(s.uri(), "limit=10") {
		t.Fatalf("default limit: %s %v", s.uri(), err)
	}
}

func TestPodcastsDropsOddResults(t *testing.T) {
	s := newPodServer(t)
	s.body = `{"results":[
{"kind":"podcast-episode","collectionId":1,"collectionName":"回","collectionViewUrl":"https://podcasts.apple.com/jp/podcast/x/id1"},
{"kind":"podcast","collectionId":2,"collectionName":"","collectionViewUrl":"https://podcasts.apple.com/jp/podcast/x/id2"},
{"kind":"podcast","collectionId":3,"collectionName":"店なし"},
{"kind":"podcast","collectionId":4,"collectionName":"偽の店","collectionViewUrl":"https://evil.example.com/id4"},
{"kind":"podcast","collectionId":5,"collectionName":"http","collectionViewUrl":"http://podcasts.apple.com/id5"},
{"kind":"podcast","collectionId":0,"collectionName":"id なし","collectionViewUrl":"https://podcasts.apple.com/id0"},
{"kind":"podcast","collectionId":6,"collectionName":"残る","collectionViewUrl":"https://podcasts.apple.com/jp/podcast/ok/id6","feedUrl":"javascript:alert(1)"}
]}`
	out, err := s.ops()["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": "x"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "残る" || out.Rows[0].Value != podcastsFeedNo {
		t.Fatalf("odd results: %+v %v", out.Rows, err)
	}
	// no results: zero rows, not an error
	s.body = `{"resultCount":0,"results":[]}`
	out, err = s.ops()["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": "x"}})
	if err != nil || out.Rows == nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %+v %v", out.Rows, err)
	}
}

func TestPodcastsCheck(t *testing.T) {
	p, _ := Lookup(Podcasts)
	for _, c := range []struct {
		args map[string]string
		ok   bool
	}{
		{map[string]string{"term": "テック"}, true},
		{map[string]string{"term": "rebuild fm", "limit": "20"}, true},
		{map[string]string{"term": strings.Repeat("あ", 100)}, true},
		{nil, false},
		{map[string]string{"term": "  "}, false},
		{map[string]string{"term": strings.Repeat("あ", 101)}, false},
		{map[string]string{"term": "a\nb"}, false},
		{map[string]string{"term": "a", "limit": "0"}, false},
		{map[string]string{"term": "a", "limit": "21"}, false},
		{map[string]string{"term": "a", "limit": "x"}, false},
		{map[string]string{"term": "https://feeds.example.com/a.xml"}, false},
		{map[string]string{"term": "a", "country": "US"}, false},
	} {
		_, err := p.CheckArgs("searchPodcasts", c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%v: %v", c.args, err)
		}
	}
}

func TestPodcastsFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && pe.Retryable }},
		{http.StatusForbidden, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && !pe.Retryable }},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"results":[],"pad":"` + strings.Repeat("x", MaxBody) + `"}`, func(err error) bool { return err != nil }},
	}
	for _, c := range cases {
		s := newPodServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops()["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": "x"}})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
}

// The real client reaches itunes.apple.com only.
func TestPodcastsEgress(t *testing.T) {
	s := newPodServer(t)
	real := newPodcasts(s.URL, NewHTTPClient(podcastsHosts, time.Second))
	_, err := real["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": "x"}})
	if !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
	other := newPodcasts("https://podcasts.apple.com", NewHTTPClient(podcastsHosts, time.Second))
	if _, err := other["searchPodcasts"].Do(context.Background(), Input{Args: map[string]string{"term": "x"}}); !errors.Is(err, ErrEgress) {
		t.Fatalf("another host: %v", err)
	}
}

// Through the Service: one call, then the cache for 6 hours.
func TestPodcastsCachedCall(t *testing.T) {
	s := newPodServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[Podcasts] = true }})
	if !e.svc.Enabled(Podcasts) {
		t.Fatal("podcasts not on")
	}
	e.svc.ops[Podcasts] = s.ops()
	args := map[string]string{"term": "テック"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Podcasts, "searchPodcasts", args))
		if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(6 * time.Hour)
	if _, err := e.call(e.req(Podcasts, "searchPodcasts", args)); err != nil || s.hits.Load() != 2 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	if _, err := e.call(e.req(Podcasts, "searchPodcasts", map[string]string{"term": "a\nb"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 2 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
}

// The Lookup API's answer: the show first, then episodes (not newest first
// here, to check the sort). Every episode carries audio URLs.
const podEpisodesBody = `{"resultCount":4,"results":[
{"wrapperType":"track","kind":"podcast","collectionId":101,"collectionName":"テック雑談","feedUrl":"https://feeds.example.com/tech.xml",
 "collectionViewUrl":"https://podcasts.apple.com/jp/podcast/tech/id101?uo=4"},
{"wrapperType":"podcastEpisode","kind":"podcast-episode","trackId":9001,"trackName":"第1回 はじめまして","collectionName":"テック雑談","releaseDate":"2026-11-01T21:00:00Z",
 "trackViewUrl":"https://podcasts.apple.com/jp/podcast/ep1/id101?i=9001&uo=4","episodeUrl":"https://audio.example.com/ep1.mp3","previewUrl":"https://audio.example.com/p1.mp3"},
{"wrapperType":"podcastEpisode","kind":"podcast-episode","trackId":9002,"trackName":"第2回 Go の話","collectionName":"テック雑談","releaseDate":"2026-11-08T21:00:00Z",
 "trackViewUrl":"https://podcasts.apple.com/jp/podcast/ep2/id101?i=9002","episodeUrl":"https://audio.example.com/ep2.mp3"},
{"wrapperType":"podcastEpisode","kind":"podcast-episode","trackId":9003,"trackName":"偽のリンク","collectionName":"テック雑談","releaseDate":"2026-11-09T21:00:00Z",
 "trackViewUrl":"https://evil.example.com/ep3","episodeUrl":"https://audio.example.com/ep3.mp3"}
]}`

func TestPodcastsNewEpisodes(t *testing.T) {
	s := newPodServer(t)
	s.body = podEpisodesBody
	out, err := s.ops()["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": " 101 ", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.uri() != "/lookup?country=JP&entity=podcastEpisode&id=101&limit=5" {
		t.Fatalf("request %s", s.uri())
	}
	want := []Row{
		{Title: "第2回 Go の話", Subtitle: "テック雑談", Value: "2026/11/9", URL: "https://podcasts.apple.com/jp/podcast/ep2/id101"},
		{Title: "第1回 はじめまして", Subtitle: "テック雑談", Value: "2026/11/2", URL: "https://podcasts.apple.com/jp/podcast/ep1/id101"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	cells := [][]string{
		{"第2回 Go の話", "テック雑談", "2026-11-08T21:00:00Z", "https://podcasts.apple.com/jp/podcast/ep2/id101"},
		{"第1回 はじめまして", "テック雑談", "2026-11-01T21:00:00Z", "https://podcasts.apple.com/jp/podcast/ep1/id101"},
	}
	if !slices.EqualFunc(out.Cells, cells, slices.Equal) {
		t.Fatalf("cells %+v", out.Cells)
	}
	// limit cuts the rows; out-of-range limits are clamped; default 10
	out, err = s.ops()["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": "101", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "第2回 Go の話" {
		t.Fatalf("limit 1: %+v %v", out.Rows, err)
	}
	for in, want := range map[string]string{"0": "limit=1", "-3": "limit=1", "500": "limit=20", "": "limit=10"} {
		if _, err := s.ops()["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": "101", "limit": in}}); err != nil || !strings.HasSuffix(s.uri(), want) {
			t.Errorf("limit %q: %s %v", in, s.uri(), err)
		}
	}
	// a show without episodes: zero rows, not an error
	s.body = `{"resultCount":1,"results":[{"wrapperType":"track","kind":"podcast","collectionId":101,"collectionName":"x"}]}`
	out, err = s.ops()["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": "101"}})
	if err != nil || out.Rows == nil || len(out.Rows) != 0 {
		t.Fatalf("empty: %+v %v", out.Rows, err)
	}
}

// The audio URL is never in the output (rows or cells).
func TestPodcastsNewEpisodesNoAudio(t *testing.T) {
	s := newPodServer(t)
	s.body = podEpisodesBody
	out, err := s.ops()["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": "101"}})
	if err != nil || len(out.Rows) == 0 {
		t.Fatalf("%+v %v", out, err)
	}
	all := fmt.Sprintf("%+v", out)
	for _, bad := range []string{"audio.example.com", ".mp3", "feeds.example.com", "?i=", "uo=4"} {
		if strings.Contains(all, bad) {
			t.Fatalf("%q in the output: %s", bad, all)
		}
	}
}

func TestPodcastsNewEpisodesCheck(t *testing.T) {
	p, _ := Lookup(Podcasts)
	for _, c := range []struct {
		args map[string]string
		ok   bool
	}{
		{map[string]string{"collectionId": "101"}, true},
		{map[string]string{"collectionId": "1535809341", "limit": "20"}, true},
		{map[string]string{"collectionId": "101", "limit": "0"}, true},   // clamped
		{map[string]string{"collectionId": "101", "limit": "999"}, true}, // clamped
		{nil, false},
		{map[string]string{"collectionId": ""}, false},
		{map[string]string{"collectionId": "0"}, false},
		{map[string]string{"collectionId": "abc"}, false},
		{map[string]string{"collectionId": "101a"}, false},
		{map[string]string{"collectionId": "-101"}, false},
		{map[string]string{"collectionId": "1.5"}, false},
		{map[string]string{"collectionId": "101&entity=x"}, false},
		{map[string]string{"collectionId": "１０１"}, false},
		{map[string]string{"collectionId": strings.Repeat("1", 16)}, false},
		{map[string]string{"collectionId": "101", "limit": "x"}, false},
		{map[string]string{"collectionId": "101", "country": "US"}, false},
	} {
		_, err := p.CheckArgs("newEpisodes", c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%v: %v", c.args, err)
		}
	}
	// the op itself refuses a bad id without calling out
	s := newPodServer(t)
	if _, err := s.ops()["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": "abc"}}); !errors.Is(err, ErrUnknown) || s.hits.Load() != 0 {
		t.Fatalf("bad id: %v hits=%d", err, s.hits.Load())
	}
}

func TestPodcastsNewEpisodesFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusServiceUnavailable, ``, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && pe.Retryable }},
		{http.StatusNotFound, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && !pe.Retryable }},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"results":[],"pad":"` + strings.Repeat("x", MaxBody) + `"}`, func(err error) bool { return err != nil }},
	}
	for _, c := range cases {
		s := newPodServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops()["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": "101"}})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
	// the real client reaches itunes.apple.com only
	s := newPodServer(t)
	real := newPodcasts(s.URL, NewHTTPClient(podcastsHosts, time.Second))
	if _, err := real["newEpisodes"].Do(context.Background(), Input{Args: map[string]string{"collectionId": "101"}}); !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
}

// Through the Service: the shared 6-hour cache, keyed by the id.
func TestPodcastsNewEpisodesCached(t *testing.T) {
	s := newPodServer(t)
	s.body = podEpisodesBody
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[Podcasts] = true }})
	e.svc.ops[Podcasts] = s.ops()
	args := map[string]string{"collectionId": "101"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Podcasts, "newEpisodes", args))
		if err != nil || len(res.Output.Rows) != 2 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
		if strings.Contains(fmt.Sprintf("%+v", res.Output), "audio.example.com") {
			t.Fatalf("audio URL through the service: %+v", res.Output)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	if _, err := e.call(e.req(Podcasts, "newEpisodes", map[string]string{"collectionId": "102"})); err != nil || s.hits.Load() != 2 {
		t.Fatalf("another id: %v hits=%d", err, s.hits.Load())
	}
	e.c.t = e.c.t.Add(6 * time.Hour)
	if _, err := e.call(e.req(Podcasts, "newEpisodes", args)); err != nil || s.hits.Load() != 3 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	if _, err := e.call(e.req(Podcasts, "newEpisodes", map[string]string{"collectionId": "abc"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 3 {
		t.Fatalf("bad id: %v hits=%d", err, s.hits.Load())
	}
}
