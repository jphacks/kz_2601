package appconfig

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/books"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// A fake openBD (httptest) serving the recorded fixture; no test reaches the real one.
func booksEnv(t *testing.T, perHour int, disabled bool) (*env, *atomic.Int32) {
	t.Helper()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	rec, err := os.ReadFile("../books/testdata/openbd_9784780802047.json")
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("isbn") == "9784780802047" {
			_, _ = w.Write(rec)
			return
		}
		_, _ = w.Write([]byte("[null]"))
	}))
	t.Cleanup(srv.Close)
	now := func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
	svc := books.New(books.Options{Client: srv.Client(), Now: now, Disabled: disabled,
		Endpoint: &books.Endpoint{GetURL: srv.URL + "/v1/get", CoverHost: "cover.openbd.jp"}})
	return &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory(), WithBooks(svc, perHour, now)))}, &hits
}

func TestBookLookup(t *testing.T) {
	e, hits := booksEnv(t, 10, false)
	r := e.do("u1", http.MethodGet, "/v1/lookup/book?isbn=978-4-7808-0204-7", nil, nil)
	if r.code != http.StatusOK {
		t.Fatalf("%d %s", r.code, r.raw)
	}
	src, _ := r.body["source"].(map[string]any)
	if r.body["isbn"] != "9784780802047" || r.body["title"] != "おにぎりレシピ101" || r.body["publisher"] != "ポット出版" ||
		r.body["publishedDate"] != "2014-04-08" || r.body["authors"] != "山田玲子／著 水野菜生／英訳" ||
		r.body["coverUrl"] != "https://cover.openbd.jp/9784780802047.jpg" || src["name"] != "openBD" {
		t.Fatalf("body %s", r.raw)
	}
	if cc := r.hdr.Get("Cache-Control"); cc != "private, max-age=3600" {
		t.Errorf("Cache-Control = %q", cc)
	}
	e.do("u2", http.MethodGet, "/v1/lookup/book?isbn=4780802040", nil, nil) // same book as ISBN-10
	if hits.Load() != 1 {
		t.Errorf("source hit %d times, want 1 (cached)", hits.Load())
	}
	if r := e.do("u1", http.MethodGet, "/v1/lookup/book?isbn=9780000000002", nil, nil); r.code != http.StatusNotFound {
		t.Errorf("unknown ISBN: %d %s", r.code, r.raw)
	}
}

func TestBookLookupRejects(t *testing.T) {
	e, hits := booksEnv(t, 100, false)
	for _, path := range []string{
		"/v1/lookup/book",
		"/v1/lookup/book?isbn=",
		"/v1/lookup/book?isbn=9784780802048",
		"/v1/lookup/book?isbn=4900000000004",
		"/v1/lookup/book?isbn=9784780802047&url=https://evil.example/",
		"/v1/lookup/book?isbn=9784780802047&isbn=9784101010014",
		"/v1/lookup/book?isbn=https%3A%2F%2Fevil.example%2F",
	} {
		if r := e.do("u1", http.MethodGet, path, nil, nil); r.code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", path, r.code, r.raw)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("rejected requests reached the source %d times", hits.Load())
	}
	if r := e.do("", http.MethodGet, "/v1/lookup/book?isbn=9784780802047", nil, nil); r.code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", r.code)
	}
}

func TestBookLookupUserCapAndKillSwitch(t *testing.T) {
	e, _ := booksEnv(t, 2, false)
	for range 2 {
		if r := e.do("u1", http.MethodGet, "/v1/lookup/book?isbn=9784780802047", nil, nil); r.code != http.StatusOK {
			t.Fatalf("%d", r.code)
		}
	}
	r := e.do("u1", http.MethodGet, "/v1/lookup/book?isbn=9784780802047", nil, nil)
	if r.code != http.StatusTooManyRequests || r.hdr.Get("Retry-After") == "" {
		t.Errorf("over the cap: %d %v", r.code, r.hdr)
	}
	if r := e.do("u2", http.MethodGet, "/v1/lookup/book?isbn=9784780802047", nil, nil); r.code != http.StatusOK {
		t.Errorf("another user: %d", r.code)
	}

	off, hits := booksEnv(t, 10, true)
	r = off.do("u1", http.MethodGet, "/v1/lookup/book?isbn=9784780802047", nil, nil)
	if r.code != http.StatusServiceUnavailable || hits.Load() != 0 {
		t.Errorf("disabled: %d %s (hits %d)", r.code, r.raw, hits.Load())
	}
	// off: 503 before the ISBN check, so a saved button always gets "not available"
	if r := off.do("u1", http.MethodGet, "/v1/lookup/book?isbn=123", nil, nil); r.code != http.StatusServiceUnavailable {
		t.Errorf("disabled, bad ISBN: %d %s", r.code, r.raw)
	}
}

// Off by default (openBD's terms, 2026-11-08): without BOOK_LOOKUP_ENABLED
// the production wiring answers 503 reason=disabled and never fetches.
func TestBookLookupOffByDefault(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	t.Setenv("BOOK_LOOKUP_ENABLED", "")
	t.Setenv("BOOK_LOOKUP_DISABLED", "")
	t.Setenv("FEEDS_DISABLED", "")
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(store.NewMemory()))}
	r := e.do("u1", http.MethodGet, "/v1/lookup/book?isbn=9784780802047", nil, nil)
	errBody, _ := r.body["error"].(map[string]any)
	details, _ := errBody["details"].(map[string]any)
	if r.code != http.StatusServiceUnavailable || details["reason"] != "disabled" || errBody["message"] != "本の情報の取得は今使えません" {
		t.Fatalf("default: %d %s", r.code, r.raw)
	}
}
