package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// omServer is a stand-in for api.open-meteo.com (no live calls).
type omServer struct {
	*httptest.Server
	hits   atomic.Int64
	last   atomic.Value // the last request's query
	status int
	body   string
}

const omForecastBody = `{"latitude":36.5625,"longitude":136.6562,"utc_offset_seconds":32400,"timezone":"Asia/Tokyo",
"daily":{"time":["2026-11-14","2026-11-15","2026-11-16","2026-11-17","not a day","2026-11-19"],
"weather_code":[3,61,0,null,1,95],
"temperature_2m_max":[15.26,12.0,18.5,10,9,-0.04],
"temperature_2m_min":[8.04,6.5,null,3,2,-5.5],
"precipitation_probability_max":[40,85,0,null,10]}}`

const omCurrentBody = `{"current":{"time":"2026-11-14T12:00","temperature_2m":12.34,"apparent_temperature":10.26,"relative_humidity_2m":61.4,
"weather_code":2,"wind_speed_10m":3.26}}`

func newOMServer(t *testing.T) *omServer {
	t.Helper()
	s := &omServer{status: http.StatusOK, body: omForecastBody}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.last.Store(r.URL.Query())
		if r.URL.Path != "/v1/forecast" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *omServer) query() url.Values { v, _ := s.last.Load().(url.Values); return v }

func (s *omServer) ops() map[string]Operation { return newOpenMeteo(s.URL, s.Client()) }

var omKanazawa = map[string]string{"lat": "36.5613", "lng": "136.6562"}

func TestOpenMeteoRegistered(t *testing.T) {
	p, ok := Lookup(OpenMeteo)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.GlobalDaily != openMeteoGlobalDaily ||
		!slices.Equal(p.Hosts, []string{"https://api.open-meteo.com"}) {
		t.Fatalf("openMeteo %+v", p)
	}
	if !slices.Contains(ReviewBuildFeatures(), "externalCall.openMeteo") {
		t.Fatal("a provider without OAuth belongs to the review build")
	}
	for _, op := range p.Ops {
		if op.Write || op.CacheTTL != 10*time.Minute {
			t.Fatalf("%s: read ops, cached 10 minutes", op.Name)
		}
	}
	cfg, err := ConfigFromEnv(func(k string) string { return map[string]string{"EXT_OPEN_METEO_ENABLED": "1"}[k] })
	if err != nil || !cfg.Enabled[OpenMeteo] || cfg.Limits[OpenMeteo].GlobalDaily != openMeteoGlobalDaily {
		t.Fatalf("env: %v %v %+v", cfg.Enabled, err, cfg.Limits[OpenMeteo])
	}
	// off by default (the free API is non-commercial only; the owner decides)
	if cfg, _ := ConfigFromEnv(func(string) string { return "" }); cfg.Enabled[OpenMeteo] {
		t.Fatal("on by default")
	}
	if !strings.Contains(OpenMeteoAttribution, "Open-Meteo") {
		t.Fatal(OpenMeteoAttribution)
	}
}

