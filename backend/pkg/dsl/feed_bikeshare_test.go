package dsl

import (
	"strings"
	"testing"
)

func TestFeedBikeshare(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedBikeshare, FeedID: "docomo-cycle-tokyo", Query: "35.681,139.767", Limit: 5},
		{Type: CompFeed, Provider: FeedBikeshare, FeedID: "hellocycling", Query: " 36.5613, 136.6562 "},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid feeds rejected: %v", err)
	}
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedBikeshare, Query: "35.681,139.767"},                                           // feedId missing
		{Type: CompFeed, Provider: FeedBikeshare, FeedID: "openstreet", Query: "35.681,139.767"},                     // not allowlisted
		{Type: CompFeed, Provider: FeedBikeshare, FeedID: "hellocycling"},                                            // point missing
		{Type: CompFeed, Provider: FeedBikeshare, FeedID: "hellocycling", Query: "東京駅"},                              // not a point
		{Type: CompFeed, Provider: FeedBikeshare, FeedID: "hellocycling", Query: "35.681,139.767", Region: "130000"}, // region
		{Type: CompFeed, Provider: FeedRSS, FeedID: "hellocycling"},                                                  // a bikeshare id on rss
	}
	joined := strings.Join(problems(t, app), "\n")
	for _, want := range []string{
		`feed bikeshare needs feedId, one of the allowlisted ids (docomo-cycle, docomo-cycle-tokyo, hellocycling) (got "")`,
		`(got "openstreet")`,
		`feed bikeshare needs query, the point to look around as "lat,lng"`,
		`(got "東京駅")`,
		`feed region is only for provider weather, warnings or pressure`,
		`feed rss needs feedId`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
	if n := strings.Count(joined, "feed bikeshare needs query"); n != 2 {
		t.Errorf("point problems = %d, want 2:\n%s", n, joined)
	}
}

func TestParseFeedPoint(t *testing.T) {
	for in, want := range map[string][2]float64{
		"35.681,139.767":         {35.681, 139.767},
		" 35.68149 , 139.76751 ": {35.681, 139.768}, // rounded to 3 places
		"+26.2124,127.6809":      {26.212, 127.681},
		"45,141":                 {45, 141},
	} {
		lat, lng, ok := ParseFeedPoint(in)
		if !ok || lat != want[0] || lng != want[1] {
			t.Errorf("%q = %v,%v %v, want %v", in, lat, lng, ok, want)
		}
	}
	for _, bad := range []string{"", "35.681", "35.681,", ",139.7", "139.767,35.681", "0,0", "-35.6,139.7", "35.6,-139.7",
		"3.5e1,139.7", "0x23,139.7", "NaN,139.7", "Inf,139.7", "35.6,139.7,1", "35.6;139.7", "35.6 139.7",
		"35.6,139.7" + strings.Repeat("0", 40)} {
		if lat, lng, ok := ParseFeedPoint(bad); ok {
			t.Errorf("%q accepted as %v,%v", bad, lat, lng)
		}
	}
}
