package extapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// OpenWeatherMap (#1498): the weather now and the 3-hourly forecast at a
// lat / lng, from the free Current Weather and 5 day / 3 hour Forecast APIs
// (api.openweathermap.org/data/2.5). Free: a call is one `runs`, no coins.
//
//	current(lat, lng)                Item: description / feels, humidity, wind / temperature
//	forecast(lat, lng, limit, uv)    Rows (3-hourly, limit 1-40, default 8 = 24 h)
//	airQuality(lat, lng)             Item: AQI 1-5 label (目安) / PM2.5, PM10, O₃, NO₂ / "AQI n/5"
//	airForecast(lat, lng, limit)     Rows (hourly, limit 1-96, default 24): time (JST) / PM2.5, PM10 / "AQI n/5 label"
//	nowcast(lat, lng)                Item: "12分後に降り出す" ... / Rows: 10-minute precipitation (openweather_nowcast.go)
//
// forecast's uv (#1658) asks for the UV index, which only One Call 3.0
// (/data/3.0/onecall, same host and key) carries: with uv=1 the rows come
// from One Call's hourly list and their subtitle ends with "UV n <段階>
// (目安)". It is one request either way — never the 5 day API and One Call
// together — because a One Call request is priced per call. Default off, so
// the answer and the cost of a forecast without uv do not change.
//
// airQuality and airForecast (#1656) are the Air Pollution API
// (/data/2.5/air_pollution and /air_pollution/forecast, same host and key).
// The level is a guide only, never a health judgement. The forecast has no
// time zone, so its times are shown in JST (the app is for Japan).
//
// lat / lng are rounded to 2 places (about 1 km) before they leave the
// runner. The key (EXT_OPEN_WEATHER_API_KEY, Secret Manager) goes in the
// query as the API wants; errors never carry the URL (keyed.go).
//
// Terms: the data is ODbL / CC BY-SA 4.0 and the "OpenWeather" credit
// (OpenWeatherAttribution) must be shown with it. Caching is allowed; the
// shared answer cache is the foundation's (not here).
const (
	OpenWeather = "openWeather"
	// OpenWeatherAttribution is the credit shown under the rows.
	OpenWeatherAttribution = "Weather data © OpenWeather (ODbL)"

	openWeatherHost = "https://api.openweathermap.org"
)

var owKeyRe = regexp.MustCompile(`^[A-Za-z0-9]{16,64}$`)

func init() {
	register(Provider{ID: OpenWeather, Billing: BillingFree,
		Ops: []OpSpec{
			{Name: "current", Args: []string{"lat", "lng"}, Check: checkOWLatLng},
			{Name: "forecast", Args: []string{"lat", "lng", "limit", "uv"}, Check: checkOWForecast},
			{Name: "airQuality", Args: []string{"lat", "lng"}, Check: checkOWLatLng},
			{Name: "airForecast", Args: []string{"lat", "lng", "limit"}, Check: checkOWAirForecast},
			{Name: "nowcast", Args: []string{"lat", "lng"}, Check: checkOWLatLng},
		},
		Hosts: []string{openWeatherHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newOpenWeatherOps(sec.APIKey, openWeatherHost, NewHTTPClient([]string{openWeatherHost}, DefaultTimeout),
				flag(os.Getenv(EnvOpenWeatherNowcast)))
		}})
}

// checkOWLatLng rejects a call without a valid lat / lng at CheckArgs, before
// it counts against any limit.
func checkOWLatLng(args map[string]string) error {
	_, _, ok, err := argLatLng(args, 2)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("lat and lng are required")
	}
	return nil
}

func checkOWForecast(args map[string]string) error {
	if err := checkOWLatLng(args); err != nil {
		return err
	}
	if _, err := argInt(args, "limit", owForecastDefault, 1, owForecastMax); err != nil {
		return err
	}
	_, err := argOWUV(args)
	return err
}

func checkOWAirForecast(args map[string]string) error {
	if err := checkOWLatLng(args); err != nil {
		return err
	}
	_, err := argInt(args, "limit", owAirForecastDefault, 1, owAirForecastMax)
	return err
}

