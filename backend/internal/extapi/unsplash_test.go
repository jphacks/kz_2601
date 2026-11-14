package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const unsplashTestKey = "abcDEF0123456789_-xyzABC0123"

const unsplashPhotoJSON = `{"id":"p1","description":"  富士山と\n湖 ","alt_description":"mountain","width":4000,"height":3000,
"urls":{"regular":"https://images.unsplash.com/photo-1?ixid=x&w=1080"},
"links":{"html":"https://unsplash.com/photos/p1","download_location":"https://api.unsplash.com/photos/p1/download?ixid=x"},
"user":{"name":"Taro","links":{"html":"https://unsplash.com/@taro"}}}`

// unsplashFake is a fake api.unsplash.com: it records the request and
// answers with status and a fixed body.
type unsplashFake struct {
	srv    *httptest.Server
	status atomic.Int64
	body   atomic.Value
	seen   atomic.Value
	hits   atomic.Int64
}

func newUnsplashFake(t *testing.T) *unsplashFake {
	t.Helper()
	f := &unsplashFake{}
	f.status.Store(200)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.seen.Store(r.Clone(r.Context()))
		if s := f.status.Load(); s != 200 {
			w.WriteHeader(int(s))
			b, _ := f.body.Load().(string)
			_, _ = w.Write([]byte(b + r.Header.Get("Authorization")))
			return
		}
		switch r.URL.Path {
		case "/search/photos":
			_, _ = w.Write([]byte(`{"total":4,"results":[` + unsplashPhotoJSON + `,
				{"id":"p2","description":null,"alt_description":"a cat","width":0,"height":0,"urls":{"regular":"https://images.unsplash.com/photo-2"},"links":{"html":"https://unsplash.com/photos/p2?a=1"},"user":{"name":""}},
				{"id":"p3","urls":{"regular":"http://evil.example.com/x.jpg"},"user":{"name":"bad"}},
				{"id":"","urls":{"regular":"https://images.unsplash.com/photo-4"}},
				{"id":"p5","urls":{"regular":"https://images.unsplash.com/photo-5"},"links":{"html":"https://user@unsplash.com/photos/p5"},"user":{"name":"Hanako"}}]}`))
		case "/photos/random":
			_, _ = w.Write([]byte(unsplashPhotoJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *unsplashFake) ops(t *testing.T) map[string]Operation {
	t.Helper()
	ops, err := newUnsplash(unsplashTestKey, f.srv.URL, f.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func (f *unsplashFake) req() *http.Request { return f.seen.Load().(*http.Request) }

func TestUnsplashSearchPhotos(t *testing.T) {
	f := newUnsplashFake(t)
	ops := f.ops(t)
	out, err := ops["searchPhotos"].Do(t.Context(), Input{Args: map[string]string{"query": " 富士山 ", "orientation": "landscape", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	r := f.req()
	q := r.URL.Query()
	if r.URL.Path != "/search/photos" || q.Get("query") != "富士山" || q.Get("orientation") != "landscape" || q.Get("per_page") != "5" ||
		q.Get("page") != "1" || q.Get("content_filter") != "high" || q.Has("client_id") ||
		r.Header.Get("Authorization") != "Client-ID "+unsplashTestKey || r.Header.Get("Accept-Version") != "v1" {
		t.Fatalf("request %v %v", r.URL, r.Header)
	}
	// no id / a non-https image are skipped; the page link gets the utm; a
	// link with a user is dropped; no description falls back to alt / 写真
	want := []Row{
		{Title: "富士山と 湖", Subtitle: "Photo by Taro on Unsplash", Value: "4000×3000", URL: "https://unsplash.com/photos/p1?" + unsplashUTM},
		{Title: "a cat", Subtitle: "Photo by Unsplash on Unsplash", URL: "https://unsplash.com/photos/p2?a=1&" + unsplashUTM},
		{Title: "写真", Subtitle: "Photo by Hanako on Unsplash"},
	}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	wantCell := []string{"p1", "https://images.unsplash.com/photo-1?ixid=x&w=1080", "https://unsplash.com/@taro?" + unsplashUTM, "https://api.unsplash.com/photos/p1/download?ixid=x"}
	if len(out.Cells) != len(want) || !slices.Equal(out.Cells[0], wantCell) {
		t.Fatalf("cells %v", out.Cells)
	}
	// limit cuts the rows; the default per_page is 10; no orientation is sent when empty
	out, err = ops["searchPhotos"].Do(t.Context(), Input{Args: map[string]string{"query": "x", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("limit 1: %+v %v", out.Rows, err)
	}
	if _, err := ops["searchPhotos"].Do(t.Context(), Input{Args: map[string]string{"query": "x"}}); err != nil ||
		f.req().URL.Query().Get("per_page") != "10" || f.req().URL.Query().Has("orientation") {
		t.Fatalf("defaults: %v %v", f.req().URL, err)
	}
}

func TestUnsplashRandom(t *testing.T) {
	f := newUnsplashFake(t)
	out, err := f.ops(t)["random"].Do(t.Context(), Input{Args: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if r := f.req(); r.URL.Path != "/photos/random" || r.URL.Query().Has("query") || r.URL.Query().Get("content_filter") != "high" {
		t.Fatalf("request %v", r.URL)
	}
	if out.Item == nil || out.Item.Title != "富士山と 湖" || len(out.Rows) != 1 || out.Rows[0] != *out.Item || len(out.Cells) != 1 {
		t.Fatalf("out %+v", out)
	}
	if _, err := f.ops(t)["random"].Do(t.Context(), Input{Args: map[string]string{"query": "cat", "orientation": "squarish"}}); err != nil ||
		f.req().URL.Query().Get("query") != "cat" || f.req().URL.Query().Get("orientation") != "squarish" {
		t.Fatalf("query: %v %v", f.req().URL, err)
	}
}

func TestUnsplashErrors(t *testing.T) {
	f := newUnsplashFake(t)
	ops := f.ops(t)
	args := map[string]string{"query": "x"}
	// 404 / 400 are the request's fault: 400 bad_request
	for _, s := range []int64{404, 400} {
		f.status.Store(s)
		f.body.Store(`{"errors":["nope"]}`)
		if _, err := ops["searchPhotos"].Do(t.Context(), Input{Args: args}); !errors.Is(err, ErrUnknown) || Reason(err) != ReasonBadRequest {
			t.Fatalf("%d: %v", s, err)
		}
	}
	// 401 (the key), 403 rate limit, 429, 5xx: ProviderError, never the key
	for _, c := range []struct {
		st    int64
		body  string
		retry bool
	}{{401, `{"errors":["OAuth error"]}`, false}, {403, `Rate Limit Exceeded`, true}, {429, ``, true}, {503, `<html>`, true}} {
		f.status.Store(c.st)
		f.body.Store(c.body)
		var pe *ProviderError
		_, err := ops["searchPhotos"].Do(t.Context(), Input{Args: args})
		if !errors.As(err, &pe) || pe.Retryable != c.retry || strings.Contains(err.Error(), unsplashTestKey) {
			t.Fatalf("%d %s: %v", c.st, c.body, err)
		}
	}
}

func TestUnsplashRefusesBadArgs(t *testing.T) {
	p, _ := Lookup(Unsplash)
	for _, a := range []map[string]string{
		{}, {"query": " "}, {"query": strings.Repeat("あ", 101)}, {"query": "a\nb"}, {"query": "x", "orientation": "wide"},
		{"query": "x", "limit": "0"}, {"query": "x", "limit": "21"}, {"query": "x", "page": "2"},
	} {
		if _, err := p.CheckArgs("searchPhotos", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("searchPhotos %v: %s (%v)", a, Reason(err), err)
		}
	}
	for _, a := range []map[string]string{{"orientation": "x"}, {"query": "a\tb"}, {"limit": "1"}} {
		if _, err := p.CheckArgs("random", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("random %v: %s (%v)", a, Reason(err), err)
		}
	}
	for _, c := range []struct {
		op string
		a  map[string]string
	}{{"searchPhotos", map[string]string{"query": strings.Repeat("あ", 100), "orientation": "portrait", "limit": "20"}}, {"random", map[string]string{}}} {
		if _, err := p.CheckArgs(c.op, c.a); err != nil {
			t.Fatalf("%s %v: %v", c.op, c.a, err)
		}
	}
	// the ops refuse them too, before any request
	f := newUnsplashFake(t)
	if _, err := f.ops(t)["searchPhotos"].Do(t.Context(), Input{Args: map[string]string{}}); !errors.Is(err, ErrBadArgs) {
		t.Fatalf("no query: %v", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("Unsplash was called with bad args")
	}
}

func TestUnsplashDefinition(t *testing.T) {
	p, ok := Lookup(Unsplash)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "UNSPLASH" {
		t.Fatalf("%+v", p)
	}
	if len(p.Hosts) != 1 || p.Hosts[0] != unsplashHost {
		t.Fatalf("hosts %v", p.Hosts)
	}
	for _, o := range p.Ops {
		want := time.Hour
		if o.Name == "random" {
			want = 0 // a new photo every time
		}
		if o.CacheTTL != want || o.Write {
			t.Errorf("%s: %+v", o.Name, o)
		}
	}
	for _, k := range []string{"", "short", "has space 0123456789abcdef", "key/with/slash0123456789"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: unsplashTestKey}); err != nil {
		t.Fatal(err)
	}
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_UNSPLASH_API_KEY" {
			return unsplashTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[Unsplash] {
		t.Fatalf("enabled without EXT_UNSPLASH_ENABLED: %v", err)
	}
	// only api.unsplash.com (the images are hotlinked by the device, never fetched here)
	hc := NewHTTPClient(p.Hosts, DefaultTimeout)
	for _, u := range []string{"https://example.com/search/photos", "https://images.unsplash.com/photo-1"} {
		if _, err := hc.Get(u); err == nil || !strings.Contains(err.Error(), ErrEgress.Error()) {
			t.Fatalf("%s reached: %v", u, err)
		}
	}
}

// Through Service.Call: one `runs`, no coins, searchPhotos cached for an
// hour, random never; a 5xx gives the run back.
func TestUnsplashThroughService(t *testing.T) {
	f := newUnsplashFake(t)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[Unsplash] = true
		c.Secrets = map[string]Secrets{Unsplash: {APIKey: unsplashTestKey}}
	}})
	if !e.svc.Enabled(Unsplash) {
		t.Fatal("not enabled")
	}
	e.svc.ops[Unsplash] = f.ops(t)
	e.fund(user, 100)
	args := map[string]string{"query": "富士山"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(Unsplash, "searchPhotos", args))
		if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if f.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", f.hits.Load(), e.runs())
	}
	for i := 0; i < 2; i++ {
		if _, err := e.call(e.req(Unsplash, "random", nil)); err != nil {
			t.Fatal(err)
		}
	}
	if f.hits.Load() != 3 || e.runs() != 3 {
		t.Fatalf("random cached: hits=%d runs=%d", f.hits.Load(), e.runs())
	}
	f.status.Store(503)
	if _, err := e.call(e.req(Unsplash, "random", nil)); Reason(err) != ReasonUpstream || strings.Contains(err.Error(), unsplashTestKey) {
		t.Fatalf("503: %v", err)
	}
	if e.runs() != 3 {
		t.Fatalf("503: runs = %d, the run was not given back", e.runs())
	}
	if w := e.wallet(user); w.Available() != 100 {
		t.Fatalf("coins touched: %+v", w)
	}
	if strings.Contains(e.logs.String(), unsplashTestKey) {
		t.Fatal("the key is in the logs")
	}
}
