package extapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
)

// ncServer is a fake One Call 3.0 answering body (no live calls).
func ncServer(t *testing.T, body []byte, seen *url.Values) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/3.0/onecall" {
			http.NotFound(w, r)
			return
		}
		*seen = r.URL.Query()
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ncOps(t *testing.T, srv *httptest.Server, on bool) map[string]Operation {
	t.Helper()
	ops, err := newOpenWeatherOps(owTestKey, srv.URL, srv.Client(), on)
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

// Off by default: the op is in the registry (documented) but not built, so
// a call is ext_unavailable; EXT_OPEN_WEATHER_NOWCAST=1 builds it.
func TestOpenWeatherNowcastOffByDefault_2134(t *testing.T) {
	p, _ := Lookup(OpenWeather)
	if spec, ok := p.Op("nowcast"); !ok || spec.Write || spec.CacheTTL != 0 {
		t.Fatalf("nowcast spec %+v %v", spec, ok)
	}
	env := map[string]string{"EXT_OPEN_WEATHER_ENABLED": "1", "EXT_OPEN_WEATHER_API_KEY": owTestKey}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	call := CallRequest{Provider: OpenWeather, Op: "nowcast", Args: map[string]string{"lat": "36.56", "lng": "136.66"}, RequestID: "r1"}
	t.Setenv(EnvOpenWeatherNowcast, "")
	s := New(cfg, nil, nil, nil, nil, nil)
	if _, _, err := s.check(call); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nowcast off: %v, want ext_unavailable", err)
	}
	if _, _, err := s.check(CallRequest{Provider: OpenWeather, Op: "current", Args: call.Args, RequestID: "r2"}); err != nil {
		t.Fatalf("current with nowcast off: %v", err)
	}
	t.Setenv(EnvOpenWeatherNowcast, "1")
	if _, _, err := New(cfg, nil, nil, nil, nil, nil).check(call); err != nil {
		t.Fatalf("nowcast on: %v", err)
	}
	if _, err := p.CheckArgs("nowcast", map[string]string{"lat": "36.56"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("lat without lng: %v", err)
	}
	if ops, _ := newOpenWeather(owTestKey, "https://api.openweathermap.org", http.DefaultClient); ops["nowcast"] != nil {
		t.Fatal("built while off")
	}
}

func TestOpenWeatherNowcast_2134(t *testing.T) {
	cases := []struct {
		file  string
		item  Row
		rows  int
		first Row
	}{
		{"dry.json", Row{Title: "1時間は降らない", Value: "0 mm/h"}, 7, Row{Title: "09:00", Value: "0 mm/h"}},
		{"starts.json", Row{Title: "12分後に降り出す", Value: "0 mm/h"}, 7, Row{Title: "09:00", Value: "0.1 mm/h"}},
		{"stops.json", Row{Title: "25分後に止む", Value: "1.5 mm/h"}, 7, Row{Title: "09:00", Value: "1.5 mm/h"}},
	}
	for _, c := range cases {
		var seen url.Values
		ops := ncOps(t, ncServer(t, readTestdata(t, "openweather_nowcast", c.file), &seen), true)
		out, err := ops["nowcast"].Do(t.Context(), Input{Args: map[string]string{"lat": "36.56133", "lng": "136.65621"}})
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if seen.Get("exclude") != "current,hourly,daily,alerts" || seen.Get("lat") != "36.56" || seen.Get("lon") != "136.66" || seen.Get("appid") != owTestKey {
			t.Fatalf("%s: query %v", c.file, seen)
		}
		if out.Item == nil || *out.Item != c.item || len(out.Rows) != c.rows || out.Rows[0] != c.first {
			t.Errorf("%s: item %+v rows %+v", c.file, out.Item, out.Rows)
		}
	}
}

func TestOpenWeatherNowcastRows_2134(t *testing.T) {
	var seen url.Values
	ops := ncOps(t, ncServer(t, readTestdata(t, "openweather_nowcast", "starts.json"), &seen), true)
	out, err := ops["nowcast"].Do(t.Context(), Input{Args: map[string]string{"lat": "36.56", "lng": "136.66"}})
	if err != nil {
		t.Fatal(err)
	}
	var titles, values []string
	for _, r := range out.Rows {
		titles, values = append(titles, r.Title), append(values, r.Value)
	}
	if !slices.Equal(titles, []string{"09:00", "09:10", "09:20", "09:30", "09:40", "09:50", "10:00"}) ||
		!slices.Equal(values, []string{"0.1 mm/h", "0.3 mm/h", "0.4 mm/h", "0.5 mm/h", "0.6 mm/h", "0.7 mm/h", "0.7 mm/h"}) {
		t.Fatalf("rows %v %v", titles, values)
	}
}

func TestOpenWeatherNowcastNoData_2134(t *testing.T) {
	var seen url.Values
	ops := ncOps(t, ncServer(t, readTestdata(t, "openweather_nowcast", "none.json"), &seen), true)
	_, err := ops["nowcast"].Do(t.Context(), Input{Args: map[string]string{"lat": "36.56", "lng": "136.66"}})
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.Retryable {
		t.Fatalf("no minutely: %v", err)
	}
	if _, err := ops["nowcast"].Do(t.Context(), Input{Args: map[string]string{}}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("no lat / lng: %v", err)
	}
}
