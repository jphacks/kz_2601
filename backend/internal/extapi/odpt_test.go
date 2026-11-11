package extapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const odptTestKey = "odptTestKey0000000000000000000000"

// odptNow is the fake clock: 2026-11-12 08:20 JST.
var odptNow = time.Date(2026, 11, 12, 8, 20, 0, 0, odptJST)

// odptBody is a fake TrainInformation answer: listed and unlisted
// operators, an expired row, a row without a time, a plain-string text.
const odptBody = `[
 {"@type":"odpt:TrainInformation","dc:date":"2026-11-12T08:15:00+09:00","dct:valid":"2026-11-12T08:25:00+09:00",
  "odpt:operator":"odpt.Operator:Toei","odpt:railway":"odpt.Railway:Toei.Oedo",
  "odpt:trainInformationStatus":{"ja":"遅延","en":"Delay"},"odpt:trainInformationText":{"ja":"信号点検の影響で遅れが出ています。","en":"Delayed."}},
 {"@type":"odpt:TrainInformation","dc:date":"2026-11-12T08:15:00+09:00","dct:valid":"2026-11-12T08:25:00+09:00",
  "odpt:operator":"odpt.Operator:TokyoMetro","odpt:railway":"odpt.Railway:TokyoMetro.Tozai",
  "odpt:trainInformationText":{"ja":"現在、平常どおり運転しています。","en":"Running on schedule."}},
 {"@type":"odpt:TrainInformation","dc:date":"2026-11-12T08:14:00+09:00","dct:valid":"2026-11-12T08:24:00+09:00",
  "odpt:operator":"odpt.Operator:TokyoMetro","odpt:railway":"odpt.Railway:TokyoMetro.Ginza",
  "odpt:trainInformationText":"現在、平常どおり運転しています。"},
 {"@type":"odpt:TrainInformation","dc:date":"2026-11-12T07:00:00+09:00","dct:valid":"2026-11-12T07:05:00+09:00",
  "odpt:operator":"odpt.Operator:TokyoMetro","odpt:railway":"odpt.Railway:TokyoMetro.Hibiya",
  "odpt:trainInformationText":{"ja":"古い情報"}},
 {"@type":"odpt:TrainInformation","dc:date":"2026-11-12T08:15:00+09:00",
  "odpt:operator":"odpt.Operator:Tokyu","odpt:railway":"odpt.Railway:Tokyu.Toyoko",
  "odpt:trainInformationText":{"ja":"コンテスト限定の事業者"}},
 {"@type":"odpt:TrainInformation","dct:valid":"2026-11-12T08:25:00+09:00",
  "odpt:operator":"odpt.Operator:TWR","odpt:railway":"odpt.Railway:TWR.Rinkai",
  "odpt:trainInformationText":{"ja":"時刻が無い"}},
 {"@type":"odpt:TrainInformation","dc:date":"2026-11-12T08:10:00+09:00","dct:valid":"2026-11-12T08:25:00+09:00",
  "odpt:operator":"odpt.Operator:YokohamaMunicipal",
  "odpt:trainInformationText":{"ja":"全線で平常どおり運転しています。"}}
]`

