package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestRetentionMetrics(t *testing.T) {
	app := parseExample(t, "warikan")
	if miss := checkRetention(map[string]any{"retention": 0.0, "retentionOnRef": false}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkRetention(map[string]any{"retention": 7.0}, app); len(miss) != 1 {
		t.Fatalf("no period must miss retention=7: %v", miss)
	}
	targets := refTargets(app)
	for i := range app.Entities {
		e := &app.Entities[i]
		if targets[e.ID] {
			e.RetentionDays = 30 // a master list (members) got the period
		}
	}
	if len(targets) == 0 {
		t.Fatal("warikan example has no ref target")
	}
	if miss := checkRetention(map[string]any{"retention": 30.0, "retentionOnRef": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkRetention(map[string]any{"retention": 0.0}, app); len(miss) != 1 {
		t.Fatalf("a period must miss retention=0: %v", miss)
	}
	_ = dsl.MaxRetentionDays
}
