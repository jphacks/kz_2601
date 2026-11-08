package feeds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/text/encoding/japanese"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// No test reaches a real source: every endpoint points at an httptest server.

var testNow = time.Date(2026, 11, 6, 12, 0, 0, 0, jst)

const weatherJSON = `[
 {"publishingOffice":"金沢地方気象台","timeSeries":[
   {"timeDefines":["2026-11-06T11:00:00+09:00","2026-11-07T00:00:00+09:00","2026-11-08T00:00:00+09:00"],
    "areas":[{"area":{"name":"加賀","code":"170010"},"weatherCodes":["201","300","101"],"weathers":["くもり　時々　晴れ","雨","晴れ　時々　くもり"]}]}]},
 {"publishingOffice":"金沢地方気象台","timeSeries":[
   {"timeDefines":["2026-11-05T00:00:00+09:00","2026-11-07T00:00:00+09:00","2026-11-08T00:00:00+09:00","2026-11-09T00:00:00+09:00"],
    "areas":[{"area":{"name":"石川県","code":"170000"},"weatherCodes":["100","300","101","400"],"pops":["0","80","20",""]}]},
   {"timeDefines":["2026-11-05T00:00:00+09:00","2026-11-07T00:00:00+09:00","2026-11-08T00:00:00+09:00","2026-11-09T00:00:00+09:00"],
    "areas":[{"area":{"name":"金沢","code":"56227"},"tempsMin":["5","9","7",""],"tempsMax":["14","13","16",""]}]}]}
]`

const rss2 = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>t</title>
<item><title>古い お知らせ</title><link>https://www.example.go.jp/old</link><pubDate>Mon, 02 Nov 2026 10:00:00 +0900</pubDate></item>
<item><title>新しい&#x200B;お知らせ</title><link>https://www.example.go.jp/new</link><pubDate>Thu, 05 Nov 2026 18:30:00 +0900</pubDate></item>
<item><title>危ないリンク</title><link>javascript:alert(1)</link></item>
<item><title></title><link>https://www.example.go.jp/empty</link></item>
</channel></rss>`

const rdf = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel><title>t</title></channel>
<item><title>RDF の記事</title><link>https://www.example.go.jp/rdf</link><dc:date>2026-11-04T09:00:00+09:00</dc:date></item>
</rdf:RDF>`