// odptServer is a fake api.odpt.org: it records the query and answers
// status with body.
func odptServer(t *testing.T, status *atomic.Int64, seen *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.URL.Query())
		if s := status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"title":"bad key ` + r.URL.Query().Get("acl:consumerKey") + `"}`))
			return
		}
		if r.URL.Path != odptPath {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(odptBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func odptOps(t *testing.T, srv *httptest.Server) map[string]Operation {
	t.Helper()
	ops, err := newODPT(odptTestKey, srv.URL, srv.Client(), func() time.Time { return odptNow })
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestODPTTrainStatus(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	ops := odptOps(t, odptServer(t, &status, &seen))
	out, err := ops["trainStatus"].Do(t.Context(), Input{Args: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	// no filter: only the key goes out
	if q := seen.Load().(url.Values); q.Get("acl:consumerKey") != odptTestKey || len(q) != 1 {
		t.Fatalf("query %v", q)
	}
	// allow-list order; expired, timeless and unlisted-operator rows dropped
	want := []Row{
		{Title: "銀座線", Subtitle: "現在、平常どおり運転しています。", Value: "11/12 08:14 更新"},
		{Title: "東西線", Subtitle: "現在、平常どおり運転しています。", Value: "11/12 08:15 更新"},
		{Title: "大江戸線", Subtitle: "遅延: 信号点検の影響で遅れが出ています。", Value: "11/12 08:15 更新"},
		{Title: "横浜市交通局", Subtitle: "全線で平常どおり運転しています。", Value: "11/12 08:10 更新"},
	}
	if len(out.Rows) != len(want) || out.Item != nil || out.Usage.Priced {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
}

func TestODPTTrainStatusFilter(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	ops := odptOps(t, odptServer(t, &status, &seen))
	for _, c := range []struct {
		args       map[string]string
		key, value string
	}{
		{map[string]string{"operator": "odpt.Operator:Toei"}, "odpt:operator", "odpt.Operator:Toei"},
		{map[string]string{"railway": "odpt.Railway:TokyoMetro.Ginza"}, "odpt:railway", "odpt.Railway:TokyoMetro.Ginza"},
		{map[string]string{"operator": "odpt.Operator:TokyoMetro", "railway": "odpt.Railway:TokyoMetro.Ginza"}, "odpt:railway", "odpt.Railway:TokyoMetro.Ginza"},
	} {
		if _, err := ops["trainStatus"].Do(t.Context(), Input{Args: c.args}); err != nil {
			t.Fatalf("%v: %v", c.args, err)
		}
		q := seen.Load().(url.Values)
		if q.Get(c.key) != c.value || len(q) != 2 {
			t.Fatalf("%v: query %v", c.args, q)
		}
	}
}

func TestODPTRefusesBadArgs(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	ops := odptOps(t, odptServer(t, &status, &seen))
	p, _ := Lookup(ODPT)
	for _, a := range []map[string]string{
		{"operator": "odpt.Operator:Tokyu"},                                            // contest-only license
		{"operator": "odpt.Operator:JR-East"},                                          // contest-only license
		{"operator": "TokyoMetro"},                                                     // not an ODPT id
		{"railway": "odpt.Railway:JR-East.Yamanote"},                                   // not listed
		{"railway": "odpt.Railway:TokyoMetro.Ginza&x=1"},                               // no smuggled query
		{"operator": "odpt.Operator:Toei", "railway": "odpt.Railway:TokyoMetro.Ginza"}, // not its line
		{"railway": "https://api.odpt.org/api/v4/odpt:Train"},                          // a URL
	} {
		if _, err := ops["trainStatus"].Do(t.Context(), Input{Args: a}); Reason(err) != ReasonBadRequest {
			t.Fatalf("%v: %s (%v)", a, Reason(err), err)
		}
		if _, err := p.CheckArgs("trainStatus", a); err == nil {
			t.Fatalf("CheckArgs accepted %v", a)
		}
	}
	if _, err := p.CheckArgs("trainStatus", map[string]string{"line": "x"}); err == nil {
		t.Fatal("unknown argument accepted")
	}
	if seen.Load() != nil {
		t.Fatal("a bad argument reached the API")
	}
}

func TestODPTUpstreamErrors(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	ops := odptOps(t, odptServer(t, &status, &seen))
	// upstream errors never carry the key
	for _, s := range []int64{401, 403, 429, 503} {
		status.Store(s)
		_, err := ops["trainStatus"].Do(t.Context(), Input{Args: map[string]string{}})
		if err == nil || Reason(err) != ReasonUpstream || strings.Contains(err.Error(), odptTestKey) {
			t.Fatalf("%d: %s %v", s, Reason(err), err)
		}
	}
}

func TestODPTClip(t *testing.T) {
	long := strings.Repeat("あ", odptMaxRunes+5)
	if got := odptClip(long); []rune(got)[odptMaxRunes] != '…' || len([]rune(got)) != odptMaxRunes+1 {
		t.Fatalf("clip %q", got)
	}
}

// Every listed railway belongs to exactly one operator and every id has the
// ODPT shape.
func TestODPTAllowList(t *testing.T) {
	seen := map[string]bool{}
	for _, o := range odptOperators {
		if !strings.HasPrefix(o.id, "odpt.Operator:") || o.name == "" || len(o.lines) == 0 || seen[o.id] {
			t.Fatalf("operator %+v", o)
		}
		seen[o.id] = true
		suffix := strings.TrimPrefix(o.id, "odpt.Operator:")
		for _, l := range o.lines {
			if !strings.HasPrefix(l.id, "odpt.Railway:"+suffix+".") || l.name == "" || seen[l.id] {
				t.Fatalf("line %+v of %s", l, o.id)
			}
			seen[l.id] = true
		}
	}
	for _, rejected := range []string{"JR-East", "Tokyu", "Tobu", "Keio", "Seibu", "Keikyu"} {
		if odptOperatorIndex("odpt.Operator:"+rejected) >= 0 {
			t.Fatalf("%s is contest-only and must not be listed", rejected)
		}
	}
}

func TestODPTDefinition(t *testing.T) {
	p, ok := Lookup(ODPT)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "ODPT" {
		t.Fatalf("%+v", p)
	}
	if len(p.Hosts) != 1 || p.Hosts[0] != odptHost {
		t.Fatalf("hosts %v", p.Hosts)
	}
	// dynamic data: cached briefly (ガイドライン 2.1.2)
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL <= 0 || o.CacheTTL > 2*time.Minute {
			t.Fatalf("op %s cache %v", o.Name, o.CacheTTL)
		}
	}
	for _, k := range []string{"", "short", "has space 0123456789abcdef"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: odptTestKey}); err != nil {
		t.Fatal(err)
	}
	// default off
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_ODPT_API_KEY" {
			return odptTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[ODPT] {
		t.Fatalf("enabled without EXT_ODPT_ENABLED: %v", err)
	}
	// only api.odpt.org
	hc := NewHTTPClient(p.Hosts, DefaultTimeout)
	if _, err := hc.Get("https://example.com" + odptPath); err == nil || !strings.Contains(err.Error(), ErrEgress.Error()) {
		t.Fatalf("other host reached: %v", err)
	}
}
