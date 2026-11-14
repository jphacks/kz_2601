package feeds

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// ---- pressure (MET Norway locationforecast/2.0/compact) ---------------------

type metForecast struct {
	Properties struct {
		Timeseries []struct {
			Time string `json:"time"`
			Data struct {
				Instant struct {
					Details struct {
						Pressure *float64 `json:"air_pressure_at_sea_level"`
					} `json:"details"`
				} `json:"instant"`
			} `json:"data"`
		} `json:"timeseries"`
	} `json:"properties"`
}

const (
	pressureHours = 48
	pressureBlock = 6 * time.Hour
	// pressureDropHPa marks a block where the pressure has fallen this much
	// or more from the highest value of the 24 hours before (the fall people
	// with weather-related headaches watch for).
	pressureDropHPa = 6.0
)

// parsePressure turns the hourly sea-level pressure of the next 48 hours
// (from now) into 6-hour rows aligned to 0/6/12/18 時 JST: date = block
// start (RFC 3339, JST), title = "6時〜12時", min / max (hPa), series = the
// hourly values, drop = a fall of pressureDropHPa or more within 24 hours
// ends in the block.
func parsePressure(body []byte, now time.Time) ([]Item, error) {
	var f metForecast
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("pressure: %w", err)
	}
	type point struct {
		t time.Time
		p float64
	}
	start := now.Truncate(time.Hour)
	end := start.Add(pressureHours * time.Hour)
	var pts []point
	for _, ts := range f.Properties.Timeseries {
		t, err := time.Parse(time.RFC3339, ts.Time)
		p := ts.Data.Instant.Details.Pressure
		if err != nil || p == nil || *p < 850 || *p > 1100 || t.Before(start) || !t.Before(end) {
			continue
		}
		// Hourly only (the source switches to 6-hourly after about 60 hours).
		if len(pts) > 0 && t.Sub(pts[len(pts)-1].t) != time.Hour {
			if t.Sub(pts[len(pts)-1].t) < time.Hour {
				continue
			}
			break
		}
		pts = append(pts, point{t, *p})
	}
	if len(pts) < 2 {
		return nil, errors.New("pressure: no hourly values")
	}
	var out []Item
	var falls []float64 // biggest 24-hour fall ending in each block
	var cur *Item
	var curStart time.Time
	for i, pt := range pts {
		local := pt.t.In(jst)
		bs := time.Date(local.Year(), local.Month(), local.Day(), local.Hour()/6*6, 0, 0, 0, jst)
		if cur == nil || !bs.Equal(curStart) {
			out = append(out, Item{Date: bs.Format(time.RFC3339),
				Title: fmt.Sprintf("%d時〜%d時", bs.Hour(), bs.Hour()+6)})
			falls = append(falls, 0)
			cur, curStart = &out[len(out)-1], bs
		}
		v := math.Round(pt.p*10) / 10
		cur.Series = append(cur.Series, v)
		if cur.Min == nil || v < *cur.Min {
			cur.Min = ptr(v)
		}
		if cur.Max == nil || v > *cur.Max {
			cur.Max = ptr(v)
		}
		// Highest value of the preceding 24 hours (as far as the data goes).
		hi := pt.p
		for j := i - 1; j >= 0 && pt.t.Sub(pts[j].t) <= 24*time.Hour; j-- {
			hi = math.Max(hi, pts[j].p)
		}
		falls[len(falls)-1] = math.Max(falls[len(falls)-1], hi-pt.p)
	}
	for i := range out {
		s := out[i].Series
		switch {
		case falls[i] >= pressureDropHPa:
			out[i].Drop = ptr(true)
			out[i].Subtitle = fmt.Sprintf("24 時間で %.0f hPa 下がります", math.Floor(falls[i]))
		case len(s) > 1:
			out[i].Subtitle = fmt.Sprintf("%.0f → %.0f hPa", s[0], s[len(s)-1])
		}
	}
	return out, nil
}

func ptr[T any](v T) *T { return &v }
