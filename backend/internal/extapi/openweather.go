package extapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// OpenWeatherMap (#1498): the weather now and the 3-hourly forecast at a
// lat / lng, from the free Current Weather and 5 day / 3 hour Forecast APIs
// (api.openweathermap.org/data/2.5). Free: a call is one `runs`, no coins.
//
//	current(lat, lng)            Item: description / feels, humidity, wind / temperature
//	forecast(lat, lng, limit)    Rows (3-hourly, limit 1-40, default 8 = 24 h)
//	airQuality(lat, lng)         Item: AQI 1-5 label (目安) / PM2.5, PM10, O₃, NO₂ / "AQI n/5"
//
// airQuality (#1656) is the Air Pollution API (/data/2.5/air_pollution, same
// host and key). The level is a guide only, never a health judgement.
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
			{Name: "forecast", Args: []string{"lat", "lng", "limit"}, Check: checkOWForecast},
			{Name: "airQuality", Args: []string{"lat", "lng"}, Check: checkOWLatLng},
		},
		Hosts: []string{openWeatherHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			return newOpenWeather(sec.APIKey, openWeatherHost, NewHTTPClient([]string{openWeatherHost}, DefaultTimeout))
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
	_, err := argInt(args, "limit", 8, 1, 40)
	return err
}

// newOpenWeather builds the ops against base (the API, or a test server).
func newOpenWeather(key, base string, hc *http.Client) (map[string]Operation, error) {
	if !owKeyRe.MatchString(key) {
		return nil, errors.New("EXT_OPEN_WEATHER_API_KEY is not set or not a key")
	}
	w := &openWeather{key: key, base: base, hc: hc}
	return map[string]Operation{
		"current":    OperationFunc(w.current),
		"forecast":   OperationFunc(w.forecast),
		"airQuality": OperationFunc(w.airQuality),
	}, nil
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

// get calls path with the rounded lat / lng.
func (w *openWeather) get(ctx context.Context, path string, args map[string]string, extra url.Values, out any) error {
	lat, lng, ok, err := argLatLng(args, 2)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: lat and lng are required", ErrUnknown)
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
	limit, err := argInt(in.Args, "limit", 8, 1, 40)
	if err != nil {
		return Output{}, err
	}
	var f owForecast
	if err := w.get(ctx, "/data/2.5/forecast", in.Args, url.Values{"cnt": {strconv.Itoa(limit)}}, &f); err != nil {
		return Output{}, err
	}
	tz := time.FixedZone("", f.City.Timezone)
	rows := make([]Row, 0, min(limit, len(f.List)))
	for _, it := range f.List {
		if len(rows) == limit {
			break
		}
		if it.Main.Temp == nil || it.Dt <= 0 {
			continue
		}
		v := celsius(*it.Main.Temp)
		if it.Pop != nil {
			v += "・降水 " + strconv.Itoa(int(math.Round(*it.Pop*100))) + "%"
		}
		rows = append(rows, Row{Title: time.Unix(it.Dt, 0).In(tz).Format("1/2 15:04"), Subtitle: describe(it.Weather), Value: v})
	}
	if len(rows) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("openweather: no forecast rows")}
	}
	return Output{Rows: rows}, nil
}

type owAirPollution struct {
	List []struct {
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
	var sub []string
	for _, c := range []struct {
		name string
		v    *float64
	}{{"PM2.5", it.Components.PM25}, {"PM10", it.Components.PM10}, {"O₃", it.Components.O3}, {"NO₂", it.Components.NO2}} {
		if c.v != nil {
			sub = append(sub, c.name+" "+strconv.FormatFloat(math.Round(*c.v*10)/10+0, 'f', -1, 64))
		}
	}
	subtitle := strings.Join(sub, "・")
	if subtitle != "" {
		subtitle += " µg/m³"
	}
	return Output{Item: &Row{
		Title:    "空気 " + owAQILabels[it.Main.AQI] + "(目安)",
		Subtitle: subtitle,
		Value:    "AQI " + strconv.Itoa(it.Main.AQI) + "/5",
	}}, nil
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
