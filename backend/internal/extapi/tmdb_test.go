package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	tmdbTestKey   = "0123456789abcdef0123456789abcdef"          // a v3 api_key
	tmdbTestToken = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ0In0.c2ln" // a v4 read access token (JWT shape)
)

// tmdbServer is a fake TMDB: it records the request and answers status with
// a fixed page.
func tmdbServer(t *testing.T, status *atomic.Int64, seen *atomic.Value, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		seen.Store(r.Clone(r.Context()))
		if s := status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"status_message":"` + r.URL.Query().Get("api_key") + r.Header.Get("Authorization") + `"}`))
			return
		}
		switch r.URL.Path {
		case "/3/search/movie":
			_, _ = w.Write([]byte(`{"page":1,"results":[
				{"id":129,"title":"千と千尋の神隠し","release_date":"2001-07-20","overview":"` + strings.Repeat("あ", 120) + `","vote_average":8.538,"poster_path":"/abc.jpg"},
				{"id":130,"title":"  改行\nあり ","release_date":"","overview":"","vote_average":0,"poster_path":null},
				{"id":0,"title":"id なし"},
				{"id":131,"title":"","overview":"題名なし"},
				{"id":132,"title":"三つ目","release_date":"2020-01-01","overview":"短い","vote_average":7.26,"poster_path":"bad/../x"}],"total_results":5}`))
		case "/3/search/tv":
			_, _ = w.Write([]byte(`{"page":1,"results":[{"id":1,"name":"ドラマ","first_air_date":"2019-04-01","overview":"話","vote_average":6.9,"poster_path":"/tv.jpg"}]}`))
		case "/3/trending/all/week":
			_, _ = w.Write([]byte(`{"page":1,"results":[
				{"id":5,"media_type":"tv","name":"人気ドラマ","first_air_date":"2024-01-01","vote_average":7.1},
				{"id":6,"media_type":"person","name":"俳優"},
				{"id":9,"media_type":"movie","adult":true,"title":"成人向け","release_date":"2024-02-01","vote_average":5},
				{"id":7,"media_type":"movie","title":"人気映画","release_date":"2024-02-02","vote_average":8}]}`))
		case "/3/trending/movie/day":
			_, _ = w.Write([]byte(`{"page":1,"results":[{"id":8,"title":"今日の映画","release_date":"2024-03-03","vote_average":6.5}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type tmdbFake struct {
	srv    *httptest.Server
	status atomic.Int64
	seen   atomic.Value
	hits   atomic.Int64
}

func newTMDBFake(t *testing.T) *tmdbFake {
	t.Helper()
	f := &tmdbFake{}
	f.status.Store(200)
	f.srv = tmdbServer(t, &f.status, &f.seen, &f.hits)
	return f
}

func (f *tmdbFake) ops(t *testing.T, key string) map[string]Operation {
	t.Helper()
	ops, err := newTMDB(key, f.srv.URL, f.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func (f *tmdbFake) req() *http.Request { return f.seen.Load().(*http.Request) }

func TestTMDBSearch(t *testing.T) {
	f := newTMDBFake(t)
	ops := f.ops(t, tmdbTestKey)
	out, err := ops["search"].Do(t.Context(), Input{Args: map[string]string{"q": " 千と千尋 ", "limit": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	r := f.req()
	q := r.URL.Query()
	// a v3 key is in the query, Japanese, page 1, no adult titles
	if r.URL.Path != "/3/search/movie" || q.Get("query") != "千と千尋" || q.Get("language") != "ja-JP" || q.Get("api_key") != tmdbTestKey ||
		q.Get("page") != "1" || q.Get("include_adult") != "false" || r.Header.Get("Authorization") != "" {
		t.Fatalf("request %v %v", r.URL, r.Header)
	}
	// the overview is cut at 100 runes; one-line titles; no vote when 0; a
	// row without an id or a title is skipped; a poster is only a plain path
	want := []Row{
		{Title: "千と千尋の神隠し", Subtitle: "2001-07-20 · " + strings.Repeat("あ", 99) + "…", Value: "8.5", URL: "https://www.themoviedb.org/movie/129"},
		{Title: "改行 あり", URL: "https://www.themoviedb.org/movie/130"},
		{Title: "三つ目", Subtitle: "2020-01-01 · 短い", Value: "7.3", URL: "https://www.themoviedb.org/movie/132"},
	}
	if len(out.Rows) != len(want) || len(out.Cells) != len(want) {
		t.Fatalf("rows %+v cells %v", out.Rows, out.Cells)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	if c := out.Cells[0]; c[4] != "https://image.tmdb.org/t/p/w185/abc.jpg" || out.Cells[1][4] != "" || out.Cells[2][4] != "" {
		t.Fatalf("poster cells %v", out.Cells)
	}
	if out, err := ops["search"].Do(t.Context(), Input{Args: map[string]string{"q": "x", "limit": "2"}}); err != nil || len(out.Rows) != 2 {
		t.Fatalf("limit 2: %+v %v", out, err)
	}
	// tv: name / first_air_date, the tv page
	out, err = ops["search"].Do(t.Context(), Input{Args: map[string]string{"q": "ドラマ", "type": "TV"}})
	if err != nil || f.req().URL.Path != "/3/search/tv" || len(out.Rows) != 1 {
		t.Fatalf("tv: %+v %v", out, err)
	}
	if want := (Row{Title: "ドラマ", Subtitle: "2019-04-01 · 話", Value: "6.9", URL: "https://www.themoviedb.org/tv/1"}); out.Rows[0] != want {
		t.Fatalf("tv row %+v", out.Rows[0])
	}
	// a v4 read access token goes in the Authorization header, not the query
	ops = f.ops(t, tmdbTestToken)
	if _, err := ops["search"].Do(t.Context(), Input{Args: map[string]string{"q": "x"}}); err != nil {
		t.Fatal(err)
	}
	if r := f.req(); r.Header.Get("Authorization") != "Bearer "+tmdbTestToken || r.URL.Query().Has("api_key") {
		t.Fatalf("token request %v %v", r.URL, r.Header)
	}
}

func TestTMDBTrending(t *testing.T) {
	f := newTMDBFake(t)
	ops := f.ops(t, tmdbTestKey)
	// the defaults: movie / day
	out, err := ops["trending"].Do(t.Context(), Input{Args: map[string]string{}})
	if err != nil || f.req().URL.Path != "/3/trending/movie/day" || len(out.Rows) != 1 || out.Rows[0].Title != "今日の映画" {
		t.Fatalf("defaults: %+v %v (%v)", out, err, f.req().URL)
	}
	if q := f.req().URL.Query(); q.Get("language") != "ja-JP" || q.Has("query") {
		t.Fatalf("query %v", q)
	}
	// all: each item's own media type; people and adult titles are left out
	out, err = ops["trending"].Do(t.Context(), Input{Args: map[string]string{"type": "all", "window": "week"}})
	if err != nil || f.req().URL.Path != "/3/trending/all/week" {
		t.Fatalf("all: %v (%v)", err, f.req().URL)
	}
	want := []Row{
		{Title: "人気ドラマ", Subtitle: "2024-01-01", Value: "7.1", URL: "https://www.themoviedb.org/tv/5"},
		{Title: "人気映画", Subtitle: "2024-02-02", Value: "8.0", URL: "https://www.themoviedb.org/movie/7"},
	}
	if len(out.Rows) != len(want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
}

func TestTMDBRefusesBadArgs(t *testing.T) {
	p, _ := Lookup(TMDB)
	for _, a := range []map[string]string{
		{}, {"q": ""}, {"q": " "}, {"q": strings.Repeat("あ", 101)}, {"q": "a\tb"}, {"q": "x", "type": "all"}, {"q": "x", "type": "person"},
		{"q": "x", "limit": "0"}, {"q": "x", "limit": "21"}, {"q": "x", "window": "day"}, {"q": "https://example.com/x"},
	} {
		if _, err := p.CheckArgs("search", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("search %v: %s (%v)", a, Reason(err), err)
		}
	}
	if _, err := p.CheckArgs("search", map[string]string{"q": strings.Repeat("あ", 100), "type": "tv", "limit": "20"}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []map[string]string{{"type": "person"}, {"window": "month"}, {"q": "x"}, {"limit": "1"}} {
		if _, err := p.CheckArgs("trending", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("trending %v: %s (%v)", a, Reason(err), err)
		}
	}
	for _, a := range []map[string]string{{}, {"type": "all", "window": "week"}, {"type": "tv"}} {
		if _, err := p.CheckArgs("trending", a); err != nil {
			t.Fatalf("trending %v: %v", a, err)
		}
	}
	// the ops refuse them too, before any request
	f := newTMDBFake(t)
	ops := f.ops(t, tmdbTestKey)
	if _, err := ops["search"].Do(t.Context(), Input{Args: map[string]string{"q": ""}}); !errors.Is(err, ErrBadArgs) {
		t.Fatalf("empty q: %v", err)
	}
	if _, err := ops["trending"].Do(t.Context(), Input{Args: map[string]string{"window": "year"}}); !errors.Is(err, ErrBadArgs) {
		t.Fatalf("bad window: %v", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("TMDB was called with bad args")
	}
}

func TestTMDBDefinition(t *testing.T) {
	p, ok := Lookup(TMDB)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "TMDB" {
		t.Fatalf("%+v", p)
	}
	if len(p.Hosts) != 1 || p.Hosts[0] != tmdbHost {
		t.Fatalf("hosts %v", p.Hosts)
	}
	for _, o := range p.Ops {
		if o.CacheTTL != time.Hour || o.Write {
			t.Errorf("%s: 1-hour cached read op expected: %+v", o.Name, o)
		}
	}
	for _, k := range []string{"", "short", "has space 0123456789abcdef", "key/with/slash0123456789"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	for _, k := range []string{tmdbTestKey, tmdbTestToken} {
		if _, err := p.New(Secrets{APIKey: k}); err != nil {
			t.Fatal(err)
		}
	}
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_TMDB_API_KEY" {
			return tmdbTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[TMDB] {
		t.Fatalf("enabled without EXT_TMDB_ENABLED: %v", err)
	}
	// only api.themoviedb.org (the image host is never fetched)
	hc := NewHTTPClient(p.Hosts, DefaultTimeout)
	for _, u := range []string{"https://example.com/3/search/movie", "https://image.tmdb.org/t/p/w185/abc.jpg"} {
		if _, err := hc.Get(u); err == nil || !strings.Contains(err.Error(), ErrEgress.Error()) {
			t.Fatalf("%s reached: %v", u, err)
		}
	}
	if !strings.Contains(TMDBAttribution, "not endorsed, certified, or otherwise approved by TMDB") {
		t.Fatal("attribution")
	}
}

// Through Service.Call: one `runs`, no coins, the second same call within
// the hour from the cache; a 5xx / 429 / 401 gives the run back and the
// key is in no error or log.
func TestTMDBThroughService(t *testing.T) {
	f := newTMDBFake(t)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[TMDB] = true
		c.Secrets = map[string]Secrets{TMDB: {APIKey: tmdbTestKey}}
	}})
	if !e.svc.Enabled(TMDB) {
		t.Fatal("not enabled")
	}
	e.svc.ops[TMDB] = f.ops(t, tmdbTestKey)
	e.fund(user, 100)
	args := map[string]string{"q": "千と千尋"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(TMDB, "search", args))
		if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if f.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", f.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(time.Hour)
	if _, err := e.call(e.req(TMDB, "search", args)); err != nil || f.hits.Load() != 2 {
		t.Fatalf("after an hour: %v hits=%d", err, f.hits.Load())
	}
	for _, s := range []int64{503, 429, 401} {
		f.status.Store(s)
		_, err := e.call(e.req(TMDB, "trending", nil))
		if Reason(err) != ReasonUpstream || strings.Contains(err.Error(), tmdbTestKey) {
			t.Fatalf("status %d: %v", s, err)
		}
		if e.runs() != 2 {
			t.Fatalf("status %d: runs = %d, the run was not given back", s, e.runs())
		}
	}
	if w := e.wallet(user); w.Available() != 100 {
		t.Fatalf("coins touched: %+v", w)
	}
	if strings.Contains(e.logs.String(), tmdbTestKey) {
		t.Fatal("the key is in the logs")
	}
}
