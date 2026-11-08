package books

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testdata/openbd_*.json are openBD v1 answers recorded 2026-11-08 (onix /
// hanmoto emptied; only summary is read). No test reaches the real source.

func TestNormalizeISBN(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"9784780802047", "9784780802047", true},
		{"978-4-7808-0204-7", "9784780802047", true},
		{" 978 4780802047 ", "9784780802047", true},
		{"4780802040", "9784780802047", true}, // ISBN-10 of the same book
		{"4-7808-0204-0", "9784780802047", true},
		{"080442957X", "9780804429573", true}, // X check digit
		{"080442957x", "9780804429573", true},
		{"9791032305690", "9791032305690", true}, // 979 prefix
		{"9784780802048", "", false},             // bad check digit
		{"4780802041", "", false},
		{"4900000000004", "", false}, // valid EAN-13, not a book
		{"X804429570", "", false},    // X not last
		{"978478080204X", "", false},
		{"97847808020", "", false},
		{"", "", false},
		{"978478080204７", "", false}, // full-width digit
		{"https://evil.example/9784780802047", "", false},
		{"9784780802047&x=1", "", false},
		{strings.Repeat("-", 41), "", false},
	} {
		got, ok := NormalizeISBN(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizeISBN(%q) = %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

type fakeSource struct {
	srv  *httptest.Server
	hits atomic.Int32
	mu   sync.Mutex
	last string // last query
}

func newSource(t *testing.T, handler func(w http.ResponseWriter, isbn string)) *fakeSource {
	t.Helper()
	f := &fakeSource{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.mu.Lock()
		f.last = r.URL.RawQuery
		f.mu.Unlock()
		if r.URL.Path != "/v1/get" {
			http.NotFound(w, r)
			return
		}
		handler(w, r.URL.Query().Get("isbn"))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// recorded serves the fixtures by ISBN; anything else is openBD's [null].
func recorded(t *testing.T) func(http.ResponseWriter, string) {
	files := map[string][]byte{
		"9784780802047": fixture(t, "openbd_9784780802047.json"),
		"9784101010014": fixture(t, "openbd_9784101010014.json"),
	}
	miss := fixture(t, "openbd_notfound.json")
	return func(w http.ResponseWriter, isbn string) {
		w.Header().Set("Content-Type", "application/json")
		if b, ok := files[isbn]; ok {
			_, _ = w.Write(b)
			return
		}
		_, _ = w.Write(miss)
	}
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newService(f *fakeSource, c *clock, o Options) *Service {
	o.Client = f.srv.Client()
	o.Endpoint = &Endpoint{GetURL: f.srv.URL + "/v1/get", CoverHost: "cover.openbd.jp"}
	o.Now = c.now
	return New(o)
}

func TestLookupRecorded(t *testing.T) {
	f := newSource(t, recorded(t))
	c := &clock{t: time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)}
	s := newService(f, c, Options{})
	b, err := s.Lookup(context.Background(), "4-7808-0204-0")
	if err != nil {
		t.Fatal(err)
	}
	want := Book{ISBN: "9784780802047", Title: "おにぎりレシピ101", Authors: "山田玲子／著 水野菜生／英訳", Publisher: "ポット出版",
		PublishedDate: "2014-04-08", CoverURL: "https://cover.openbd.jp/9784780802047.jpg", Source: openBD,
		FetchedAt: c.now()}
	if *b != want {
		t.Fatalf("got %+v\nwant %+v", *b, want)
	}
	if f.last != "isbn=9784780802047" {
		t.Errorf("query = %q, want only the normalized ISBN-13", f.last)
	}
	// NDL-derived record: YYYYMM date, no cover, author as written by the source.
	b, err = s.Lookup(context.Background(), "9784101010014")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "吾輩は猫である" || b.PublishedDate != "2003-06" || b.CoverURL != "" || b.Authors != "夏目,漱石,1867-1916" || b.Publisher != "新潮社" {
		t.Errorf("got %+v", *b)
	}
}

func TestLookupRejectsBeforeFetch(t *testing.T) {
	f := newSource(t, recorded(t))
	s := newService(f, &clock{t: time.Now()}, Options{})
	for _, in := range []string{"", "123", "9784780802048", "../../etc", "9784780802047?x"} {
		if _, err := s.Lookup(context.Background(), in); !errors.Is(err, ErrInvalidISBN) {
			t.Errorf("%q: %v", in, err)
		}
	}
	if f.hits.Load() != 0 {
		t.Errorf("invalid ISBNs reached the source %d times", f.hits.Load())
	}
}

func TestLookupNotFoundAndCache(t *testing.T) {
	f := newSource(t, recorded(t))
	c := &clock{t: time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC)}
	s := newService(f, c, Options{})
	ctx := context.Background()
	if _, err := s.Lookup(ctx, "9780000000002"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	for range 3 {
		if _, err := s.Lookup(ctx, "978-4-7808-0204-7"); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = s.Lookup(ctx, "9780000000002")
	if n := f.hits.Load(); n != 2 {
		t.Fatalf("source hit %d times, want 2 (found and miss are cached)", n)
	}
	c.add(missTTL + time.Minute) // miss expires, found does not
	_, _ = s.Lookup(ctx, "9780000000002")
	_, _ = s.Lookup(ctx, "9784780802047")
	if n := f.hits.Load(); n != 3 {
		t.Fatalf("after missTTL: %d hits, want 3", n)
	}
	c.add(foundTTL)
	_, _ = s.Lookup(ctx, "9784780802047")
	if n := f.hits.Load(); n != 4 {
		t.Fatalf("after a day: %d hits, want 4", n)
	}
}

func TestLookupFailures(t *testing.T) {
	for name, h := range map[string]func(http.ResponseWriter, string){
		"500":      func(w http.ResponseWriter, _ string) { w.WriteHeader(http.StatusInternalServerError) },
		"not json": func(w http.ResponseWriter, _ string) { _, _ = w.Write([]byte("<html>maintenance</html>")) },
		"too large": func(w http.ResponseWriter, _ string) {
			_, _ = w.Write([]byte("[" + strings.Repeat(" ", maxBody) + "null]"))
		},
		"other isbn": func(w http.ResponseWriter, _ string) {
			_, _ = w.Write([]byte(`[{"summary":{"isbn":"9784101010014","title":"x"}}]`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSource(t, h)
			c := &clock{t: time.Now()}
			s := newService(f, c, Options{})
			if _, err := s.Lookup(context.Background(), "9784780802047"); !errors.Is(err, ErrUpstream) {
				t.Fatalf("got %v, want ErrUpstream", err)
			}
			// The failure is remembered for a minute.
			_, _ = s.Lookup(context.Background(), "9784780802047")
			if f.hits.Load() != 1 {
				t.Errorf("retried %d times within failTTL", f.hits.Load())
			}
			c.add(failTTL + time.Second)
			_, _ = s.Lookup(context.Background(), "9784780802047")
			if f.hits.Load() != 2 {
				t.Errorf("not retried after failTTL: %d", f.hits.Load())
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	f := newSource(t, func(w http.ResponseWriter, _ string) {
		_, _ = w.Write([]byte(`[{"summary":{"isbn":"9784780802047","title":"本\u0000の\u200b名前\n 上","author":"` + strings.Repeat("著", 400) +
			`","publisher":"出版社","pubdate":"2014年4月","cover":"http://cover.openbd.jp/x.jpg"}}]`))
	})
	s := newService(f, &clock{t: time.Now()}, Options{})
	b, err := s.Lookup(context.Background(), "9784780802047")
	if err != nil {
		t.Fatal(err)
	}
	if b.Title != "本の名前 上" || len([]rune(b.Authors)) != maxAuthors || b.PublishedDate != "" || b.CoverURL != "" {
		t.Errorf("got %+v", *b)
	}
	for _, c := range []struct{ raw, want string }{
		{"20140408", "2014-04-08"}, {"201404", "2014-04"}, {"2014", "2014"}, {"20141399", ""}, {"0000", ""}, {"2014-04", ""},
	} {
		if got := pubDate(c.raw); got != c.want {
			t.Errorf("pubDate(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
	if got := s.coverURL("https://evil.example/x.jpg"); got != "" {
		t.Errorf("cover on another host kept: %q", got)
	}
}

func TestKillSwitchAndBudget(t *testing.T) {
	f := newSource(t, recorded(t))
	c := &clock{t: time.Now()}
	off := newService(f, c, OptionsFromEnv(func(k string) string { return map[string]string{"BOOK_LOOKUP_DISABLED": "true"}[k] }))
	if _, err := off.Lookup(context.Background(), "9784780802047"); !errors.Is(err, ErrDisabled) || off.Enabled() {
		t.Fatalf("disabled: %v", err)
	}
	if !OptionsFromEnv(func(k string) string { return map[string]string{"FEEDS_DISABLED": "rss, all"}[k] }).Disabled {
		t.Error("FEEDS_DISABLED=all should switch the lookup off too")
	}
	if OptionsFromEnv(func(k string) string { return map[string]string{"FEEDS_DISABLED": "rss"}[k] }).Disabled {
		t.Error("FEEDS_DISABLED=rss must not switch the lookup off")
	}
	if f.hits.Load() != 0 {
		t.Fatal("disabled service reached the source")
	}
	s := newService(f, c, Options{PerMinute: 2})
	for _, isbn := range []string{"9784780802047", "9784101010014"} {
		if _, err := s.Lookup(context.Background(), isbn); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Lookup(context.Background(), "9780000000002"); !errors.Is(err, ErrBusy) {
		t.Fatalf("over budget: %v", err)
	}
	if _, err := s.Lookup(context.Background(), "9784780802047"); err != nil {
		t.Fatalf("cached answer should not need budget: %v", err)
	}
	c.add(time.Minute)
	if _, err := s.Lookup(context.Background(), "9780000000002"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("next minute: %v", err)
	}
}

func TestConcurrentLookupsFetchOnce(t *testing.T) {
	release := make(chan struct{})
	f := newSource(t, func(w http.ResponseWriter, isbn string) {
		<-release
		recorded(t)(w, isbn)
	})
	s := newService(f, &clock{t: time.Now()}, Options{})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Lookup(context.Background(), "9784780802047")
			errs <- err
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f.hits.Load() != 1 {
		t.Errorf("source hit %d times, want 1", f.hits.Load())
	}
}

// The production client (feeds.NewClient) never connects to a loopback
// address, and redirects may not leave the source's host.
func TestProductionClientAndRedirects(t *testing.T) {
	f := newSource(t, recorded(t))
	s := New(Options{Endpoint: &Endpoint{GetURL: f.srv.URL + "/v1/get"}})
	if _, err := s.Lookup(context.Background(), "9784780802047"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("loopback: %v", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("production client reached a loopback server")
	}
	other := newSource(t, recorded(t))
	redir := newSource(t, func(w http.ResponseWriter, isbn string) {
		w.Header().Set("Location", other.srv.URL+"/v1/get?isbn="+isbn)
		w.WriteHeader(http.StatusFound)
	})
	rs := newService(redir, &clock{t: time.Now()}, Options{})
	if _, err := rs.Lookup(context.Background(), "9784780802047"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("redirect to another host: %v", err)
	}
	if other.hits.Load() != 0 {
		t.Error("followed a redirect to another host")
	}
}

// A caller that disconnects mid-fetch neither fails the waiters nor caches
// a failure: the fetch finishes and the book is cached for everyone.
func TestCanceledCallerDoesNotPoisonCache(t *testing.T) {
	release := make(chan struct{})
	f := newSource(t, func(w http.ResponseWriter, isbn string) {
		<-release
		recorded(t)(w, isbn)
	})
	s := newService(f, &clock{t: time.Now()}, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := s.Lookup(ctx, "9784780802047")
		first <- err
	}()
	for f.hits.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	waiter := make(chan error, 1)
	go func() {
		_, err := s.Lookup(context.Background(), "9784780802047")
		waiter <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first caller: %v", err)
	}
	if err := <-waiter; err != nil {
		t.Fatalf("waiter: %v", err)
	}
	if b, err := s.Lookup(context.Background(), "9784780802047"); err != nil || b == nil {
		t.Fatalf("after: %v %v", b, err)
	}
	if f.hits.Load() != 1 {
		t.Errorf("source hit %d times, want 1", f.hits.Load())
	}
}
