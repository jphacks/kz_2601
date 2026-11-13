package dsl

import (
	"strings"
	"testing"
)

func TestFeedStatus(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedStatus, FeedID: "github", Title: "GitHub の稼働状況", Limit: 10},
		{Type: CompFeed, Provider: FeedStatus, FeedID: "claude"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid status feeds rejected: %v", err)
	}
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedStatus}, // feedId missing
		{Type: CompFeed, Provider: FeedStatus, FeedID: "https://www.githubstatus.com/api/v2/summary.json"}, // URL
		{Type: CompFeed, Provider: FeedStatus, FeedID: "openai"},                                           // not allowlisted
		{Type: CompFeed, Provider: FeedStatus, FeedID: "github", Region: "130000"},
		{Type: CompFeed, Provider: FeedRSS, FeedID: "github"}, // a status id on rss
	}
	joined := strings.Join(problems(t, app), "\n")
	for _, want := range []string{
		`feed status needs feedId, one of the allowlisted ids (claude, cloudflare, github, vercel) (got "")`,
		`(got "https://www.githubstatus.com/api/v2/summary.json")`,
		`(got "openai")`,
		`feed region is only for provider weather, warnings or pressure`,
		`feed rss needs feedId`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestFeedSchemaStatus(t *testing.T) {
	for comp, valid := range map[string]bool{
		`{"type":"feed","provider":"status","feedId":"cloudflare","limit":10}`: true,
		`{"type":"feed","provider":"statuspage","feedId":"github"}`:            false,
	} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
		if _, err := Parse(raw); (err == nil) != valid {
			t.Errorf("%s: err = %v, want valid=%v", comp, err, valid)
		}
	}
}
