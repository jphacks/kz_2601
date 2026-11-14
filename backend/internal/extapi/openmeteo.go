package extapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Open-Meteo: the weather now and the daily forecast at a lat / lng, from
// the keyless API (api.open-meteo.com/v1/forecast). Free, no key, no OAuth:
// a call is one `runs`. Off by default (EXT_OPEN_METEO_ENABLED=1).
//
//	forecast(lat, lng, days?)   Rows: one a day (JST) — weather / high, low, rain chance
//	current(lat, lng)           Item: weather / feels, humidity, wind / temperature
//
// days is 1-16 (default 7). lat / lng are rounded to 2 places (about 1 km)
// before they leave the runner, as for openWeather. The same arguments are
// answered from the cache for 10 minutes.
//
// Terms (docs/backend/features/external-api-terms.md): the free API is for
// non-commercial use only — the terms list apps with subscriptions or ads
// as commercial, which an app behind Tailor It Pro may be; the paid plans
// use a key and another host (customer-api.open-meteo.com), not this
// provider. The data is CC BY 4.0: "Weather data by Open-Meteo.com" with a
// link (OpenMeteoAttribution) goes with it. Whether to ship it, and on which
// plan, is the owner's decision: default off (decisions/external-apis.md
// says the free API is not usable, #348).
//
// The free quota is 600 a minute, 5,000 an hour and 10,000 a day per IP:
// GlobalDaily stays under the day.
const (
	OpenMeteo = "openMeteo"
	// OpenMeteoAttribution is the credit shown under the rows (CC BY 4.0).
	OpenMeteoAttribution = "Weather data by Open-Meteo.com (CC BY 4.0)"
	// OpenMeteoAttributionURL is the link the credit carries.
	OpenMeteoAttributionURL = "https://open-meteo.com/"

	openMeteoHost        = "https://api.open-meteo.com"
	openMeteoGlobalDaily = 8000
	openMeteoCacheTTL    = 10 * time.Minute
	openMeteoDaysDefault = 7
	openMeteoDaysMax     = 16
)

func init() {
	register(Provider{ID: OpenMeteo, Billing: BillingFree, GlobalDaily: openMeteoGlobalDaily,
		Ops: []OpSpec{
			{Name: "forecast", Args: []string{"lat", "lng", "days"}, Check: checkOpenMeteoForecast, CacheTTL: openMeteoCacheTTL},
			{Name: "current", Args: []string{"lat", "lng"}, Check: checkOWLatLng, CacheTTL: openMeteoCacheTTL},
		},
		Hosts: []string{openMeteoHost},
		New: func(Secrets) (map[string]Operation, error) {
			return newOpenMeteo(openMeteoHost, NewHTTPClient([]string{openMeteoHost}, DefaultTimeout)), nil
		}})
}

func checkOpenMeteoForecast(args map[string]string) error {
	if err := checkOWLatLng(args); err != nil {
		return err
	}
	_, err := argInt(args, "days", openMeteoDaysDefault, 1, openMeteoDaysMax)
	return err
}

