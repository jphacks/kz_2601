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

// q reaches 国会会議録 only as a validated query value; a fake source records it.
func TestFeedDietQuery(t *testing.T) {
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
	svc := feeds.New(feeds.Options{Client: srv.Client(), Now: now, UserMissesPerHour: 1,
		Endpoints: &feeds.Endpoints{Diet: srv.URL + "/speech", Arxiv: srv.URL + "/query"}})
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithFeeds(svc, 100, now)))}

	for _, c := range []struct {
		path string
		code int
	}{
		{"/v1/feeds/diet", http.StatusBadRequest},
		{"/v1/feeds/diet?q=", http.StatusBadRequest},
		{"/v1/feeds/diet?q=" + url.QueryEscape(strings.Repeat("あ", 41)), http.StatusBadRequest},
		{"/v1/feeds/diet?q=%E4%BA%88%0A%E7%AE%97", http.StatusBadRequest}, // newline
		{"/v1/feeds/diet?q=a&q=b", http.StatusBadRequest},
		{"/v1/feeds/diet?q=a&url=https://evil.example/", http.StatusBadRequest},
		{"/v1/feeds/rss?feedId=digital-news&q=a", http.StatusBadRequest},
		{"/v1/feeds/arxiv?feedId=math.QA", http.StatusBadRequest},
		{"/v1/feeds/nhk?feedId=cat0", http.StatusNotFound},
	} {
		if r := e.do("u1", http.MethodGet, c.path, nil, nil); r.code != c.code {
			t.Errorf("%s: %d %s, want %d", c.path, r.code, r.raw, c.code)
		}
	}
	if len(got) != 0 {
		t.Fatalf("rejected requests reached the source: %v", got)
	}
	r := e.do("u1", http.MethodGet, "/v1/feeds/diet?q="+url.QueryEscape("予算 委員会")+"&limit=3", nil, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 1 || r.body["title"] != "国会会議録「予算 委員会」" {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	if len(got) != 1 || got[0].Get("any") != "予算 委員会" {
		t.Errorf("sent %v", got)
	}
	// u1's search words reached the source once this hour (the cap here): a new word is 429
	r = e.do("u1", http.MethodGet, "/v1/feeds/diet?q="+url.QueryEscape("決算"), nil, nil)
	if r.code != http.StatusTooManyRequests || r.hdr.Get("Retry-After") == "" || len(got) != 1 {
		t.Errorf("over the per-user source cap: %d %s (sent %d)", r.code, r.raw, len(got))
	}
}
