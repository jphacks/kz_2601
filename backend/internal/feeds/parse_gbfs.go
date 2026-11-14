package feeds

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// ---- bikeshare (GBFS 2.x station_information + station_status, #1694) ------
//
// The source is asked for the whole system (both files are fixed URLs); the
// point (Query.Q, "lat,lng" rounded by dsl.ParseFeedPoint) never leaves the
// server. One cache entry per system holds every station; Get picks the
// nearest ones within bikeshareRadiusM.

const (
	// gbfsInfoTTL: station names and places change rarely, and HELLO
	// CYCLING's station_information is about 8 MB, so it is read again only
	// this often (station_status follows the provider TTL).
	gbfsInfoTTL = 6 * time.Hour
	// bikeshareRadiusM: stations farther than this are not "near".
	bikeshareRadiusM = 2000
)

type gbfsInfo struct {
	Data struct {
		Stations []struct {
			ID   json.RawMessage `json:"station_id"`
			Name string          `json:"name"`
			Lat  *float64        `json:"lat"`
			Lon  *float64        `json:"lon"`
		} `json:"stations"`
	} `json:"data"`
}

type gbfsStatus struct {
	Data struct {
		Stations []struct {
			ID           json.RawMessage `json:"station_id"`
			Bikes        *int            `json:"num_bikes_available"`
			Docks        *int            `json:"num_docks_available"`
			Installed    *bool           `json:"is_installed"`
			Renting      *bool           `json:"is_renting"`
			Returning    *bool           `json:"is_returning"`
			LastReported int64           `json:"last_reported"`
		} `json:"stations"`
	} `json:"data"`
}

type gbfsStation struct {
	name     string
	lat, lon float64
}

// gbfsID reads a station_id written as a string (the spec) or a number.
func gbfsID(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

func parseGBFSInfo(body []byte) (map[string]gbfsStation, error) {
	var f gbfsInfo
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("bikeshare info: %w", err)
	}
	out := make(map[string]gbfsStation, len(f.Data.Stations))
	for _, st := range f.Data.Stations {
		id, name := gbfsID(st.ID), clean(st.Name, maxTitleRunes)
		if id == "" || name == "" || st.Lat == nil || st.Lon == nil || math.Abs(*st.Lat) > 90 || math.Abs(*st.Lon) > 180 {
			continue
		}
		out[id] = gbfsStation{name: name, lat: *st.Lat, lon: *st.Lon}
	}
	if len(out) == 0 {
		return nil, errors.New("bikeshare info: no stations")
	}
	return out, nil
}

// parseGBFSStatus joins station_status with the stations: one row per
// installed station with a place, title = name, subtitle = bikes / docks
// (or that renting / returning is stopped), date = last_reported (JST).
func parseGBFSStatus(body []byte, info map[string]gbfsStation) ([]Item, error) {
	var f gbfsStatus
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("bikeshare status: %w", err)
	}
	if len(f.Data.Stations) == 0 {
		return nil, errors.New("bikeshare status: no stations")
	}
	var out []Item
	for _, s := range f.Data.Stations {
		st, ok := info[gbfsID(s.ID)]
		if !ok || s.Bikes == nil || (s.Installed != nil && !*s.Installed) {
			continue
		}
		sub := "自転車 " + strconv.Itoa(max(*s.Bikes, 0)) + " 台"
		if s.Docks != nil {
			sub += "・返せる空き " + strconv.Itoa(max(*s.Docks, 0))
		}
		switch {
		case s.Renting != nil && !*s.Renting:
			sub = "貸出停止中"
		case s.Returning != nil && !*s.Returning:
			sub += "・返却停止中"
		}
		it := Item{Title: st.name, Subtitle: sub, lat: st.lat, lon: st.lon, located: true}
		if s.LastReported > 0 {
			it.Date = time.Unix(s.LastReported, 0).In(jst).Format(time.RFC3339)
		}
		out = append(out, it)
	}
	return out, nil
}

// fetchBikeshare reads station_status (and station_information when its copy
// is older than gbfsInfoTTL) and joins them. n is the bytes read.
func (s *Service) fetchBikeshare(ctx context.Context, q Query) (string, []Item, int, error) {
	sys, ok := s.ep.Bikeshare[q.FeedID]
	if !ok {
		return "", nil, 0, ErrUnknown
	}
	maxBody := providerLimits[dsl.FeedBikeshare].maxBody
	n := 0
	s.mu.Lock()
	info, at := s.gbfsInfo[q.FeedID], s.gbfsInfoAt[q.FeedID]
	s.mu.Unlock()
	if info == nil || s.now().Sub(at) >= gbfsInfoTTL {
		body, err := s.get(ctx, sys.Info, maxBody)
		n += len(body)
		if err != nil {
			return "", nil, n, err
		}
		if info, err = parseGBFSInfo(body); err != nil {
			return "", nil, n, fmt.Errorf("%w: %w", ErrBadResponse, err)
		}
		s.mu.Lock()
		s.gbfsInfo[q.FeedID], s.gbfsInfoAt[q.FeedID] = info, s.now()
		s.mu.Unlock()
	}
	body, err := s.get(ctx, sys.Status, maxBody)
	n += len(body)
	if err != nil {
		return "", nil, n, err
	}
	items, err := parseGBFSStatus(body, info)
	if err != nil {
		return "", nil, n, fmt.Errorf("%w: %w", ErrBadResponse, err)
	}
	return sys.Name + "の近くのポート", items, n, nil
}

// nearest returns up to limit stations within bikeshareRadiusM of the point,
// nearest first, with the distance added to the subtitle.
func nearest(items []Item, lat, lon float64, limit int) []Item {
	type hit struct {
		it Item
		d  float64
	}
	var hits []hit
	for _, it := range items {
		if !it.located {
			continue
		}
		if d := distanceM(lat, lon, it.lat, it.lon); d <= bikeshareRadiusM {
			hits = append(hits, hit{it, d})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return cmp.Compare(a.d, b.d) })
	out := []Item{}
	for _, h := range hits[:min(limit, len(hits))] {
		h.it.Subtitle += "・約 " + roundDistance(h.d)
		out = append(out, h.it)
	}
	return out
}

// distanceM is the equirectangular distance in metres (good to well under
// 1 % over a few kilometres).
func distanceM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	x := (lon2 - lon1) * rad * math.Cos((lat1+lat2)/2*rad)
	y := (lat2 - lat1) * rad
	return r * math.Hypot(x, y)
}

// roundDistance is 50 m steps under 1 km, then 0.1 km.
func roundDistance(m float64) string {
	if m < 975 {
		return strconv.Itoa(int(math.Max(50, math.Round(m/50)*50))) + " m"
	}
	return strconv.FormatFloat(math.Round(m/100)/10, 'f', 1, 64) + " km"
}