// The air forecast is hourly for about 4 days.
const (
	owAirForecastDefault = 24
	owAirForecastMax     = 96
)

// The 5 day / 3 hour API has 40 steps; One Call's hourly list has 48, so the
// same limit fits both.
const (
	owForecastDefault = 8
	owForecastMax     = 40
)

// argOWUV reads forecast's uv: 1 / true / on asks for the UV index (One
// Call 3.0), empty / 0 / false / off is the plain forecast. Anything else
// is bad_request.
func argOWUV(args map[string]string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(args["uv"])) {
	case "", "0", "false", "off":
		return false, nil
	case "1", "true", "on":
		return true, nil
	}
	return false, fmt.Errorf("%w: uv must be 1 or 0", ErrUnknown)
}

// newOpenWeather builds the ops against base (the API, or a test server),
// nowcast left off.
func newOpenWeather(key, base string, hc *http.Client) (map[string]Operation, error) {
	return newOpenWeatherOps(key, base, hc, false)
}

// newOpenWeatherOps also builds nowcast when it is switched on
// (EnvOpenWeatherNowcast); off, the op is not built and a call to it is
// ext_unavailable (Service.check).
func newOpenWeatherOps(key, base string, hc *http.Client, nowcast bool) (map[string]Operation, error) {
	if !owKeyRe.MatchString(key) {
		return nil, errors.New("EXT_OPEN_WEATHER_API_KEY is not set or not a key")
	}
	w := &openWeather{key: key, base: base, hc: hc}
	ops := map[string]Operation{
		"current":     OperationFunc(w.current),
		"forecast":    OperationFunc(w.forecast),
		"airQuality":  OperationFunc(w.airQuality),
		"airForecast": OperationFunc(w.airForecast),
	}
	if nowcast {
		ops["nowcast"] = OperationFunc(w.nowcast)
	}
	return ops, nil
}

type openWeather struct {
	key  string
	base string
	hc   *http.Client
}

type owCondition struct {
	Description string `json:"description"`
}

type owMain struct {
	Temp      *float64 `json:"temp"`
	FeelsLike *float64 `json:"feels_like"`
	Humidity  *float64 `json:"humidity"`
}

type owCurrent struct {
	Weather []owCondition `json:"weather"`
	Main    owMain        `json:"main"`
	Wind    struct {
		Speed *float64 `json:"speed"`
	} `json:"wind"`
}

type owForecast struct {
	List []struct {
		Dt      int64         `json:"dt"`
		Main    owMain        `json:"main"`
		Weather []owCondition `json:"weather"`
		Pop     *float64      `json:"pop"`
	} `json:"list"`
	City struct {
		Timezone int `json:"timezone"` // seconds from UTC
	} `json:"city"`
}

// owOneCall is One Call 3.0's hourly list: the same values as the 5 day API
// plus uvi (the UV index of that hour).
type owOneCall struct {
	TimezoneOffset int `json:"timezone_offset"` // seconds from UTC
	Hourly         []struct {
		Dt      int64         `json:"dt"`
		Temp    *float64      `json:"temp"`
		Weather []owCondition `json:"weather"`
		Pop     *float64      `json:"pop"`
		UVI     *float64      `json:"uvi"`
	} `json:"hourly"`
}

// owStep is one step of a forecast, from either API (uvi only from One Call).
type owStep struct {
	dt      int64
	temp    *float64
	pop     *float64
	uvi     *float64
	weather []owCondition
}