// forecast: one row a day in JST; days without both temperatures or with a
// bad date are skipped; a missing rain chance leaves the value shorter.
func TestOpenMeteoForecast(t *testing.T) {
	s := newOMServer(t)
	args := map[string]string{"lat": "36.5613", "lng": "136.6562", "days": "5"}
	out, err := s.ops()["forecast"].Do(context.Background(), Input{Args: args})
	if err != nil {
		t.Fatal(err)
	}
	q := s.query()
	// rounded to 2 places; JST; the asked days; the daily columns
	if q.Get("latitude") != "36.56" || q.Get("longitude") != "136.66" || q.Get("timezone") != "Asia/Tokyo" || q.Get("forecast_days") != "5" ||
		q.Get("daily") != "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max" || q.Has("apikey") {
		t.Fatalf("query %v", q)
	}
	want := []Row{
		{Title: "11/14(土)", Subtitle: "曇り", Value: "最高 15.3°C / 最低 8°C・降水 40%"},
		{Title: "11/15(日)", Subtitle: "弱い雨", Value: "最高 12°C / 最低 6.5°C・降水 85%"},
		{Title: "11/17(火)", Value: "最高 10°C / 最低 3°C"},
		{Title: "11/19(木)", Subtitle: "雷雨", Value: "最高 0°C / 最低 -5.5°C"},
	}
	if !slices.Equal(out.Rows, want) || out.Item != nil {
		t.Fatalf("rows %+v", out.Rows)
	}
	// days caps the rows too
	out, err = s.ops()["forecast"].Do(context.Background(), Input{Args: map[string]string{"lat": "36.5613", "lng": "136.6562", "days": "1"}})
	if err != nil || len(out.Rows) != 1 {
		t.Fatalf("days=1: %+v %v", out.Rows, err)
	}
	if _, err := s.ops()["forecast"].Do(context.Background(), Input{Args: map[string]string{"lat": "36.5613", "lng": "136.6562", "days": "17"}}); Reason(err) != ReasonBadRequest {
		t.Fatalf("days=17: %v", err)
	}
}

func TestOpenMeteoCurrent(t *testing.T) {
	s := newOMServer(t)
	s.body = omCurrentBody
	out, err := s.ops()["current"].Do(context.Background(), Input{Args: omKanazawa})
	if err != nil {
		t.Fatal(err)
	}
	q := s.query()
	if q.Get("current") != "temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m" || q.Get("wind_speed_unit") != "ms" || q.Has("daily") {
		t.Fatalf("query %v", q)
	}
	want := Row{Title: "一部曇り", Subtitle: "体感 10.3°C・湿度 61%・風 3.3 m/s", Value: "12.3°C"}
	if out.Item == nil || *out.Item != want || len(out.Rows) != 0 {
		t.Fatalf("item %+v", out.Item)
	}
	// an unknown code shows its number; no code is a plain title
	s.body = `{"current":{"temperature_2m":5,"weather_code":42}}`
	if out, err := s.ops()["current"].Do(context.Background(), Input{Args: omKanazawa}); err != nil || *out.Item != (Row{Title: "天気コード 42", Value: "5°C"}) {
		t.Fatalf("unknown code: %+v %v", out.Item, err)
	}
	s.body = `{"current":{"temperature_2m":5}}`
	if out, err := s.ops()["current"].Do(context.Background(), Input{Args: omKanazawa}); err != nil || out.Item.Title != "今の天気" {
		t.Fatalf("no code: %+v %v", out.Item, err)
	}
}

func TestOpenMeteoCheck(t *testing.T) {
	p, _ := Lookup(OpenMeteo)
	for _, c := range []struct {
		op   string
		args map[string]string
		ok   bool
	}{
		{"forecast", omKanazawa, true},
		{"forecast", map[string]string{"lat": "-90", "lng": "180", "days": "16"}, true},
		{"forecast", nil, false},
		{"forecast", map[string]string{"lat": "36.5"}, false},
		{"forecast", map[string]string{"lat": "91", "lng": "136"}, false},
		{"forecast", map[string]string{"lat": "1e1", "lng": "136"}, false},
		{"forecast", map[string]string{"lat": "36.5", "lng": "136", "days": "0"}, false},
		{"forecast", map[string]string{"lat": "36.5", "lng": "136", "days": "17"}, false},
		{"forecast", map[string]string{"lat": "36.5", "lng": "136", "hourly": "1"}, false},
		{"current", omKanazawa, true},
		{"current", map[string]string{"lat": "36.5", "lng": "136", "days": "1"}, false},
		{"current", nil, false},
	} {
		_, err := p.CheckArgs(c.op, c.args)
		if (err == nil) != c.ok || (err != nil && !errors.Is(err, ErrUnknown)) {
			t.Errorf("%s %v: %v", c.op, c.args, err)
		}
	}
}

