package dsl

import (
	"strings"
	"testing"
)

func TestFeedArxivDiet(t *testing.T) {
	app := Initial()
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedArxiv, FeedID: "cs.CL", Limit: 10},
		{Type: CompFeed, Provider: FeedDiet, Query: "こども家庭庁"},
		{Type: CompFeed, Provider: FeedDiet, Query: "防災　予算"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("valid feeds rejected: %v", err)
	}
	app.Screens[0].Components = []Component{
		{Type: CompFeed, Provider: FeedArxiv},                                // feedId missing
		{Type: CompFeed, Provider: FeedArxiv, FeedID: "digital-news"},        // an rss id
		{Type: CompFeed, Provider: FeedRSS, FeedID: "cs.AI"},                 // an arxiv id
		{Type: CompFeed, Provider: FeedDiet},                                 // query missing
		{Type: CompFeed, Provider: FeedDiet, Query: strings.Repeat("国", 41)}, // too long
		{Type: CompFeed, Provider: FeedDiet, Query: "予算\n委員会"},               // newline
		{Type: CompFeed, Provider: FeedDiet, Query: "予算", FeedID: "cs.AI"},   // feedId on diet
		{Type: CompFeed, Provider: FeedHolidays, Query: "予算"},                // query off diet
		{Type: CompFeed, Provider: FeedDiet, Query: "予算", Region: "130000"},  // region on diet
	}
	joined := strings.Join(problems(t, app), "\n")
	for _, want := range []string{
		`feed arxiv needs feedId, one of the allowlisted ids (cs.AI, cs.CL,`,
		`(got "digital-news")`,
		`feed rss needs feedId`,
		`feed diet needs query, 1..40 characters`,
		`(got "予算\n委員会")`,
		`feed feedId is only for provider rss, arxiv or status`,
		`feed query is only for provider diet`,
		`feed region is only for provider weather, warnings or pressure`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
	if n := strings.Count(joined, "feed diet needs query"); n != 3 {
		t.Errorf("diet query problems = %d, want 3:\n%s", n, joined)
	}
}

func TestNormalizeFeedQuery(t *testing.T) {
	for in, want := range map[string]string{"こども家庭庁": "こども家庭庁", "　防災 　予算 ": "防災 予算", strings.Repeat("a", 40): strings.Repeat("a", 40)} {
		if got, ok := NormalizeFeedQuery(in); !ok || got != want {
			t.Errorf("%q = %q %v, want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", " 　", strings.Repeat("a", 41), "a\tb", "a\u200bb", "a\x7fb", "a\xffb", "a\u202eb"} {
		if got, ok := NormalizeFeedQuery(bad); ok {
			t.Errorf("%q accepted as %q", bad, got)
		}
	}
}

func TestFeedSchemaArxivDiet(t *testing.T) {
	for comp, valid := range map[string]bool{
		`{"type":"feed","provider":"arxiv","feedId":"cs.AI"}`:          true,
		`{"type":"feed","provider":"diet","query":"こども家庭庁","limit":5}`: true,
		`{"type":"feed","provider":"nhk","feedId":"cat0"}`:             false,
		`{"type":"feed","provider":"diet","query":3}`:                  false,
	} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
		if _, err := Parse(raw); (err == nil) != valid {
			t.Errorf("%s: err = %v, want valid=%v", comp, err, valid)
		}
	}
}