// get calls path with the rounded lat / lng.
func (w *openWeather) get(ctx context.Context, path string, args map[string]string, extra url.Values, out any) error {
	lat, lng, ok, err := argLatLng(args, 2)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: lat and lng are required", ErrBadArgs)
	}
	q := url.Values{"lat": {formatCoord(lat)}, "lon": {formatCoord(lng)}, "units": {"metric"}, "lang": {"ja"}, "appid": {w.key}}
	for k, v := range extra {
		q[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return errors.New("openweather: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	return sendJSON(ctx, w.hc, req, out)
}

func (w *openWeather) current(ctx context.Context, in Input) (Output, error) {
	var c owCurrent
	if err := w.get(ctx, "/data/2.5/weather", in.Args, nil, &c); err != nil {
		return Output{}, err
	}
	if c.Main.Temp == nil {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("openweather: no temperature")}
	}
	var sub []string
	if c.Main.FeelsLike != nil {
		sub = append(sub, "体感 "+celsius(*c.Main.FeelsLike))
	}
	if c.Main.Humidity != nil {
		sub = append(sub, "湿度 "+strconv.Itoa(int(math.Round(*c.Main.Humidity)))+"%")
	}
	if c.Wind.Speed != nil {
		sub = append(sub, "風 "+strconv.FormatFloat(*c.Wind.Speed, 'f', 1, 64)+" m/s")
	}
	return Output{Item: &Row{Title: describe(c.Weather), Subtitle: strings.Join(sub, "・"), Value: celsius(*c.Main.Temp)}}, nil
}

func (w *openWeather) forecast(ctx context.Context, in Input) (Output, error) {
	limit, err := argInt(in.Args, "limit", owForecastDefault, 1, owForecastMax)
	if err != nil {
		return Output{}, err
	}
	uv, err := argOWUV(in.Args)
	if err != nil {
		return Output{}, err
	}
	steps, tz, err := w.forecastSteps(ctx, in, limit, uv)
	if err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, min(limit, len(steps)))
	for _, it := range steps {
		if len(rows) == limit {
			break
		}
		if it.temp == nil || it.dt <= 0 {
			continue
		}
		v := celsius(*it.temp)
		if it.pop != nil {
			v += "・降水 " + strconv.Itoa(int(math.Round(*it.pop*100))) + "%"
		}
		rows = append(rows, Row{
			Title:    time.Unix(it.dt, 0).In(tz).Format("1/2 15:04"),
			Subtitle: joinDots(describe(it.weather), uvIndex(it.uvi)),
			Value:    v,
		})
	}
	if len(rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("openweather: no forecast rows")}
	}
	return Output{Rows: rows}, nil
}

// forecastSteps reads the forecast and its time zone: the 5 day / 3 hour API
// (3-hourly), or One Call 3.0's hourly list when the UV index is asked for.
// Only one of them is called.
func (w *openWeather) forecastSteps(ctx context.Context, in Input, limit int, uv bool) ([]owStep, *time.Location, error) {
	if uv {
		var o owOneCall
		if err := w.get(ctx, "/data/3.0/onecall", in.Args, url.Values{"exclude": {"current,minutely,daily,alerts"}}, &o); err != nil {
			return nil, nil, err
		}
		steps := make([]owStep, 0, len(o.Hourly))
		for _, it := range o.Hourly {
			steps = append(steps, owStep{dt: it.Dt, temp: it.Temp, pop: it.Pop, uvi: it.UVI, weather: it.Weather})
		}
		return steps, time.FixedZone("", o.TimezoneOffset), nil
	}
	var f owForecast
	if err := w.get(ctx, "/data/2.5/forecast", in.Args, url.Values{"cnt": {strconv.Itoa(limit)}}, &f); err != nil {
		return nil, nil, err
	}
	steps := make([]owStep, 0, len(f.List))
	for _, it := range f.List {
		steps = append(steps, owStep{dt: it.Dt, temp: it.Main.Temp, pop: it.Pop, weather: it.Weather})
	}
	return steps, time.FixedZone("", f.City.Timezone), nil
}

// owUVBand is the UV index band (WHO's 1-2 / 3-5 / 6-7 / 8-10 / 11+). The
// band is a guide to cover up, never a health judgement.
func owUVBand(v float64) string {
	switch {
	case v < 3:
		return "弱い"
	case v < 6:
		return "中程度"
	case v < 8:
		return "強い"
	case v < 11:
		return "非常に強い"
	}
	return "極端に強い"
}