func TestOpenMeteoFailures(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(error) bool
	}{
		{http.StatusBadRequest, `{"error":true,"reason":"Latitude must be in range of -90 to 90°."}`, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && !pe.Retryable && !pe.Billable && Reason(err) == ReasonUpstream
		}},
		{http.StatusTooManyRequests, `{"error":true,"reason":"Minutely API request limit exceeded."}`, func(err error) bool {
			var pe *ProviderError
			return errors.As(err, &pe) && pe.Retryable && pe.Status == 429
		}},
		{http.StatusServiceUnavailable, ``, func(err error) bool { var pe *ProviderError; return errors.As(err, &pe) && pe.Retryable }},
		{http.StatusOK, `not json`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"daily":{"time":["2026-11-14"],"temperature_2m_max":[null],"temperature_2m_min":[1]}}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
		{http.StatusOK, `{"daily":{}}`, func(err error) bool { return Reason(err) == ReasonUpstream }},
	}
	for _, c := range cases {
		s := newOMServer(t)
		s.status, s.body = c.status, c.body
		_, err := s.ops()["forecast"].Do(context.Background(), Input{Args: omKanazawa})
		if err == nil || !c.check(err) {
			t.Errorf("%d %.40q: %v", c.status, c.body, err)
		}
	}
	s := newOMServer(t)
	s.body = `{"current":{"temperature_2m":null}}`
	if _, err := s.ops()["current"].Do(context.Background(), Input{Args: omKanazawa}); Reason(err) != ReasonUpstream {
		t.Fatalf("no temperature: %v", err)
	}
}

// The real client reaches api.open-meteo.com only.
func TestOpenMeteoEgress(t *testing.T) {
	s := newOMServer(t)
	real := newOpenMeteo(s.URL, NewHTTPClient([]string{openMeteoHost}, time.Second))
	if _, err := real["current"].Do(context.Background(), Input{Args: omKanazawa}); !errors.Is(err, ErrEgress) || s.hits.Load() != 0 {
		t.Fatalf("local server reached: %v hits=%d", err, s.hits.Load())
	}
}

// Through the Service: the same place is answered from the cache for 10
// minutes (one `runs`), then asked again; another place is another call.
func TestOpenMeteoCachedCall(t *testing.T) {
	s := newOMServer(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[OpenMeteo] = true }})
	if !e.svc.Enabled(OpenMeteo) {
		t.Fatal("openMeteo not on")
	}
	e.svc.ops[OpenMeteo] = s.ops()
	for i := 0; i < 2; i++ {
		res, err := e.call(e.req(OpenMeteo, "forecast", omKanazawa))
		if err != nil || len(res.Output.Rows) != 4 || res.Charge.Coins != 0 {
			t.Fatalf("call %d: %+v %v", i, res, err)
		}
	}
	if s.hits.Load() != 1 || e.runs() != 1 {
		t.Fatalf("cached: hits=%d runs=%d", s.hits.Load(), e.runs())
	}
	// the same place written with more decimals is the same cache key? no:
	// the key is the arguments as given; it is still one call to the API
	// per distinct argument set
	if _, err := e.call(e.req(OpenMeteo, "forecast", map[string]string{"lat": "35.68", "lng": "139.77"})); err != nil || s.hits.Load() != 2 || e.runs() != 2 {
		t.Fatalf("other place: %v hits=%d runs=%d", err, s.hits.Load(), e.runs())
	}
	e.c.t = e.c.t.Add(10 * time.Minute)
	if _, err := e.call(e.req(OpenMeteo, "forecast", omKanazawa)); err != nil || s.hits.Load() != 3 {
		t.Fatalf("expired: %v hits=%d", err, s.hits.Load())
	}
	// bad arguments count nothing and never reach the provider
	if _, err := e.call(e.req(OpenMeteo, "forecast", map[string]string{"lat": "x", "lng": "1"})); Reason(err) != ReasonBadRequest || s.hits.Load() != 3 {
		t.Fatalf("bad args: %v hits=%d", err, s.hits.Load())
	}
}