// newOpenMeteo builds the ops against base (the API, or a test server).
func newOpenMeteo(base string, hc *http.Client) map[string]Operation {
	m := &openMeteo{base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{"forecast": OperationFunc(m.forecast), "current": OperationFunc(m.current)}
}

type openMeteo struct {
	base string
	hc   *http.Client
}

// omWeatherJa names the WMO weather codes Open-Meteo uses.
var omWeatherJa = map[int]string{
	0: "快晴", 1: "晴れ", 2: "一部曇り", 3: "曇り",
	45: "霧", 48: "着氷性の霧",
	51: "弱い霧雨", 53: "霧雨", 55: "強い霧雨", 56: "弱い着氷性の霧雨", 57: "強い着氷性の霧雨",
	61: "弱い雨", 63: "雨", 65: "強い雨", 66: "弱い着氷性の雨", 67: "強い着氷性の雨",
	71: "弱い雪", 73: "雪", 75: "強い雪", 77: "霧雪",
	80: "弱いにわか雨", 81: "にわか雨", 82: "激しいにわか雨", 85: "弱いにわか雪", 86: "強いにわか雪",
	95: "雷雨", 96: "雷雨(弱いひょう)", 99: "雷雨(強いひょう)",
}

// omWeather writes the code's name ("" when missing; an unknown code shows
// the number).
func omWeather(code *int) string {
	if code == nil {
		return ""
	}
	if ja, ok := omWeatherJa[*code]; ok {
		return ja
	}
	return "天気コード " + strconv.Itoa(*code)
}

var omWeekdays = [...]string{"日", "月", "火", "水", "木", "金", "土"}

// omDay writes "11/14(金)" for a daily time ("2026-11-14"); "" when it is
// not a date.
func omDay(s string) string {
	d, err := time.ParseInLocation(time.DateOnly, s, jst)
	if err != nil {
		return ""
	}
	return d.Format("1/2") + "(" + omWeekdays[d.Weekday()] + ")"
}

// get calls /v1/forecast with the rounded lat / lng, JST and extra.
func (m *openMeteo) get(ctx context.Context, args map[string]string, extra url.Values, out any) error {
	lat, lng, ok, err := argLatLng(args, 2)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("lat and lng are required")
	}
	q := url.Values{"latitude": {formatCoord(lat)}, "longitude": {formatCoord(lng)}, "timezone": {"Asia/Tokyo"}}
	for k, v := range extra {
		q[k] = v
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.base+"/v1/forecast?"+q.Encode(), nil)
	if err != nil {
		return errors.New("open-meteo: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	return sendJSON(ctx, m.hc, req, out)
}

type omDaily struct {
	Time       []string   `json:"time"`
	Code       []*int     `json:"weather_code"`
	TempMax    []*float64 `json:"temperature_2m_max"`
	TempMin    []*float64 `json:"temperature_2m_min"`
	RainChance []*float64 `json:"precipitation_probability_max"`
}

// omAt returns the i-th value of a column, nil past its end.
func omAt[T any](col []*T, i int) *T {
	if i < len(col) {
		return col[i]
	}
	return nil
}

// forecast: one row a day — title = the day (JST), subtitle = the day's
// weather, value = "最高 15°C / 最低 8°C・降水 40%". A day without a
// temperature is skipped.
func (m *openMeteo) forecast(ctx context.Context, in Input) (Output, error) {
	days, err := argInt(in.Args, "days", openMeteoDaysDefault, 1, openMeteoDaysMax)
	if err != nil {
		return Output{}, err
	}
	var a struct {
		Daily omDaily `json:"daily"`
	}
	extra := url.Values{"daily": {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"}, "forecast_days": {strconv.Itoa(days)}}
	if err := m.get(ctx, in.Args, extra, &a); err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, min(days, len(a.Daily.Time)))
	for i, day := range a.Daily.Time {
		if len(rows) == days {
			break
		}
		title := omDay(day)
		hi, lo := omAt(a.Daily.TempMax, i), omAt(a.Daily.TempMin, i)
		if title == "" || hi == nil || lo == nil || !finite(*hi) || !finite(*lo) {
			continue
		}
		v := "最高 " + celsius(*hi) + " / 最低 " + celsius(*lo)
		if p := omAt(a.Daily.RainChance, i); p != nil && finite(*p) && *p >= 0 {
			v += "・降水 " + strconv.Itoa(int(math.Round(*p))) + "%"
		}
		rows = append(rows, Row{Title: title, Subtitle: omWeather(omAt(a.Daily.Code, i)), Value: v})
	}
	if len(rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("open-meteo: no forecast rows")}
	}
	return Output{Rows: rows}, nil
}

// current: title = the weather, subtitle = feels like, humidity, wind,
// value = the temperature.
func (m *openMeteo) current(ctx context.Context, in Input) (Output, error) {
	var a struct {
		Current struct {
			Temp     *float64 `json:"temperature_2m"`
			Feels    *float64 `json:"apparent_temperature"`
			Humidity *float64 `json:"relative_humidity_2m"`
			Wind     *float64 `json:"wind_speed_10m"`
			Code     *int     `json:"weather_code"`
		} `json:"current"`
	}
	extra := url.Values{"current": {"temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m"}, "wind_speed_unit": {"ms"}}
	if err := m.get(ctx, in.Args, extra, &a); err != nil {
		return Output{}, err
	}
	c := a.Current
	if c.Temp == nil || !finite(*c.Temp) {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("open-meteo: no temperature")}
	}
	var sub []string
	if c.Feels != nil && finite(*c.Feels) {
		sub = append(sub, "体感 "+celsius(*c.Feels))
	}
	if c.Humidity != nil && finite(*c.Humidity) {
		sub = append(sub, "湿度 "+strconv.Itoa(int(math.Round(*c.Humidity)))+"%")
	}
	if c.Wind != nil && finite(*c.Wind) {
		sub = append(sub, "風 "+strconv.FormatFloat(math.Round(*c.Wind*10)/10+0, 'f', 1, 64)+" m/s")
	}
	title := omWeather(c.Code)
	if title == "" {
		title = "今の天気"
	}
	return Output{Item: &Row{Title: title, Subtitle: strings.Join(sub, "・"), Value: celsius(*c.Temp)}}, nil
}