// uvIndex writes "UV 6.8 強い(目安)"; a missing or negative uvi is "".
func uvIndex(v *float64) string {
	if v == nil || *v < 0 {
		return ""
	}
	u := math.Round(*v*10) / 10
	return "UV " + strconv.FormatFloat(u, 'f', -1, 64) + " " + owUVBand(u) + "(目安)"
}

// joinDots joins the parts that are not empty with "・".
func joinDots(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "・")
}

type owAirPollution struct {
	List []struct {
		Dt   int64 `json:"dt"`
		Main struct {
			AQI int `json:"aqi"`
		} `json:"main"`
		Components struct {
			PM25 *float64 `json:"pm2_5"`
			PM10 *float64 `json:"pm10"`
			O3   *float64 `json:"o3"`
			NO2  *float64 `json:"no2"`
		} `json:"components"`
	} `json:"list"`
}

// owAQILabels are OpenWeather's AQI 1-5 (Good … Very Poor).
var owAQILabels = [...]string{1: "良い", 2: "まあまあ", 3: "ふつう", 4: "悪い", 5: "とても悪い"}

func (w *openWeather) airQuality(ctx context.Context, in Input) (Output, error) {
	var a owAirPollution
	if err := w.get(ctx, "/data/2.5/air_pollution", in.Args, nil, &a); err != nil {
		return Output{}, err
	}
	if len(a.List) == 0 || a.List[0].Main.AQI < 1 || a.List[0].Main.AQI > 5 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("openweather: no air quality index")}
	}
	it := a.List[0]
	c := it.Components
	return Output{Item: &Row{
		Title:    "空気 " + owAQILabels[it.Main.AQI] + "(目安)",
		Subtitle: micrograms([]string{"PM2.5", "PM10", "O₃", "NO₂"}, c.PM25, c.PM10, c.O3, c.NO2),
		Value:    "AQI " + strconv.Itoa(it.Main.AQI) + "/5",
	}}, nil
}

// jst labels the air forecast's hours (the API gives no time zone).
var jst = time.FixedZone("JST", 9*60*60)

// airForecast is the hourly forecast: one row an hour from now, skipping
// hours without a valid AQI.
func (w *openWeather) airForecast(ctx context.Context, in Input) (Output, error) {
	limit, err := argInt(in.Args, "limit", owAirForecastDefault, 1, owAirForecastMax)
	if err != nil {
		return Output{}, err
	}
	var a owAirPollution
	if err := w.get(ctx, "/data/2.5/air_pollution/forecast", in.Args, nil, &a); err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, min(limit, len(a.List)))
	for _, it := range a.List {
		if len(rows) == limit {
			break
		}
		if it.Dt <= 0 || it.Main.AQI < 1 || it.Main.AQI > 5 {
			continue
		}
		rows = append(rows, Row{
			Title:    time.Unix(it.Dt, 0).In(jst).Format("1/2 15:04"),
			Subtitle: micrograms([]string{"PM2.5", "PM10"}, it.Components.PM25, it.Components.PM10),
			Value:    "AQI " + strconv.Itoa(it.Main.AQI) + "/5 " + owAQILabels[it.Main.AQI] + "(目安)",
		})
	}
	if len(rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("openweather: no air forecast rows")}
	}
	return Output{Rows: rows}, nil
}

// micrograms writes "PM2.5 12.3・PM10 20.1 µg/m³", leaving out missing values.
func micrograms(names []string, vs ...*float64) string {
	var sub []string
	for i, v := range vs {
		if v != nil {
			sub = append(sub, names[i]+" "+strconv.FormatFloat(math.Round(*v*10)/10+0, 'f', -1, 64))
		}
	}
	if len(sub) == 0 {
		return ""
	}
	return strings.Join(sub, "・") + " µg/m³"
}

func celsius(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10+0, 'f', -1, 64) + "°C" // +0: no "-0"
}

func describe(c []owCondition) string {
	if len(c) == 0 {
		return ""
	}
	d := strings.TrimSpace(c[0].Description)
	if r := []rune(d); len(r) > 40 {
		d = string(r[:40])
	}
	return d
}

// formatCoord writes a rounded coordinate without trailing zeros.
func formatCoord(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
