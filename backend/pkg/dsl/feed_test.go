package dsl

import (
	"strings"
	"testing"
)

func TestFeedValid(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedWeather, Region: "170000", Title: "金沢の天気", Limit: 7},
		{Type: CompFeed, Provider: FeedHolidays},
		{Type: CompFeed, Provider: FeedRSS, FeedID: "digital-news", Limit: 20},
		{Type: CompFeed, Provider: FeedWarnings, Region: "130000"},
		{Type: CompFeed, Provider: FeedPressure, Region: "016000", Limit: 8},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid feeds rejected: %v", err)
	}
}

func TestFeedSemanticErrors(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedWeather},                                     // region missing
		{Type: CompFeed, Provider: FeedWeather, Region: "999999"},                   // not a forecast office
		{Type: CompFeed, Provider: FeedRSS, FeedID: "https://example.com/feed.xml"}, // URL instead of an id
		{Type: CompFeed, Provider: FeedHolidays, Region: "130000"},                  // region on holidays
		{Type: CompFeed, Provider: FeedWeather, Region: "130000", FeedID: "fdma-news"},
		{Type: CompFeed, Provider: FeedHolidays, Limit: 21},
		{Type: CompFeed, Provider: FeedWarnings, Region: "13000"}, // not 6 digits
		{Type: CompFeed, Provider: FeedPressure},                  // region missing
		{Type: CompFeed, Provider: FeedWarnings, Region: "130000", FeedID: "fdma-news"},
	}
	joined := strings.Join(problems(t, app), "\n")
	for _, want := range []string{
		`feed weather needs region`,
		`(got "999999")`,
		`feed rss needs feedId`,
		`feed region is only for provider weather, warnings or pressure`,
		`feed feedId is only for provider rss`,
		`feed limit must be 1..20`,
		`feed warnings needs region`,
		`(got "13000")`,
		`feed pressure needs region`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

// The schema closes provider and rejects keys a feed does not have (url).
func TestFeedSchemaClosed(t *testing.T) {
	for _, comp := range []string{
		`{"type":"feed","provider":"http"}`,
		`{"type":"feed","provider":"rss","url":"https://example.com/feed.xml"}`,
		`{"type":"feed"}`,
	} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
		if _, err := Parse(raw); err == nil {
			t.Errorf("%s should fail schema validation", comp)
		}
	}
	ok := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[{"type":"feed","provider":"weather","region":"130000","limit":3}]}]}`)
	if _, err := Parse(ok); err != nil {
		t.Errorf("weather feed: %v", err)
	}
	for _, p := range []string{"warnings", "pressure"} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[{"type":"feed","provider":"` + p + `","region":"170000"}]}]}`)
		if _, err := Parse(raw); err != nil {
			t.Errorf("%s feed: %v", p, err)
		}
	}
}

func TestFeedTables(t *testing.T) {
	for code, name := range WeatherRegions {
		if len(code) != 6 || strings.Trim(code, "0123456789") != "" || name == "" {
			t.Errorf("bad weather region %q=%q", code, name)
		}
	}
	for id := range RSSFeeds {
		if !idRe.MatchString(id) {
			t.Errorf("rss feed id %q is not an id", id)
		}
	}
}
