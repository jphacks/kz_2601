package appconfig

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// A fake source (httptest); no test reaches a real one.
func feedsEnv(t *testing.T, perHour int, disabled ...string) (*env, *atomic.Int32) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`<rss version="2.0"><channel><item><title>お知らせ</title><link>https://www.example.go.jp/a</link><pubDate>Thu, 05 Nov 2026 18:30:00 +0900</pubDate></item></channel></rss>`))
	}))
	t.Cleanup(srv.Close)
	now := func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) }
	svc := feeds.New(feeds.Options{Client: srv.Client(), Now: now, Disabled: disabled, Endpoints: &feeds.Endpoints{
		WeatherBase: srv.URL + "/w/", Holidays: srv.URL + "/h.csv",
		RSS: map[string]feeds.RSSFeed{"digital-news": {Name: "デジタル庁 新着・更新", URL: srv.URL + "/rss", Publisher: "デジタル庁", SiteURL: "https://www.digital.go.jp/"}},
	}})
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithFeeds(svc, perHour, now)))}, &hits
}

func TestFeedRSS(t *testing.T) {
	e, hits := feedsEnv(t, 10)
	r := e.do("u1", http.MethodGet, "/v1/feeds/rss?feedId=digital-news&limit=3", nil, nil)
	if r.code != http.StatusOK {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	items, _ := r.body["items"].([]any)
	src, _ := r.body["source"].(map[string]any)
	if len(items) != 1 || r.body["provider"] != "rss" || r.body["title"] != "デジタル庁 新着・更新" || src["license"] == "" {
		t.Fatalf("body %s", r.raw)
	}
	if cc := r.hdr.Get("Cache-Control"); cc != "private, max-age=300" {
		t.Errorf("Cache-Control = %q", cc)
	}
	e.do("u2", http.MethodGet, "/v1/feeds/rss?feedId=digital-news", nil, nil)
	if hits.Load() != 1 {
		t.Errorf("source hit %d times, want 1 (cached)", hits.Load())
	}
}

func TestFeedRejects(t *testing.T) {
	e, hits := feedsEnv(t, 100)
	for _, c := range []struct {
		path string
		code int
	}{
		{"/v1/feeds/http", http.StatusNotFound},
		{"/v1/feeds/rss?feedId=nope", http.StatusBadRequest},
		{"/v1/feeds/rss?url=https://evil.example/feed", http.StatusBadRequest},
		{"/v1/feeds/rss?feedId=digital-news&feedId=fdma-news", http.StatusBadRequest},
		{"/v1/feeds/weather?region=999999", http.StatusBadRequest},
		{"/v1/feeds/weather?region=130000&lat=35.6&lng=139.7", http.StatusBadRequest},
		{"/v1/feeds/holidays?limit=0", http.StatusBadRequest},
		{"/v1/feeds/holidays?limit=21", http.StatusBadRequest},
		{"/v1/feeds/holidays?region=130000", http.StatusBadRequest},
	} {
		r := e.do("u1", http.MethodGet, c.path, nil, nil)
		if r.code != c.code {
			t.Errorf("%s: %d %s, want %d", c.path, r.code, r.raw, c.code)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("rejected requests reached the source %d times", hits.Load())
	}
	if r := e.do("", http.MethodGet, "/v1/feeds/holidays", nil, nil); r.code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", r.code)
	}
}

func TestFeedUserLimitAndKillSwitch(t *testing.T) {
	e, _ := feedsEnv(t, 2, "rss")
	for i := range 3 {
		r := e.do("u1", http.MethodGet, "/v1/feeds/rss?feedId=digital-news", nil, nil)
		switch {
		case i < 2 && (r.code != http.StatusServiceUnavailable || !strings.Contains(string(r.raw), "upstream_unavailable")):
			t.Errorf("disabled rss #%d: %d %s", i, r.code, r.raw)
		case i == 2 && (r.code != http.StatusTooManyRequests || r.hdr.Get("Retry-After") == ""):
			t.Errorf("over the cap: %d %s", r.code, r.raw)
		}
	}
	if r := e.do("u2", http.MethodGet, "/v1/feeds/rss?feedId=digital-news", nil, nil); r.code == http.StatusTooManyRequests {
		t.Error("the cap is per user")
	}
}
