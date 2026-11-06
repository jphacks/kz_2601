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
