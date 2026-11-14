package dsl

import (
	"math"
	"strconv"
	"strings"
)

// bikeshare feed (#1694): the free bikes and docks of the stations nearest
// to a fixed point, read from an operator's GBFS feed (Component.FeedID =
// BikeshareSystems). The point is Component.Query "lat,lng"; the server
// rounds it (FeedPointDigits) and keeps it to itself: the source is asked
// for the whole system, never for a place.
// docs/backend/features/external-feeds.md

// BikeshareSystems maps an allowlisted GBFS system id to its name. The URLs
// live on the server (internal/feeds); adding one is a reviewed change to
// both tables after its terms are checked.
var BikeshareSystems = map[string]string{
	"docomo-cycle":       "ドコモ・バイクシェア(東京以外)",
	"docomo-cycle-tokyo": "ドコモ・バイクシェア(東京)",
	"hellocycling":       "HELLO CYCLING",
}

// FeedPointDigits rounds the bikeshare point (3 places ≈ 100 m: enough to
// pick the nearest stations, which are a few hundred metres apart).
const FeedPointDigits = 3

// ParseFeedPoint reads "lat,lng" (plain decimals) inside Japan's rough
// bounds and rounds it to FeedPointDigits places.
func ParseFeedPoint(s string) (lat, lng float64, ok bool) {
	a, b, found := strings.Cut(strings.TrimSpace(s), ",")
	if !found || len(s) > FeedQueryMaxRunes {
		return 0, 0, false
	}
	lat, ok1 := plainDecimal(a)
	lng, ok2 := plainDecimal(b)
	if !ok1 || !ok2 || lat < 20 || lat > 46 || lng < 122 || lng > 154 {
		return 0, 0, false
	}
	p := math.Pow10(FeedPointDigits)
	return math.Round(lat*p) / p, math.Round(lng*p) / p, true
}

func plainDecimal(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.Trim(s, "0123456789.-+") != "" {
		return 0, false // no hex, exponent, Inf or NaN
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func checkFeedPoint(where string, c *Component, add func(string, ...any)) {
	if _, _, ok := ParseFeedPoint(c.Query); !ok {
		add("%s: feed bikeshare needs query, the point to look around as \"lat,lng\" in Japan such as \"35.681,139.767\" (got %q)", where, c.Query)
	}
}
