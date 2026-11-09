package dsl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const feedFile = "server_rules_feed.json"

// saveFeed parses, validates, round-trips and describes.
func TestServerRulesSaveFeedRoundTrip(t *testing.T) {
	raw := readsRaw(t, feedFile)
	a := readsApp(t, feedFile)
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	_ = json.Unmarshal(raw, &want)
	_ = json.Unmarshal(out, &got)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round-trip mismatch\n got: %s", out)
	}
	if got, want := a.DescribeServerRule(&a.ServerRules[0]), "毎日 07:00、東京都の気圧を「気圧の記録」に記録する・東京都の天気を「天気の記録」に記録する"; got != want {
		t.Errorf("describe: %s", got)
	}
}

func TestServerRulesSaveFeedRejects(t *testing.T) {
	sf := func(a *App) *RuleSaveFeed { return a.ServerRules[0].Do[0].SaveFeed }
	cases := []struct {
		name string
		edit func(a *App)
		want string
	}{
		{"record trigger", func(a *App) { a.ServerRules[0].On = RuleTrigger{Record: TriggerCreated, Entity: "conditions"} }, "saveFeed needs a time trigger without each"},
		{"each", func(a *App) { a.ServerRules[0].On.Each = "conditions" }, "saveFeed needs a time trigger without each"},
		{"provider", func(a *App) { sf(a).Provider = "rss" }, `provider "rss" must be weather, pressure, warnings`},
		{"region", func(a *App) { sf(a).Region = "999999" }, `region "999999" is not a forecast office code`},
		{"entity", func(a *App) { sf(a).Entity = "ghost" }, `entity "ghost" does not exist`},
		{"single", func(a *App) { a.Entities[1].Single = true }, "is single"},
		{"other side", func(a *App) { a.Entities[0].Scope = ScopeShared }, "a rule writes only records on the same side"},
		{"part of provider", func(a *App) { a.ServerRules[0].Do[1].SaveFeed.Fields.Drop = "low" }, "provider weather has no drop"},
		{"twice", func(a *App) { sf(a).Fields.Max = "low" }, `field "low" is written twice`},
		{"unknown field", func(a *App) { sf(a).Fields.Summary = "nope" }, `"nope" is not a field of entity "pressure_log"`},
		{"type", func(a *App) { sf(a).Fields.Summary = "falling" }, "summary needs a string / text field"},
		{"nothing to save", func(a *App) { sf(a).Fields = SaveFeedFields{Date: "day"} }, "needs at least one of summary, min, max, drop"},
		{"required min", func(a *App) { a.Entities[1].Fields[2].Required = true }, `required field "low" of entity "pressure_log" is not filled`},
		{"too many", func(a *App) {
			a.ServerRules = append(a.ServerRules, a.ServerRules[0])
			a.ServerRules[1].ID = "again"
		}, "too many saveFeed actions (4 > 3)"},
		{"two actions", func(a *App) {
			a.ServerRules[0].Do[0].Notify = &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}, Title: "t", Body: "b"}
		}, "write exactly one of notify, createRecord, updateRecord, increment, saveFeed"},
		{"chain", func(a *App) {
			a.ServerRules = append(a.ServerRules, ServerRule{ID: "on_log", On: RuleTrigger{Record: TriggerCreated, Entity: "pressure_log"},
				Do: []RuleAction{{Notify: &RuleNotify{To: RuleRecipient{Kind: RecipientOwner}, Title: "t", Body: "b"}}}})
		}, `writes entity "pressure_log", which triggers rule "on_log"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := readsApp(t, feedFile)
			c.edit(a)
			err := a.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// saveFeed is a closed object: unknown keys, a missing region, an unknown
// provider and a free field fail the schema.
func TestServerRulesSaveFeedSchema(t *testing.T) {
	raw := string(readsRaw(t, feedFile))
	from := `"provider": "pressure", "region": "130000"`
	if !strings.Contains(raw, from) {
		t.Fatal("fixture changed")
	}
	for _, to := range []string{
		`"provider": "pressure", "region": "130000", "url": "https://example.com"`,
		`"provider": "pressure"`,
		`"provider": "rss", "region": "130000"`,
	} {
		if _, err := Parse([]byte(strings.Replace(raw, from, to, 1))); err == nil {
			t.Errorf("accepted %s", to)
		}
	}
	bad := strings.Replace(raw, `"drop": "falling"}`, `"drop": "falling", "lat": "low"}`, 1)
	if _, err := Parse([]byte(bad)); err == nil {
		t.Error("accepted an unknown fields key")
	}
}

// A rule cannot save into an addressed entity (run-time table too).
func TestServerRulesSaveFeedAddressed(t *testing.T) {
	a := readsApp(t, "server_rules_addressed.json")
	r := &ServerRule{ID: "r", On: RuleTrigger{Every: "daily", Time: "07:00"},
		Do: []RuleAction{{SaveFeed: &RuleSaveFeed{Provider: FeedWeather, Region: "130000", Entity: "messages", Fields: SaveFeedFields{Summary: "x"}}}}}
	if got := a.AddressedRuleProblems(r); len(got) != 1 || !strings.Contains(got[0], "saveFeed: entity \"messages\" is addressed") {
		t.Fatalf("run-time: %v", got)
	}
}
