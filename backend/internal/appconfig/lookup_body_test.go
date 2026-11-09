package appconfig

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/feeds"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// POST variants: the user-entered value travels in the body, the checks and
// the reply are the GET ones.

func TestBookLookupPost(t *testing.T) {
	e, hits := booksEnv(t, 10, false)
	r := e.do("u1", http.MethodPost, "/v1/lookup/book", `{"isbn":"978-4-7808-0204-7"}`, nil)
	if r.code != http.StatusOK || r.body["isbn"] != "9784780802047" || r.body["title"] != "おにぎりレシピ101" {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	if cc := r.hdr.Get("Cache-Control"); cc != "private, max-age=3600" {
		t.Errorf("Cache-Control = %q", cc)
	}
	// GET still answers (older iOS builds), from the same cache
	if g := e.do("u2", http.MethodGet, "/v1/lookup/book?isbn=4780802040", nil, nil); g.code != http.StatusOK || g.body["title"] != "おにぎりレシピ101" {
		t.Fatalf("GET: %d %s", g.code, g.raw)
	}
	if hits.Load() != 1 {
		t.Errorf("source hit %d times, want 1 (cached)", hits.Load())
	}
	if r := e.do("u1", http.MethodPost, "/v1/lookup/book", `{"isbn":"9780000000002"}`, nil); r.code != http.StatusNotFound {
		t.Errorf("unknown ISBN: %d %s", r.code, r.raw)
	}
}

func TestBookLookupPostRejects(t *testing.T) {
	e, hits := booksEnv(t, 100, false)
	for _, c := range []struct {
		path, body string
		code       int
	}{
		{"/v1/lookup/book", ``, http.StatusBadRequest},
		{"/v1/lookup/book", `{}`, http.StatusBadRequest},
		{"/v1/lookup/book", `{"isbn":""}`, http.StatusBadRequest},
		{"/v1/lookup/book", `{"isbn":"9784780802048"}`, http.StatusBadRequest},
		{"/v1/lookup/book", `{"isbn":9784780802047}`, http.StatusBadRequest},
		{"/v1/lookup/book", `{"isbn":"9784780802047","url":"https://evil.example/"}`, http.StatusBadRequest},
		{"/v1/lookup/book", `[]`, http.StatusBadRequest},
		{"/v1/lookup/book?isbn=9784780802047", `{"isbn":"9784780802047"}`, http.StatusBadRequest},
		{"/v1/lookup/book", `{"isbn":"` + strings.Repeat("9", lookupBodyMaxBytes) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		if r := e.do("u1", http.MethodPost, c.path, c.body, nil); r.code != c.code {
			t.Errorf("%s %.60s: %d %s, want %d", c.path, c.body, r.code, r.raw, c.code)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("rejected requests reached the source %d times", hits.Load())
	}
	if r := e.do("", http.MethodPost, "/v1/lookup/book", `{"isbn":"9784780802047"}`, nil); r.code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", r.code)
	}
}

// One hourly count for both methods: switching to the other one does not reset it.
func TestBookLookupCapSharedWithGet(t *testing.T) {
	e, _ := booksEnv(t, 2, false)
	if r := e.do("u1", http.MethodGet, "/v1/lookup/book?isbn=9784780802047", nil, nil); r.code != http.StatusOK {
		t.Fatalf("GET: %d", r.code)
	}
	if r := e.do("u1", http.MethodPost, "/v1/lookup/book", `{"isbn":"9784780802047"}`, nil); r.code != http.StatusOK {
		t.Fatalf("POST: %d", r.code)
	}
	for _, m := range []string{http.MethodGet, http.MethodPost} {
		path, body := "/v1/lookup/book?isbn=9784780802047", any(nil)
		if m == http.MethodPost {
			path, body = "/v1/lookup/book", `{"isbn":"9784780802047"}`
		}
		if r := e.do("u1", m, path, body, nil); r.code != http.StatusTooManyRequests || r.hdr.Get("Retry-After") == "" {
			t.Errorf("%s over the cap: %d %s", m, r.code, r.raw)
		}
	}
	off, _ := booksEnv(t, 10, true)
	if r := off.do("u1", http.MethodPost, "/v1/lookup/book", `{"isbn":"123"}`, nil); r.code != http.StatusServiceUnavailable {
		t.Errorf("disabled: %d %s", r.code, r.raw)
	}
}

func TestFeedPost(t *testing.T) {
	e, hits := feedsEnv(t, 100)
	r := e.do("u1", http.MethodPost, "/v1/feeds/rss", `{"feedId":"digital-news","limit":3}`, nil)
	if r.code != http.StatusOK || r.body["provider"] != "rss" || r.body["title"] != "デジタル庁 新着・更新" {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	if cc := r.hdr.Get("Cache-Control"); cc != "private, max-age=300" {
		t.Errorf("Cache-Control = %q", cc)
	}
	for _, c := range []struct {
		path, body string
		code       int
	}{
		{"/v1/feeds/http", `{}`, http.StatusNotFound},
		{"/v1/feeds/rss", `{"feedId":"nope"}`, http.StatusBadRequest},
		{"/v1/feeds/rss", `{"feedId":"digital-news","url":"https://evil.example/feed"}`, http.StatusBadRequest},
		{"/v1/feeds/rss", `{"feedId":"digital-news","q":"a"}`, http.StatusBadRequest},
		{"/v1/feeds/holidays", `{"limit":0}`, http.StatusBadRequest},
		{"/v1/feeds/holidays", `{"limit":21}`, http.StatusBadRequest},
		{"/v1/feeds/holidays", `{"limit":"3"}`, http.StatusBadRequest},
		{"/v1/feeds/rss?feedId=digital-news", `{"feedId":"digital-news"}`, http.StatusBadRequest},
		{"/v1/feeds/rss", `{"feedId":"` + strings.Repeat("a", lookupBodyMaxBytes) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		if r := e.do("u1", http.MethodPost, c.path, c.body, nil); r.code != c.code {
			t.Errorf("%s %.60s: %d %s, want %d", c.path, c.body, r.code, r.raw, c.code)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("source hit %d times, want 1", hits.Load())
	}
}

// One hourly count for both methods.
func TestFeedCapSharedWithGet(t *testing.T) {
	e, _ := feedsEnv(t, 2)
	if r := e.do("u1", http.MethodGet, "/v1/feeds/rss?feedId=digital-news", nil, nil); r.code != http.StatusOK {
		t.Fatalf("GET: %d %s", r.code, r.raw)
	}
	if r := e.do("u1", http.MethodPost, "/v1/feeds/rss", `{"feedId":"digital-news"}`, nil); r.code != http.StatusOK {
		t.Fatalf("POST: %d %s", r.code, r.raw)
	}
	if r := e.do("u1", http.MethodGet, "/v1/feeds/rss?feedId=digital-news", nil, nil); r.code != http.StatusTooManyRequests {
		t.Errorf("GET over the cap: %d", r.code)
	}
	if r := e.do("u1", http.MethodPost, "/v1/feeds/rss", `{"feedId":"digital-news"}`, nil); r.code != http.StatusTooManyRequests || r.hdr.Get("Retry-After") == "" {
		t.Errorf("POST over the cap: %d %v", r.code, r.hdr)
	}
}

// The diet search term in the body reaches the source like the GET q does.
func TestFeedDietPost(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	var mu sync.Mutex
	var got []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.URL.Query())
		mu.Unlock()
		_, _ = w.Write([]byte(`{"numberOfRecords":1,"speechRecord":[{"speechOrder":2,"speaker":"山田太郎","nameOfHouse":"衆議院","nameOfMeeting":"予算委員会","issue":"第1号","date":"2026-11-05","speechURL":"https://kokkai.ndl.go.jp/txt/x/2"}]}`))
	}))
	t.Cleanup(srv.Close)
	now := func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) }
	svc := feeds.New(feeds.Options{Client: srv.Client(), Now: now, Endpoints: &feeds.Endpoints{Diet: srv.URL + "/speech"}})
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithFeeds(svc, 100, now)))}

	for _, body := range []string{
		`{}`,
		`{"q":""}`,
		`{"q":"` + strings.Repeat("あ", 41) + `"}`,
		`{"q":"予\n算"}`,
		`{"q":["a","b"]}`,
		`{"q":"a","query":"b"}`,
	} {
		if r := e.do("u1", http.MethodPost, "/v1/feeds/diet", body, nil); r.code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", body, r.code, r.raw)
		}
	}
	if len(got) != 0 {
		t.Fatalf("rejected requests reached the source: %v", got)
	}
	r := e.do("u1", http.MethodPost, "/v1/feeds/diet", `{"q":"予算 委員会","limit":3}`, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 1 || r.body["title"] != "国会会議録「予算 委員会」" {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	if len(got) != 1 || got[0].Get("any") != "予算 委員会" {
		t.Errorf("sent %v", got)
	}
	// the GET form still answers, from the same cache
	if r := e.do("u2", http.MethodGet, "/v1/feeds/diet?q="+url.QueryEscape("予算 委員会")+"&limit=3", nil, nil); r.code != http.StatusOK || len(got) != 1 {
		t.Errorf("GET: %d %s (sent %d)", r.code, r.raw, len(got))
	}
}

func TestPublicAppSearchPost(t *testing.T) {
	e := newEnv(t)
	e.ownApp("owner", "/v1/apps/OWNER-KAKEIBO", kakeibo(t))
	e.do("owner", http.MethodPost, "/v1/apps/OWNER-KAKEIBO/publish", `{"tags":["節約"]}`, nil)
	e.ownApp("owner2", "/v1/apps/OTHER-WALK", walk(t))
	e.do("owner2", http.MethodPost, "/v1/apps/OTHER-WALK/publish", `{"tags":["健康"]}`, nil)

	search := func(body string) []any {
		t.Helper()
		r := e.do("finder", http.MethodPost, "/v1/public-apps/search", body, nil)
		if r.code != http.StatusOK {
			t.Fatalf("search %s: %d %s", body, r.code, r.raw)
		}
		return r.body["items"].([]any)
	}
	if got := search(`{}`); len(got) != 2 {
		t.Fatalf("all: %v", got)
	}
	if got := search(`{"q":"節約","sort":"new","pageSize":50}`); len(got) != 1 || got[0].(map[string]any)["id"] != "OWNER-KAKEIBO" {
		t.Fatalf("tag: %v", got)
	}
	r := e.do("finder", http.MethodPost, "/v1/public-apps/search", `{"pageSize":1}`, nil)
	tok, _ := r.body["nextPageToken"].(string)
	if len(r.body["items"].([]any)) != 1 || tok == "" {
		t.Fatalf("page 1: %s", r.raw)
	}
	if got := search(`{"pageSize":1,"pageToken":"` + tok + `"}`); len(got) != 1 {
		t.Fatalf("page 2: %v", got)
	}
	// GET still answers (older iOS builds)
	if g := e.do("finder", http.MethodGet, "/v1/public-apps?q=%E7%AF%80%E7%B4%84", nil, nil); g.code != http.StatusOK || len(g.body["items"].([]any)) != 1 {
		t.Fatalf("GET: %d %s", g.code, g.raw)
	}
	for _, c := range []struct {
		path, body string
		code       int
	}{
		{"/v1/public-apps/search", `{"sort":"bogus"}`, http.StatusBadRequest},
		{"/v1/public-apps/search", `{"pageSize":0}`, http.StatusBadRequest},
		{"/v1/public-apps/search", `{"pageToken":"!"}`, http.StatusBadRequest},
		{"/v1/public-apps/search", `{"q":"a","tags":["x"]}`, http.StatusBadRequest},
		{"/v1/public-apps/search?q=a", `{}`, http.StatusBadRequest},
		{"/v1/public-apps/search", `{"q":"` + strings.Repeat("a", lookupBodyMaxBytes) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		if r := e.do("finder", http.MethodPost, c.path, c.body, nil); r.code != c.code {
			t.Errorf("%s %.60s: %d %s, want %d", c.path, c.body, r.code, r.raw, c.code)
		}
	}
	if r := e.do("", http.MethodPost, "/v1/public-apps/search", `{}`, nil); r.code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", r.code)
	}
}
