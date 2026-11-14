package main

import (
	"os"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// rulePick reports fewest for an assignment; ruleSim runs it (created and
// on.each) on made-up records.
func TestServerRulesAssignMetrics(t *testing.T) {
	raw, err := os.ReadFile("../../pkg/dsl/testdata/server_rules_assign.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkServerRules(map[string]any{"rulePick": "fewest", "ruleAction": "updateRecord", "ruleSim": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkServerRules(map[string]any{"rulePick": "most"}, app); len(miss) != 1 {
		t.Fatalf("want 1 miss: %v", miss)
	}
}
