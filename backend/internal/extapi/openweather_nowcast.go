package extapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// openWeather nowcast (#2134, a trial for #1655): whether rain starts or
// stops within the hour, from One Call 3.0's minutely list (the same host
// and key as forecast's uv). Off by default: the op is built only with
// EXT_OPEN_WEATHER_NOWCAST=1 (a One Call request is priced per call, and
// #1655 has not chosen the source yet). lat / lng are rounded and checked
// as for forecast.
//
//	nowcast(lat, lng)   Item: "12分後に降り出す" / "25分後に止む" /
//	                    "1時間は降らない" / "1時間は降り続く", value = now's mm/h
//	                    Rows: one per 10 minutes — "15:04" / the most mm/h in it
const EnvOpenWeatherNowcast = "EXT_OPEN_WEATHER_NOWCAST"

// owRainMMh is the least precipitation (mm/h) counted as rain.
const owRainMMh = 0.1

// owNowcastStep is the minutes one row covers.
const owNowcastStep = 10

type owMinutely struct {
	TimezoneOffset int `json:"timezone_offset"` // seconds from UTC
	Minutely       []struct {
		Dt            int64    `json:"dt"`
		Precipitation *float64 `json:"precipitation"`
	} `json:"minutely"`
}

type owMinute struct {
	dt int64
	mm float64
}

func (w *openWeather) nowcast(ctx context.Context, in Input) (Output, error) {
	var o owMinutely
	if err := w.get(ctx, "/data/3.0/onecall", in.Args, url.Values{"exclude": {"current,hourly,daily,alerts"}}, &o); err != nil {
		return Output{}, err
	}
	var ms []owMinute
	for _, m := range o.Minutely {
		if m.Dt <= 0 || m.Precipitation == nil || !finite(*m.Precipitation) || *m.Precipitation < 0 {
			continue
		}
		if len(ms) > 0 && m.Dt <= ms[len(ms)-1].dt {
			continue // out of order
		}
		ms = append(ms, owMinute{dt: m.Dt, mm: *m.Precipitation})
	}
	if len(ms) == 0 {
		// minutely is not given for every place
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("openweather: no minutely precipitation here")}
	}
	tz := time.FixedZone("", o.TimezoneOffset)
	return Output{Item: owNowcastItem(ms), Rows: owNowcastRows(ms, tz)}, nil
}

// owNowcastItem says when the rain starts or stops (minutes from the first
// point, which is now).
func owNowcastItem(ms []owMinute) *Row {
	raining := ms[0].mm >= owRainMMh
	item := &Row{Value: mmh(ms[0].mm)}
	for _, m := range ms[1:] {
		if (m.mm >= owRainMMh) != raining {
			n := strconv.Itoa(int((m.dt - ms[0].dt) / 60))
			if raining {
				item.Title = n + "分後に止む"
			} else {
				item.Title = n + "分後に降り出す"
			}
			return item
		}
	}
	if raining {
		item.Title = "1時間は降り続く"
	} else {
		item.Title = "1時間は降らない"
	}
	return item
}

// owNowcastRows: one row per owNowcastStep minutes from the first point,
// the most precipitation in it.
func owNowcastRows(ms []owMinute, tz *time.Location) []Row {
	var rows []Row
	var start int64
	most := -1.0
	flush := func() {
		if most >= 0 {
			rows = append(rows, Row{Title: time.Unix(start, 0).In(tz).Format("15:04"), Value: mmh(most)})
		}
	}
	for _, m := range ms {
		if most < 0 || m.dt >= start+owNowcastStep*60 {
			flush()
			start, most = ms[0].dt+(m.dt-ms[0].dt)/(owNowcastStep*60)*(owNowcastStep*60), m.mm
			continue
		}
		most = math.Max(most, m.mm)
	}
	flush()
	return rows
}

// mmh writes "0.3 mm/h" (one place).
func mmh(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10+0, 'f', -1, 64) + " mm/h"
}
