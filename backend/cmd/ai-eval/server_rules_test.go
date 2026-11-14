package main

import (
	"os"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestServerRulesMetrics(t *testing.T) {
	raw, err := os.ReadFile("../../pkg/dsl/testdata/server_rules.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]any{"serverRules": true, "ruleTrigger": "weekly", "ruleAction": "increment", "ruleTo": "authorOf", "ruleSim": true}
	if miss := checkServerRules(all, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkServerRules(map[string]any{"serverRules": false, "ruleTrigger": "daily", "ruleTo": "author"}, app); len(miss) != 3 {
		t.Fatalf("want 3 misses: %v", miss)
	}
	app.ServerRules = nil
	if miss := checkServerRules(map[string]any{"serverRules": false}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkServerRules(map[string]any{"ruleSim": true}, app); len(miss) != 1 {
		t.Fatalf("no rules must miss ruleSim: %v", miss)
	}
}

// ruleLookup names the op; ruleSim runs the lookup on made-up records.
func TestServerRulesLookupMetrics(t *testing.T) {
	raw, err := os.ReadFile("../../pkg/dsl/testdata/server_rules_lookup.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkServerRules(map[string]any{"ruleLookup": "notExists", "ruleEach": true, "ruleSim": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkServerRules(map[string]any{"ruleLookup": "exists"}, app); len(miss) != 1 {
		t.Fatalf("want 1 miss: %v", miss)
	}
}

// ruleSimilar reports a rule with similar; ruleSim runs it on made-up records.
func TestServerRulesSimilarMetrics(t *testing.T) {
	raw, err := os.ReadFile("../../pkg/dsl/testdata/server_rules_similar.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkServerRules(map[string]any{"ruleSimilar": true, "ruleEach": true, "ruleSim": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	app.ServerRules = app.ServerRules[:0]
	if miss := checkServerRules(map[string]any{"ruleSimilar": true}, app); len(miss) != 1 {
		t.Fatalf("want 1 miss: %v", miss)
	}
}

// rulePick reports the order of a rule with pick; ruleSim runs it on made-up
// records (with and without a row).
func TestServerRulesPickMetrics(t *testing.T) {
	raw, err := os.ReadFile("../../pkg/dsl/testdata/server_rules_pick.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkServerRules(map[string]any{"rulePick": "random", "ruleSim": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkServerRules(map[string]any{"rulePick": "oldest"}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkServerRules(map[string]any{"rulePick": "newest"}, app); len(miss) != 1 {
		t.Fatalf("want 1 miss: %v", miss)
	}
}

// ruleAction saveFeed; ruleSim plans it on a made-up snapshot.
func TestServerRulesSaveFeedMetrics(t *testing.T) {
	raw, err := os.ReadFile("../../pkg/dsl/testdata/server_rules_feed.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkServerRules(map[string]any{"ruleTrigger": "daily", "ruleAction": "saveFeed", "ruleSim": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
}
