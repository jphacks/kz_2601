package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const nhkTestKey = "nhktest0000000000000000000000000"

// nhkNow is the test clock: 2026-11-14 10:00 JST.
var nhkNow = time.Date(2026, 11, 14, 10, 0, 0, 0, jst)

// nhkServer is a fake NHK 番組表 API: it records the path and query and
// answers status with a fixed body.
func nhkServer(t *testing.T, status *atomic.Int64, seen *atomic.Value, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		seen.Store(r.URL)
		if s := status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"fault":{"faultstring":"` + r.URL.Query().Get("key") + `"}}`))
			return
		}
		switch r.URL.Path {
		case "/v2/pg/list/130/g1/2026-11-14.json":
			_, _ = w.Write([]byte(`{"list":{"g1":[
				{"id":"2026111400001","event_id":"00001","start_time":"2026-11-14T05:00:00+09:00","end_time":"2026-11-14T06:00:00+09:00","area":{"id":"130","name":"東京"},"service":{"id":"g1","name":"ＮＨＫ総合１・東京"},"title":"  ニュース   おはよう日本 ","subtitle":"▽最新ニュース","content":"c","act":"","genres":["0000"]},
				{"id":"2026111400002","start_time":"2026-11-14T06:00:00+09:00","end_time":"2026-11-14T06:30:00+09:00","service":{"id":"g1","name":"ＮＨＫ総合１・東京"},"title":"","subtitle":"なし"},
				{"id":"2026111400003","start_time":"bad","service":{"id":"g1","name":"x"},"title":"時刻なし"},
				{"id":"2026111400004","start_time":"2026-11-14T23:50:00+09:00","end_time":"2026-11-15T00:20:00+09:00","service":{"id":"g1","name":"ＮＨＫ総合１・東京"},"title":"深夜","subtitle":""}]}}`))
		case "/v2/pg/list/130/tv/2026-11-15.json":
			_, _ = w.Write([]byte(`{"list":{"e1":[
				{"id":"2","start_time":"2026-11-15T07:00:00+09:00","end_time":"2026-11-15T07:30:00+09:00","service":{"id":"e1","name":"ＮＨＫＥテレ１・東京"},"title":"教育"}],
				"g1":[{"id":"1","start_time":"2026-11-15T06:00:00+09:00","end_time":"2026-11-15T07:00:00+09:00","service":{"id":"g1","name":"ＮＨＫ総合１・東京"},"title":"総合"}]}}`))
		case "/v2/pg/list/130/g1/2026-11-13.json":
			_, _ = w.Write([]byte(`{"list":{"g1":[]}}`))
		case "/v2/pg/now/130/g1.json":
			_, _ = w.Write([]byte(`{"nowonair_list":{"g1":{
				"previous":{"id":"p","start_time":"2026-11-14T09:00:00+09:00","end_time":"2026-11-14T10:00:00+09:00","service":{"id":"g1","name":"ＮＨＫ総合１・東京"},"title":"前の番組"},
				"present":{"id":"n","start_time":"2026-11-14T10:00:00+09:00","end_time":"2026-11-14T10:45:00+09:00","service":{"id":"g1","name":"ＮＨＫ総合１・東京"},"title":"今の番組","subtitle":"字幕"},
				"following":{"id":"f","start_time":"2026-11-14T10:45:00+09:00","end_time":"2026-11-14T11:00:00+09:00","service":{"id":"g1","name":"ＮＨＫ総合１・東京"},"title":"次の番組"}}}}`))
		case "/v2/pg/now/130/r1.json":
			_, _ = w.Write([]byte(`{"nowonair_list":{}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type nhkFake struct {
	srv    *httptest.Server
	status atomic.Int64
	seen   atomic.Value
	hits   atomic.Int64
}

func newNHKFake(t *testing.T) *nhkFake {
	t.Helper()
	f := &nhkFake{}
	f.status.Store(200)
	f.srv = nhkServer(t, &f.status, &f.seen, &f.hits)
	return f
}

func (f *nhkFake) ops(t *testing.T) map[string]Operation {
	t.Helper()
	ops, err := newNHK(nhkTestKey, f.srv.URL, f.srv.Client(), func() time.Time { return nhkNow })
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func (f *nhkFake) url() *url.URL { return f.seen.Load().(*url.URL) }

func TestNHKPrograms(t *testing.T) {
	f := newNHKFake(t)
	ops := f.ops(t)
	// the defaults: 130 / g1 / today (JST)
	out, err := ops["programs"].Do(t.Context(), Input{Args: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if u := f.url(); u.Path != "/v2/pg/list/130/g1/2026-11-14.json" || u.Query().Get("key") != nhkTestKey {
		t.Fatalf("request %v", u)
	}
	// a program without a title or a start time is skipped; titles are one
	// line; the span is JST; the end past midnight is still HH:MM
	want := []Row{
		{Title: "ニュース おはよう日本", Subtitle: "05:00–06:00 · ▽最新ニュース", Value: "ＮＨＫ総合１・東京"},
		{Title: "深夜", Subtitle: "23:50–00:20", Value: "ＮＨＫ総合１・東京"},
	}
	if len(out.Rows) != len(want) || out.Item != nil {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	if out, err := ops["programs"].Do(t.Context(), Input{Args: map[string]string{"limit": "1"}}); err != nil || len(out.Rows) != 1 {
		t.Fatalf("limit 1: %+v %v", out, err)
	}
	// a bundle (tv) is several lists, merged by start time
	out, err = ops["programs"].Do(t.Context(), Input{Args: map[string]string{"service": "TV", "date": "2026-11-15"}})
	if err != nil || len(out.Rows) != 2 || out.Rows[0].Title != "総合" || out.Rows[1].Title != "教育" {
		t.Fatalf("tv: %+v %v", out, err)
	}
	// an empty day is upstream_unavailable (nothing to show)
	if _, err := ops["programs"].Do(t.Context(), Input{Args: map[string]string{"date": "2026-11-13"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("empty: %s (%v)", Reason(err), err)
	}
}

func TestNHKNow(t *testing.T) {
	f := newNHKFake(t)
	ops := f.ops(t)
	out, err := ops["now"].Do(t.Context(), Input{Args: map[string]string{"area": "130", "service": "g1"}})
	if err != nil {
		t.Fatal(err)
	}
	if u := f.url(); u.Path != "/v2/pg/now/130/g1.json" {
		t.Fatalf("request %v", u)
	}
	want := []Row{
		{Title: "前の番組", Subtitle: "前 09:00–10:00", Value: "ＮＨＫ総合１・東京"},
		{Title: "今の番組", Subtitle: "今 10:00–10:45 · 字幕", Value: "ＮＨＫ総合１・東京"},
		{Title: "次の番組", Subtitle: "次 10:45–11:00", Value: "ＮＨＫ総合１・東京"},
	}
	if len(out.Rows) != len(want) || out.Item == nil || *out.Item != want[1] {
		t.Fatalf("output %+v %+v", out, out.Item)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	if _, err := ops["now"].Do(t.Context(), Input{Args: map[string]string{"service": "r1"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("nothing on air: %s (%v)", Reason(err), err)
	}
}

func TestNHKRefusesBadArgs(t *testing.T) {
	p, _ := Lookup(NHK)
	today := time.Now().In(jst).Format("2006-01-02")
	for _, a := range []map[string]string{
		{"area": "13"}, {"area": "1300"}, {"area": "abc"}, {"service": "g9"}, {"service": "https://example.com"},
		{"date": "2026/11/14"}, {"date": "20261114"}, {"date": "2000-01-01"}, {"date": "2099-12-31"}, {"date": "2026-13-01"},
		{"limit": "0"}, {"limit": "101"}, {"q": "x"},
	} {
		if _, err := p.CheckArgs("programs", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("programs %v: %s (%v)", a, Reason(err), err)
		}
	}
	for _, a := range []map[string]string{{}, {"area": "270", "service": "e1"}, {"date": today, "limit": "100"}, {"service": "netradio"}} {
		if _, err := p.CheckArgs("programs", a); err != nil {
			t.Fatalf("programs %v: %v", a, err)
		}
	}
	for _, a := range []map[string]string{{"area": "1"}, {"service": "x"}, {"date": today}} {
		if _, err := p.CheckArgs("now", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("now %v: %s (%v)", a, Reason(err), err)
		}
	}
	// the date's range is around the clock's day: yesterday to 7 days ahead
	f := newNHKFake(t)
	ops := f.ops(t)
	for _, d := range []string{"2026-11-12", "2026-11-22"} {
		if _, err := ops["programs"].Do(t.Context(), Input{Args: map[string]string{"date": d}}); !errors.Is(err, ErrBadArgs) {
			t.Fatalf("date %s: %v", d, err)
		}
	}
	if f.hits.Load() != 0 {
		t.Fatal("NHK was called with bad args")
	}
	for _, d := range []string{"2026-11-13", "2026-11-21"} {
		if _, err := argNHKDate(map[string]string{"date": d}, nhkNow); err != nil {
			t.Fatalf("date %s: %v", d, err)
		}
	}
}

func TestNHKDefinition(t *testing.T) {
	p, ok := Lookup(NHK)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "NHK" || p.GlobalDaily != 300 {
		t.Fatalf("%+v", p)
	}
	if len(p.Hosts) != 1 || p.Hosts[0] != nhkHost {
		t.Fatalf("hosts %v", p.Hosts)
	}
	for _, o := range p.Ops {
		if o.CacheTTL != 10*time.Minute || o.Write {
			t.Errorf("%s: 10-minute cached read op expected: %+v", o.Name, o)
		}
	}
	for _, k := range []string{"", "short", "has space 0123456789abcdef"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: nhkTestKey}); err != nil {
		t.Fatal(err)
	}
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_NHK_API_KEY" {
			return nhkTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[NHK] {
		t.Fatalf("enabled without EXT_NHK_ENABLED: %v", err)
	}
	// the key's 300 a day is the environment-wide default; env wins
	if c.Limits[NHK].GlobalDaily != 300 {
		t.Fatalf("global daily %d", c.Limits[NHK].GlobalDaily)
	}
	c, err = ConfigFromEnv(func(k string) string {
		if k == "EXT_GLOBAL_DAILY_NHK" {
			return "250"
		}
		return ""
	})
	if err != nil || c.Limits[NHK].GlobalDaily != 250 {
		t.Fatalf("env global daily: %d %v", c.Limits[NHK].GlobalDaily, err)
	}
	hc := NewHTTPClient(p.Hosts, DefaultTimeout)
	if _, err := hc.Get("https://example.com/v2/pg/now/130/g1.json"); err == nil || !strings.Contains(err.Error(), ErrEgress.Error()) {
		t.Fatalf("other host reached: %v", err)
	}
	if !strings.Contains(NHKAttribution, "NHK") {
		t.Fatal("attribution")
	}
}

// Through Service.Call: one `runs`, no coins, the second same call within
// 10 minutes from the cache; a 5xx / 429 gives the run back and the key is
// in no error or log.
func TestNHKThroughService(t *testing.T) {
	f := newNHKFake(t)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[NHK] = true
		c.Secrets = map[string]Secrets{NHK: {APIKey: nhkTestKey}}
	}})
	if !e.svc.Enabled(NHK) {
		t.Fatal("not enabled")
	}
	e.svc.ops[NHK] = f.ops(t)
	e.fund(user, 100)
	args := map[string]string{"area": "130", "service": "g1"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(NHK, "now", args))
		if err != nil || res.Output.Item == nil || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if f.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", f.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(10 * time.Minute)
	if _, err := e.call(e.req(NHK, "now", args)); err != nil || f.hits.Load() != 2 {
		t.Fatalf("after 10 minutes: %v hits=%d", err, f.hits.Load())
	}
	for _, s := range []int64{503, 429, 401} {
		f.status.Store(s)
		_, err := e.call(e.req(NHK, "now", map[string]string{"service": "e1"}))
		if Reason(err) != ReasonUpstream || strings.Contains(err.Error(), nhkTestKey) {
			t.Fatalf("status %d: %v", s, err)
		}
		if e.runs() != 2 {
			t.Fatalf("status %d: runs = %d, the run was not given back", s, e.runs())
		}
	}
	if w := e.wallet(user); w.Available() != 100 {
		t.Fatalf("coins touched: %+v", w)
	}
	if strings.Contains(e.logs.String(), nhkTestKey) {
		t.Fatal("the key is in the logs")
	}
}
