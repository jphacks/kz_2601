package appconfig

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/books"
	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// clientHeaders are what each client sends (desktop: desktop/.../net/ClientPlatform.kt).
var clientHeaders = map[string]map[string]string{
	"ios": {httpx.ClientPlatformHeader: "ios", httpx.ClientVersionHeader: "1.4.2", httpx.ClientBuildHeader: "42",
		"User-Agent": "TailorIt/42 CFNetwork/1568 Darwin/25.0.0"},
	"windows": {httpx.ClientPlatformHeader: "windows", httpx.ClientVersionHeader: "0.1.0", httpx.ClientBuildHeader: "1",
		"User-Agent": "TailorIt-Desktop/0.1.0 (Windows 11 10.0; amd64)"},
	"linux": {httpx.ClientPlatformHeader: "linux", httpx.ClientVersionHeader: "0.1.0", httpx.ClientBuildHeader: "1",
		"User-Agent": "TailorIt-Desktop/0.1.0 (Linux 6.8.0; amd64)"},
}

// desktopEnv serves the main endpoints with fake upstreams (no network).
func desktopEnv(t *testing.T) *env {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	rec, err := os.ReadFile("../books/testdata/openbd_9784780802047.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rss":
			_, _ = w.Write([]byte(`<rss version="2.0"><channel><item><title>お知らせ</title><link>https://www.example.go.jp/a</link><pubDate>Thu, 05 Nov 2026 18:30:00 +0900</pubDate></item></channel></rss>`))
		case "/v1/get":
			_, _ = w.Write(rec)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	now := func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
	fs := feeds.New(feeds.Options{Client: srv.Client(), Now: now, Endpoints: &feeds.Endpoints{
		WeatherBase: srv.URL + "/w/", Holidays: srv.URL + "/h.csv",
		RSS: map[string]feeds.RSSFeed{"digital-news": {Name: "デジタル庁 新着・更新", URL: srv.URL + "/rss", Publisher: "デジタル庁ホームページ", SiteURL: "https://www.digital.go.jp/"}},
	}})
	bs := books.New(books.Options{Client: srv.Client(), Now: now,
		Endpoint: &books.Endpoint{GetURL: srv.URL + "/v1/get", CoverHost: "cover.openbd.jp"}})
	mux := New(store.NewMemory(), WithFeeds(fs, 1000, now), WithBooks(bs, 1000, now))
	mux.Handle("GET /v1/client-config", ClientConfig(ClientConfigFromEnv(func(k string) string {
		return map[string]string{EnvMinIOSBuild: "10", EnvLatestIOSBuild: "40", EnvMinMacOSBuild: "3"}[k]
	})))
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(mux)}
}

// Desktop (windows / linux) gets the same answers as iOS from the main
// endpoints (#2300): appconfig does not branch on X-Client-Platform or the
// User-Agent, so an OS it does not know is neither refused nor given
// something else.
func TestDesktopPlatformsGetSameAnswersAsIOS(t *testing.T) {
	e := desktopEnv(t)
	// each platform reads one app it owns (same uid, so the bodies match)
	paths := []string{
		"/v1/apps/DESKTOP-APP-1/dsl", // auto-created on the first read
		"/v1/apps",
		"/v1/apps/DESKTOP-APP-1/dsl",
		"/v1/feeds/rss?feedId=digital-news&limit=3",
		"/v1/lookup/book?isbn=9784780802047",
		"/v1/client-config",
	}
	for _, p := range paths {
		want := e.do("u-desk", http.MethodGet, p, nil, clientHeaders["ios"])
		if want.code != http.StatusOK {
			t.Fatalf("ios %s: %d %s", p, want.code, want.raw)
		}
		for _, plat := range []string{"windows", "linux"} {
			got := e.do("u-desk", http.MethodGet, p, nil, clientHeaders[plat])
			if got.code != want.code || !bytes.Equal(got.raw, want.raw) {
				t.Errorf("%s %s: %d %s\nios: %d %s", plat, p, got.code, got.raw, want.code, want.raw)
			}
			for _, h := range []string{"Cache-Control", "ETag", "Content-Type"} {
				if got.hdr.Get(h) != want.hdr.Get(h) {
					t.Errorf("%s %s: %s %q, ios %q", plat, p, h, got.hdr.Get(h), want.hdr.Get(h))
				}
			}
		}
	}
	// conditional GET of the DSL works the same (desktop caches by ETag too)
	for plat, hdr := range clientHeaders {
		h := map[string]string{"If-None-Match": `"1"`}
		for k, v := range hdr {
			h[k] = v
		}
		if r := e.do("u-desk", http.MethodGet, "/v1/apps/DESKTOP-APP-1/dsl", nil, h); r.code != http.StatusNotModified {
			t.Errorf("%s If-None-Match: %d", plat, r.code)
		}
	}
}

// client-config has no windows / linux fields: desktop gets the same body
// (the iOS / macOS numbers are for those clients; desktop reads none of
// them), so no minimum build is enforced on desktop yet.
func TestClientConfigDesktopNoEnforcement(t *testing.T) {
	e := desktopEnv(t)
	for _, plat := range []string{"windows", "linux"} {
		r := e.do("u-desk", http.MethodGet, "/v1/client-config", nil, clientHeaders[plat]) // public in production; the test mux sits behind auth
		if r.code != http.StatusOK || len(r.body) != 3 || r.body["minIOSBuild"] != float64(10) || r.body["minMacOSBuild"] != float64(3) {
			t.Fatalf("%s: %d %s", plat, r.code, r.raw)
		}
		for k := range r.body {
			if k != "minIOSBuild" && k != "latestIOSBuild" && k != "minMacOSBuild" {
				t.Fatalf("%s: unexpected field %q", plat, k)
			}
		}
	}
}
