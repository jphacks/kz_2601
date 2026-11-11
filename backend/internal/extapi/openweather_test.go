package extapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const owTestKey = "test0000000000000000000000000000"

// owServer is a fake OpenWeatherMap: it checks the query and answers
// status with body.
func owServer(t *testing.T, status *atomic.Int64, seen *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.URL.Query())
		if s := status.Load(); s != 200 {
			w.WriteHeader(int(s))
			_, _ = w.Write([]byte(`{"cod":401,"message":"bad key ` + r.URL.Query().Get("appid") + `"}`))
			return
		}
		switch r.URL.Path {
		case "/data/2.5/weather":
			_, _ = w.Write([]byte(`{"weather":[{"description":"曇りがち"}],"main":{"temp":12.34,"feels_like":-0.04,"humidity":61.6},"wind":{"speed":3.21},"name":"Kanazawa"}`))
		case "/data/2.5/forecast":
			_, _ = w.Write([]byte(`{"city":{"timezone":32400},"list":[
				{"dt":1762819200,"main":{"temp":10},"weather":[{"description":"小雨"}],"pop":0.42},
				{"dt":1762830000,"main":{"temp":9.5},"weather":[],"pop":0},
				{"dt":0,"main":{"temp":1}},
				{"dt":1762840800,"main":{}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func owOps(t *testing.T, srv *httptest.Server) map[string]Operation {
	t.Helper()
	ops, err := newOpenWeather(owTestKey, srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestOpenWeatherCurrent(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	ops := owOps(t, owServer(t, &status, &seen))
	out, err := ops["current"].Do(t.Context(), Input{Args: map[string]string{"lat": "36.56133", "lng": "136.65621"}})
	if err != nil {
		t.Fatal(err)
	}
	q := seen.Load().(url.Values)
	// rounded to 2 places; the key, metric, Japanese
	if q.Get("lat") != "36.56" || q.Get("lon") != "136.66" || q.Get("appid") != owTestKey || q.Get("units") != "metric" || q.Get("lang") != "ja" {
		t.Fatalf("query %v", q)
	}
	want := Row{Title: "曇りがち", Subtitle: "体感 0°C・湿度 62%・風 3.2 m/s", Value: "12.3°C"}
	if out.Item == nil || *out.Item != want || len(out.Rows) != 0 || out.Usage.Priced {
		t.Fatalf("output %+v %+v", out, out.Item)
	}
}

func TestOpenWeatherForecast(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	ops := owOps(t, owServer(t, &status, &seen))
	out, err := ops["forecast"].Do(t.Context(), Input{Args: map[string]string{"lat": "35", "lng": "139", "limit": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if q := seen.Load().(url.Values); q.Get("cnt") != "3" {
		t.Fatalf("cnt %v", q)
	}
	// rows without a time or a temperature are skipped; the city's zone (JST)
	want := []Row{
		{Title: "11/11 09:00", Subtitle: "小雨", Value: "10°C・降水 42%"},
		{Title: "11/11 12:00", Value: "9.5°C・降水 0%"},
	}
	if len(out.Rows) != len(want) {
		t.Fatalf("rows %+v", out.Rows)
	}
	for i := range want {
		if out.Rows[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, out.Rows[i], want[i])
		}
	}
	out, err = ops["forecast"].Do(t.Context(), Input{Args: map[string]string{"lat": "35", "lng": "139"}})
	if err != nil || seen.Load().(url.Values).Get("cnt") != "8" || len(out.Rows) != 2 {
		t.Fatalf("default limit: %+v %v", out, err)
	}
}

func TestOpenWeatherRefusesBadArgs(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	ops := owOps(t, owServer(t, &status, &seen))
	for _, a := range []map[string]string{
		{},
		{"lat": "35"},
		{"lat": "95", "lng": "139"},
		{"lat": "35", "lng": "139e0"},
		{"lat": "35", "lng": "139", "limit": "41"},
		{"lat": "35", "lng": "139", "limit": "0"},
	} {
		_, err := ops["forecast"].Do(t.Context(), Input{Args: a})
		if Reason(err) != ReasonBadRequest {
			t.Fatalf("%v: %s (%v)", a, Reason(err), err)
		}
	}
	if seen.Load() != nil {
		t.Fatal("a bad argument reached the API")
	}
}

func TestOpenWeatherDefinition(t *testing.T) {
	p, ok := Lookup(OpenWeather)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "OPEN_WEATHER" {
		t.Fatalf("%+v", p)
	}
	if len(p.Hosts) != 1 || p.Hosts[0] != openWeatherHost {
		t.Fatalf("hosts %v", p.Hosts)
	}
	// no key (or not a key): the provider stays off
	for _, k := range []string{"", "short", "has space 0123456789abcdef"} {
		if _, err := p.New(Secrets{APIKey: k}); err == nil {
			t.Fatalf("key %q accepted", k)
		}
	}
	if _, err := p.New(Secrets{APIKey: owTestKey}); err != nil {
		t.Fatal(err)
	}
	// default off
	c, err := ConfigFromEnv(func(k string) string {
		if k == "EXT_OPEN_WEATHER_API_KEY" {
			return owTestKey
		}
		return ""
	})
	if err != nil || c.Enabled[OpenWeather] {
		t.Fatalf("enabled without EXT_OPEN_WEATHER_ENABLED: %v", err)
	}
	// only api.openweathermap.org
	hc := NewHTTPClient(p.Hosts, DefaultTimeout)
	if _, err := hc.Get("https://example.com/data/2.5/weather"); err == nil || !strings.Contains(err.Error(), ErrEgress.Error()) {
		t.Fatalf("other host reached: %v", err)
	}
}

// Through Service.Call: one `runs`, no coins; a 5xx gives the run back and
// the key is in no error or log line.
func TestOpenWeatherThroughService(t *testing.T) {
	var status atomic.Int64
	status.Store(200)
	var seen atomic.Value
	srv := owServer(t, &status, &seen)
	e := newEnv(t, opts{cfg: func(c *Config) {
		c.Enabled[OpenWeather] = true
		c.Secrets = map[string]Secrets{OpenWeather: {APIKey: owTestKey}}
	}})
	if !e.svc.Enabled(OpenWeather) {
		t.Fatal("not enabled")
	}
	e.svc.ops[OpenWeather] = owOps(t, srv)
	e.fund(user, 100)
	res, err := e.call(e.req(OpenWeather, "current", map[string]string{"lat": "36.5", "lng": "136.6"}))
	if err != nil || res.Output.Item == nil || res.Charge.Coins != 0 || e.runs() != 1 {
		t.Fatalf("%+v %v runs=%d", res, err, e.runs())
	}
	for _, s := range []int64{503, 429, 401} {
		status.Store(s)
		_, err = e.call(e.req(OpenWeather, "current", map[string]string{"lat": "36.5", "lng": "136.6"}))
		if Reason(err) != ReasonUpstream || strings.Contains(err.Error(), owTestKey) {
			t.Fatalf("status %d: %v", s, err)
		}
		if e.runs() != 1 {
			t.Fatalf("status %d: runs = %d, the run was not given back", s, e.runs())
		}
	}
	if w := e.wallet(user); w.Available() != 100 {
		t.Fatalf("coins touched: %+v", w)
	}
	if strings.Contains(e.logs.String(), owTestKey) {
		t.Fatal("the key is in the logs")
	}
	// disabled by EXT_DISABLED
	e2 := newEnv(t, opts{cfg: func(c *Config) {
		c.Disabled = true
		c.Enabled[OpenWeather] = true
		c.Secrets = map[string]Secrets{OpenWeather: {APIKey: owTestKey}}
	}})
	if _, err := e2.call(e2.req(OpenWeather, "current", map[string]string{"lat": "1", "lng": "1"})); Reason(err) != ReasonUnavailable {
		t.Fatalf("EXT_DISABLED: %v", err)
	}
}
