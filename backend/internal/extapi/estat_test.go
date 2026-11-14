package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const estatTestKey = "0123456789abcdef0123456789abcdef01234567"

// estatServer is a fake e-Stat: it records the query and answers status
// with a fixed body (a list of two tables, or a table's cells; one-element
// lists are objects, as the API writes them).
func estatServer(t *testing.T, status *atomic.Int64, seen *atomic.Value, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		seen.Store(r.URL.Query())
		if s := status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"error":"` + r.URL.Query().Get("appId") + `"}`))
			return
		}
		switch r.URL.Path {
		case "/rest/3.0/app/json/getStatsList":
			switch r.URL.Query().Get("searchWord") {
			case "なし":
				_, _ = w.Write([]byte(`{"GET_STATS_LIST":{"RESULT":{"STATUS":1,"ERROR_MSG":"該当データはありませんでした。"},"DATALIST_INF":{"NUMBER":0}}}`))
			case "拒否":
				_, _ = w.Write([]byte(`{"GET_STATS_LIST":{"RESULT":{"STATUS":102,"ERROR_MSG":"値が正しくありません。"}}}`))
			case "落ちた":
				_, _ = w.Write([]byte(`{"GET_STATS_LIST":{"RESULT":{"STATUS":200,"ERROR_MSG":"DB"}}}`))
			case "ひとつ":
				_, _ = w.Write([]byte(`{"GET_STATS_LIST":{"RESULT":{"STATUS":0},"DATALIST_INF":{"NUMBER":1,"TABLE_INF":
					{"@id":"0003411955","STAT_NAME":{"@code":"00200521","$":"国勢調査"},"TITLE":{"@no":"1","$":"人口"},"SURVEY_DATE":"202010"}}}}`))
			default:
				_, _ = w.Write([]byte(`{"GET_STATS_LIST":{"RESULT":{"STATUS":0},"DATALIST_INF":{"NUMBER":3,"TABLE_INF":[
					{"@id":"0003411955","STAT_NAME":{"@code":"00200521","$":"国勢調査"},"TITLE":{"@no":"1","$":"  人口  、世帯数 "},"SURVEY_DATE":"202010"},
					{"@id":"0003412000","STAT_NAME":{"@code":"00200502","$":"労働力調査"},"TITLE":"就業者数","SURVEY_DATE":"202001-202012"},
					{"@id":"","TITLE":"id なし"},
					{"@id":"0003412001","STAT_NAME":{"$":"家計調査"},"TITLE":"支出","SURVEY_DATE":0}]}}}`))
			}
		case "/rest/3.0/app/json/getStatsData":
			if r.URL.Query().Get("statsDataId") == "0000000000" {
				_, _ = w.Write([]byte(`{"GET_STATS_DATA":{"RESULT":{"STATUS":300,"ERROR_MSG":"データは存在しません"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"GET_STATS_DATA":{"RESULT":{"STATUS":0},"STATISTICAL_DATA":{"CLASS_INF":{"CLASS_OBJ":[
				{"@id":"tab","@name":"表章項目","CLASS":{"@code":"020","@name":"人口","@unit":"人"}},
				{"@id":"area","@name":"地域","CLASS":[{"@code":"00000","@name":"全国"},{"@code":"13000","@name":"東京都"}]},
				{"@id":"time","@name":"時間軸","CLASS":{"@code":"2020000000","@name":"2020年"}}]},
				"DATA_INF":{"VALUE":[
				{"@tab":"020","@area":"00000","@time":"2020000000","@unit":"人","$":"126146099"},
				{"@tab":"020","@area":"13000","@time":"2020000000","@unit":"人","$":"14047594"},
				{"@tab":"020","@area":"99999","@time":"2020000000","@unit":"人","$":"-"},
				{"@tab":"020","@area":"13000","@time":"2020000000","$":""}]}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type estatFake struct {
	srv    *httptest.Server
	status atomic.Int64
	seen   atomic.Value
	hits   atomic.Int64
}

func newEStatFake(t *testing.T) *estatFake {
	t.Helper()
	f := &estatFake{}
	f.status.Store(200)
	f.srv = estatServer(t, &f.status, &f.seen, &f.hits)
	return f
}

func (f *estatFake) ops(t *testing.T) map[string]Operation {
	t.Helper()
	ops, err := newEStat(estatTestKey, f.srv.URL, f.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestEStatSearchStats(t *testing.T) {
	f := newEStatFake(t)
	ops := f.ops(t)
	out, err := ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": " 人口 ", "limit": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	q := f.seen.Load().(url.Values)
	if q.Get("appId") != estatTestKey || q.Get("searchWord") != "人口" || q.Get("limit") != "3" || q.Get("lang") != "J" {
		t.Fatalf("query %v", q)
	}
	// a table without an id is skipped; titles are one line; the survey
	// month is Japanese; 0 means no month
	want := []Row{
		{Title: "人口 、世帯数", Subtitle: "国勢調査・2020年10月", Value: "0003411955", URL: "https://www.e-stat.go.jp/dbview?sid=0003411955"},
		{Title: "就業者数", Subtitle: "労働力調査・2020年1月〜2020年12月", Value: "0003412000", URL: "https://www.e-stat.go.jp/dbview?sid=0003412000"},
		{Title: "支出", Subtitle: "家計調査", Value: "0003412001", URL: "https://www.e-stat.go.jp/dbview?sid=0003412001"},
	}
	if len(out.Rows) != len(want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	// limit cuts the rows; the default limit is sent
	out, err = ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": "人口", "limit": "1"}})
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("limit 1: %+v %v", out, err)
	}
	if _, err := ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": "人口"}}); err != nil || f.seen.Load().(url.Values).Get("limit") != "10" {
		t.Fatalf("default limit: %v %v", err, f.seen.Load())
	}
	// one table is an object, not an array
	out, err = ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": "ひとつ"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "人口" {
		t.Fatalf("one table: %+v %v", out, err)
	}
	// STATUS 1: nothing matched, no rows, no error
	out, err = ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": "なし"}})
	if err != nil || len(out.Rows) != 0 {
		t.Fatalf("no data: %+v %v", out, err)
	}
	// STATUS 102: the request's fault (bad_request, not ErrBadArgs since the call was made)
	if _, err := ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": "拒否"}}); Reason(err) != ReasonBadRequest || estatBadArgs(err) {
		t.Fatalf("status 102: %s (%v)", Reason(err), err)
	}
	// STATUS 200: e-Stat's database (upstream)
	if _, err := ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": "落ちた"}}); Reason(err) != ReasonUpstream {
		t.Fatalf("status 200: %s (%v)", Reason(err), err)
	}
}

func TestEStatStatsData(t *testing.T) {
	f := newEStatFake(t)
	ops := f.ops(t)
	out, err := ops["statsData"].Do(t.Context(), Input{Args: map[string]string{"statsDataId": "0003411955", "limit": "5"}})
	if err != nil {
		t.Fatal(err)
	}
	q := f.seen.Load().(url.Values)
	if q.Get("statsDataId") != "0003411955" || q.Get("limit") != "5" || q.Get("metaGetFlg") != "Y" || q.Get("appId") != estatTestKey {
		t.Fatalf("query %v", q)
	}
	// class codes become names in the table's class order; a cell without a
	// value is skipped; an unknown code is left out of the title
	want := []Row{
		{Title: "人口・全国・2020年", Value: "126146099 人"},
		{Title: "人口・東京都・2020年", Value: "14047594 人"},
		{Title: "人口・2020年", Value: "- 人"},
	}
	if len(out.Rows) != len(want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	if out, err := ops["statsData"].Do(t.Context(), Input{Args: map[string]string{"statsDataId": "0003411955", "limit": "2"}}); err != nil || len(out.Rows) != 2 {
		t.Fatalf("limit 2: %+v %v", out, err)
	}
	// an id e-Stat does not have (STATUS 300) is bad_request
	if _, err := ops["statsData"].Do(t.Context(), Input{Args: map[string]string{"statsDataId": "0000000000"}}); Reason(err) != ReasonBadRequest || estatBadArgs(err) {
		t.Fatalf("status 300: %s (%v)", Reason(err), err)
	}
}

func TestEStatRefusesBadArgs(t *testing.T) {
	p, _ := Lookup(EStat)
	for _, a := range []map[string]string{
		{}, {"q": ""}, {"q": "  "}, {"q": strings.Repeat("あ", 101)}, {"q": "a\nb"}, {"q": "人口", "limit": "0"}, {"q": "人口", "limit": "51"},
		{"q": "人口", "statsDataId": "1"}, {"q": "https://example.com/x"},
	} {
		if _, err := p.CheckArgs("searchStats", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("searchStats %v: %s (%v)", a, Reason(err), err)
		}
	}
	for _, a := range []map[string]string{
		{}, {"statsDataId": "abc"}, {"statsDataId": "1234567"}, {"statsDataId": "0003411955", "limit": "101"}, {"statsDataId": "0003411955", "q": "x"},
	} {
		if _, err := p.CheckArgs("statsData", a); Reason(err) != ReasonBadRequest {
			t.Fatalf("statsData %v: %s (%v)", a, Reason(err), err)
		}
	}
	if _, err := p.CheckArgs("searchStats", map[string]string{"q": strings.Repeat("あ", 100), "limit": "50"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CheckArgs("statsData", map[string]string{"statsDataId": "0003411955", "limit": "100"}); err != nil {
		t.Fatal(err)
	}
	// the op refuses them too (the args reach it unchecked from a test)
	f := newEStatFake(t)
	ops := f.ops(t)
	if _, err := ops["searchStats"].Do(t.Context(), Input{Args: map[string]string{"q": ""}}); !estatBadArgs(err) {
		t.Fatalf("empty q: %v", err)
	}
	if _, err := ops["statsData"].Do(t.Context(), Input{Args: map[string]string{"statsDataId": "x"}}); !estatBadArgs(err) {
		t.Fatalf("bad id: %v", err)
	}
	if f.hits.Load() != 0 {
		t.Fatal("e-Stat was called with bad args")
	}
}

func TestEStatDefinition(t *testing.T) {
	p, ok := Lookup(EStat)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "ESTAT" {
		t.Fatalf("%+v", p)
	}
	if len(p.Hosts) != 1 || p.Hosts[0] != estatHost {
		t.Fatalf("hosts %v", p.Hosts)
	}
	for _, o := range p.Ops {
		if o.CacheTTL <= 0 || o.Write {
			t.Errorf("%s: cached read op expected: %+v", o.Name, o)
		}
	}
	for _, k := range []string{"", "short", "has space 0123456789abcdef"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: estatTestKey}); err != nil {
		t.Fatal(err)
	}
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_ESTAT_API_KEY" {
			return estatTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[EStat] {
		t.Fatalf("enabled without EXT_ESTAT_ENABLED: %v", err)
	}
	hc := NewHTTPClient(p.Hosts, DefaultTimeout)
	if _, err := hc.Get("https://example.com/rest/3.0/app/json/getStatsList"); err == nil || !strings.Contains(err.Error(), ErrEgress.Error()) {
		t.Fatalf("other host reached: %v", err)
	}
	if !strings.Contains(EStatAttribution, "e-Stat") || !strings.Contains(EStatSource, "e-Stat") {
		t.Fatal("attribution")
	}
}

// Through Service.Call: one `runs`, no coins, the second same call from the
// cache; a 5xx / 429 gives the run back and the appId is in no error or log.
func TestEStatThroughService(t *testing.T) {
	f := newEStatFake(t)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[EStat] = true
		c.Secrets = map[string]Secrets{EStat: {APIKey: estatTestKey}}
	}})
	if !e.svc.Enabled(EStat) {
		t.Fatal("not enabled")
	}
	e.svc.ops[EStat] = f.ops(t)
	e.fund(user, 100)
	args := map[string]string{"q": "人口"}
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(EStat, "searchStats", args))
		if err != nil || len(res.Output.Rows) != 3 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if f.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", f.hits.Load(), e.runs())
	}
	for _, s := range []int64{503, 429, 403} {
		f.status.Store(s)
		_, err := e.call(e.req(EStat, "statsData", map[string]string{"statsDataId": "0003411955"}))
		if Reason(err) != ReasonUpstream || strings.Contains(err.Error(), estatTestKey) {
			t.Fatalf("status %d: %v", s, err)
		}
		if e.runs() != 1 {
			t.Fatalf("status %d: runs = %d, the run was not given back", s, e.runs())
		}
	}
	if w := e.wallet(user); w.Available() != 100 {
		t.Fatalf("coins touched: %+v", w)
	}
	if strings.Contains(e.logs.String(), estatTestKey) {
		t.Fatal("the appId is in the logs")
	}
}

func TestEStatSurveyDate(t *testing.T) {
	for in, want := range map[string]string{"202010": "2020年10月", "202001-202012": "2020年1月〜2020年12月", "0": "", "": "", "202013": "", "20201": ""} {
		if got := estatSurveyDate(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// estatBadArgs: err is ErrBadArgs (refused before the call).
func estatBadArgs(err error) bool { return errors.Is(err, ErrBadArgs) }