const atom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>t</title>
<entry><title>Atom の記事</title><link rel="alternate" href="https://www.example.go.jp/atom"/><updated>2026-11-03T08:00:00Z</updated></entry>
</feed>`

func holidaysCSV(t *testing.T) []byte {
	t.Helper()
	b, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte("国民の祝日・休日月日,国民の祝日・休日名称\r\n1955/1/1,元日\r\n2026/11/3,文化の日\r\n2026/11/23,勤労感謝の日\r\n2027/1/1,元日\r\n2027/1/11,成人の日\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fake struct {
	mu       sync.Mutex
	redirect string
	srv      *httptest.Server
	hits     sync.Map // path -> *atomic.Int32
	body     map[string][]byte
	code     map[string]int
}

func newFake(t *testing.T) *fake {
	f := &fake{body: map[string][]byte{
		"/forecast/170000.json": []byte(weatherJSON),
		"/syukujitsu.csv":       holidaysCSV(t),
		"/rss2.xml":             []byte(rss2),
		"/rdf.xml":              []byte(rdf),
		"/atom.xml":             []byte(atom),
		"/warning/016000.json":  fixture(t, "jma_warning_r8_016000.json"),
		"/met/compact":          fixture(t, "met_compact_170000.json"),
	}, code: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := f.hits.LoadOrStore(r.URL.Path, new(atomic.Int32))
		n.(*atomic.Int32).Add(1)
		f.mu.Lock()
		c, b, ok, to := f.code[r.URL.Path], f.body[r.URL.Path], false, f.redirect
		_, ok = f.body[r.URL.Path]
		f.mu.Unlock()
		if to != "" {
			http.Redirect(w, r, to, http.StatusFound)
			return
		}
		if c != 0 {
			w.WriteHeader(c)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) count(path string) int32 {
	if n, ok := f.hits.Load(path); ok {
		return n.(*atomic.Int32).Load()
	}
	return 0
}

func (f *fake) endpoints() *Endpoints {
	return &Endpoints{
		WeatherBase: f.srv.URL + "/forecast/",
		WarningBase: f.srv.URL + "/warning/",
		Pressure:    f.srv.URL + "/met/compact",
		Holidays:    f.srv.URL + "/syukujitsu.csv",
		RSS: map[string]RSSFeed{
			"digital-news":  {Name: "RSS2", URL: f.srv.URL + "/rss2.xml", Publisher: "A", SiteURL: "https://a.example/"},
			"fdma-news":     {Name: "RDF", URL: f.srv.URL + "/rdf.xml", Publisher: "B", SiteURL: "https://b.example/"},
			"fdma-disaster": {Name: "Atom", URL: f.srv.URL + "/atom.xml", Publisher: "C", SiteURL: "https://c.example/"},
		},
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

func newSvc(f *fake, c *clock, o Options) *Service {
	o.Client, o.Endpoints, o.Now = f.srv.Client(), f.endpoints(), c.now
	return New(o)
}

func TestWeather(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedWeather, Region: "170000"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "石川県の天気" || res.Source.Name != "気象庁ホームページ" || res.Source.License == "" {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	// 11-05 is before today and dropped; 11-06 comes from the short-term report only.
	var dates []string
	for _, it := range res.Items {
		dates = append(dates, it.Date)
	}
	if want := []string{"2026-11-06", "2026-11-07", "2026-11-08", "2026-11-09"}; !slices.Equal(dates, want) {
		t.Fatalf("dates = %v, want %v", dates, want)
	}
	d7 := res.Items[1]
	if d7.Title != "雨" || d7.Icon != "cloud.rain" || d7.Subtitle != "雨" || *d7.Pop != 80 || *d7.Min != 9 || *d7.Max != 13 {
		t.Errorf("11-07 = %+v", d7)
	}
	if res.Items[0].Subtitle != "くもり 時々 晴れ" || res.Items[0].Pop != nil {
		t.Errorf("11-06 = %+v", res.Items[0])
	}
	if d9 := res.Items[3]; d9.Title != "雪" || d9.Pop != nil || d9.Min != nil || d9.Max != nil {
		t.Errorf("11-09 blanks should be omitted: %+v", d9)
	}
	if res, _ := s.Get(context.Background(), Query{Provider: dsl.FeedWeather, Region: "170000"}, 2); len(res.Items) != 2 {
		t.Errorf("limit 2 gave %d", len(res.Items))
	}
}

func TestHolidays(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedHolidays}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range res.Items {
		got = append(got, it.Date+" "+it.Title+" "+it.Subtitle)
	}
	want := []string{"2026-11-23 勤労感謝の日 月曜日", "2027-01-01 元日 金曜日", "2027-01-11 成人の日 月曜日"}
	if !slices.Equal(got, want) {
		t.Errorf("holidays = %v, want %v", got, want)
	}
}

func TestRSSFormats(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	res, err := s.Get(context.Background(), Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("items = %+v", res.Items)
	}
	if it := res.Items[0]; it.Title != "新しいお知らせ" || it.URL != "https://www.example.go.jp/new" || it.Date != "2026-11-05T18:30:00+09:00" {
		t.Errorf("newest = %+v (control chars dropped, newest first)", it)
	}
	if it := res.Items[1]; it.Title != "古い お知らせ" {
		t.Errorf("second = %+v", it)
	}
	if it := res.Items[2]; it.URL != "" {
		t.Errorf("non-https link must be dropped: %+v", it)
	}
	if res.Title != "RSS2" || res.Source.URL != "https://a.example/" {
		t.Errorf("title/source = %q %+v", res.Title, res.Source)
	}
	for id, want := range map[string]string{"fdma-news": "RDF の記事", "fdma-disaster": "Atom の記事"} {
		res, err := s.Get(context.Background(), Query{Provider: dsl.FeedRSS, FeedID: id}, 5)
		if err != nil || len(res.Items) != 1 || res.Items[0].Title != want || res.Items[0].URL == "" || res.Items[0].Date == "" {
			t.Errorf("%s: %+v %v", id, res, err)
		}
	}
}

// Anything off the allowlist is refused before any request is made.
func TestAllowlistOnly(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	for _, q := range []Query{
		{Provider: "http"},
		{Provider: dsl.FeedWeather},
		{Provider: dsl.FeedWeather, Region: "999999"},
		{Provider: dsl.FeedWeather, Region: "170000/../../x"},
		{Provider: dsl.FeedWeather, Region: "170000?x=1"},
		{Provider: dsl.FeedRSS, FeedID: "https://evil.example/feed"},
		{Provider: dsl.FeedRSS, FeedID: "nope"},
		{Provider: dsl.FeedHolidays, Region: "130000"},
		{Provider: dsl.FeedRSS, FeedID: "digital-news", Region: "130000"},
	} {
		if _, err := s.Get(context.Background(), q, 5); !errors.Is(err, ErrUnknown) {
			t.Errorf("%+v: err = %v, want ErrUnknown", q, err)
		}
	}
	var total int32
	f.hits.Range(func(_, v any) bool { total += v.(*atomic.Int32).Load(); return true })
	if total != 0 {
		t.Errorf("rejected queries reached the source %d times", total)
	}
}

func TestDisabled(t *testing.T) {
	f := newFake(t)
	o := OptionsFromEnv(func(k string) string {
		return map[string]string{"FEEDS_DISABLED": " rss ,", "FEEDS_UPSTREAM_PER_MINUTE": "7"}[k]
	})
	if o.PerMinute != 7 {
		t.Errorf("perMinute = %d", o.PerMinute)
	}
	s := newSvc(f, &clock{t: testNow}, o)
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}, 5); !errors.Is(err, ErrDisabled) {
		t.Errorf("rss err = %v", err)
	}
	if s.Enabled(dsl.FeedRSS) || !s.Enabled(dsl.FeedWeather) {
		t.Error("Enabled")
	}
	all := OptionsFromEnv(func(k string) string { return map[string]string{"FEEDS_DISABLED": "all"}[k] })
	if len(all.Disabled) != len(dsl.FeedProviders) {
		t.Errorf("all = %v", all.Disabled)
	}
}

func TestCacheAndStale(t *testing.T) {
	f := newFake(t)
	c := &clock{t: testNow}
	s := newSvc(f, c, Options{})
	q := Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}
	ctx := context.Background()
	for range 5 {
		if _, err := s.Get(ctx, q, 5); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count("/rss2.xml"); n != 1 {
		t.Fatalf("source hit %d times within the TTL, want 1", n)
	}
	// After the TTL the source fails: the last good copy is served as stale,
	// and the failure is remembered for FailTTL (no retry storm).
	c.add(16 * time.Minute)
	f.mu.Lock()
	f.code["/rss2.xml"] = http.StatusInternalServerError
	f.mu.Unlock()
	for range 3 {
		res, err := s.Get(ctx, q, 5)
		if err != nil || !res.Stale || len(res.Items) == 0 {
			t.Fatalf("stale = %+v %v", res, err)
		}
	}
	if n := f.count("/rss2.xml"); n != 2 {
		t.Errorf("failing source hit %d times, want 2", n)
	}
	// Too old to stand in.
	c.add(25 * time.Hour)
	if _, err := s.Get(ctx, q, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("err = %v, want ErrUpstream", err)
	}
	// A source that never answered is an error, not an empty list.
	wq := Query{Provider: dsl.FeedWeather, Region: "130000"}
	if _, err := s.Get(ctx, wq, 5); !errors.Is(err, ErrUpstream) || errors.Is(err, ErrRecentFailure) {
		t.Errorf("404 source: %v", err)
	}
	// Within FailTTL the cached failure answers, marked as such (a caller
	// waits FailTTL instead of retrying into it).
	if _, err := s.Get(ctx, wq, 5); !errors.Is(err, ErrRecentFailure) || !errors.Is(err, ErrUpstream) {
		t.Errorf("cached failure: %v", err)
	}
}

func TestConcurrentSameQueryFetchesOnce(t *testing.T) {
	f := newFake(t)
	s := newSvc(f, &clock{t: testNow}, Options{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedHolidays}, 5); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := f.count("/syukujitsu.csv"); n != 1 {
		t.Errorf("source hit %d times, want 1", n)
	}
}

func TestUpstreamBudget(t *testing.T) {
	f := newFake(t)
	c := &clock{t: testNow}
	s := newSvc(f, c, Options{PerMinute: 2})
	ctx := context.Background()
	for _, id := range []string{"digital-news", "fdma-news"} {
		if _, err := s.Get(ctx, Query{Provider: dsl.FeedRSS, FeedID: id}, 5); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedRSS, FeedID: "fdma-disaster"}, 5); !errors.Is(err, ErrBusy) {
		t.Errorf("third fetch in a minute: %v, want ErrBusy", err)
	}
	// Cached answers do not use the budget.
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}, 5); err != nil {
		t.Errorf("cached: %v", err)
	}
	c.add(time.Minute)
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedRSS, FeedID: "fdma-disaster"}, 5); err != nil {
		t.Errorf("next minute: %v", err)
	}
}

func TestBodyCap(t *testing.T) {
	f := newFake(t)
	f.body["/rss2.xml"] = []byte("<rss><channel>" + strings.Repeat("<item><title>x</title></item>", (512<<10)/28+10) + "</channel></rss>")
	s := newSvc(f, &clock{t: testNow}, Options{})
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("oversized body: %v, want ErrUpstream", err)
	}
}

// Redirects may only stay on an allowlisted host.
func TestRedirectOffAllowlist(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(rss2)) }))
	defer other.Close()
	f := newFake(t)
	f.mu.Lock()
	f.redirect = other.URL + "/rss2.xml"
	f.mu.Unlock()
	s := newSvc(f, &clock{t: testNow}, Options{})
	if _, err := s.Get(context.Background(), Query{Provider: dsl.FeedRSS, FeedID: "digital-news"}, 5); !errors.Is(err, ErrUpstream) {
		t.Errorf("redirect to another host: %v, want ErrUpstream", err)
	}
}

// The production client refuses internal addresses even when a name or a
// redirect points there (httptest listens on 127.0.0.1).
func TestClientRefusesInternalAddresses(t *testing.T) {
	f := newFake(t)
	ep := f.endpoints()
	s := New(Options{Endpoints: ep, Now: (&clock{t: testNow}).now}) // NewClient()
	_, err := s.Get(context.Background(), Query{Provider: dsl.FeedHolidays}, 5)
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
	if n := f.count("/syukujitsu.csv"); n != 0 {
		t.Errorf("loopback server was reached %d times", n)
	}
	if _, err := NewClient().Get(f.srv.URL); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("direct dial: %v, want ErrBlockedAddress", err)
	}
}

func TestPublicAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "133.242.0.1": true, "2001:4860:4860::8888": true,
		"127.0.0.1": false, "10.0.0.1": false, "172.16.5.4": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "224.0.0.1": false,
		"::1": false, "fe80::1": false, "fc00::1": false, "fd00:ec2::254": false, "::ffff:127.0.0.1": false,
		"::ffff:169.254.169.254": false, "64:ff9b::a9fe:a9fe": false, "198.18.0.1": false, "255.255.255.255": false,
	} {
		if got := PublicAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("PublicAddr(%s) = %v, want %v", addr, got, want)
		}
	}
}

// The production URLs are fixed https URLs on the allowlisted hosts, and the
// rss allowlist matches the ids the DSL accepts.
func TestDefaultEndpoints(t *testing.T) {
	ep := DefaultEndpoints()
	for h := range ep.hosts() {
		if !strings.HasPrefix(h, "https://") {
			t.Errorf("non-https host %s", h)
		}
	}
	if got := ep.weatherURL("130000"); got != "https://www.jma.go.jp/bosai/forecast/data/forecast/130000.json" {
		t.Errorf("weather url = %s", got)
	}
	var ids []string
	for id, f := range ep.RSS {
		ids = append(ids, id)
		if dsl.RSSFeeds[id] != f.Name {
			t.Errorf("rss %s name %q != dsl %q", id, f.Name, dsl.RSSFeeds[id])
		}
		if !ep.allowedURL(f.URL) || f.Publisher == "" || !strings.HasPrefix(f.SiteURL, "https://") {
			t.Errorf("rss %s: %+v", id, f)
		}
	}
	slices.Sort(ids)
	if !slices.Equal(ids, dsl.FeedIDs()) {
		t.Errorf("rss ids %v != dsl %v", ids, dsl.FeedIDs())
	}
	for _, raw := range []string{"https://user:pw@www.jma.go.jp/x", "http://www.jma.go.jp/x", "https://169.254.169.254/", "https://www.jma.go.jp.evil.example/"} {
		if ep.allowedURL(raw) {
			t.Errorf("allowedURL(%s) = true", raw)
		}
	}
	for p := range dsl.FeedProviders {
		if _, ok := providerLimits[p]; !ok {
			t.Errorf("no limits for %s", p)
		}
	}
}
