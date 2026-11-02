package main

import (
	"os"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestMatchingMetrics(t *testing.T) {
	raw, err := os.ReadFile("../../pkg/dsl/testdata/matching.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if miss := checkMatching(map[string]any{"matchProfile": true, "matchMatched": true, "matchSafe": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	app.Entities[0].Fields = append(app.Entities[0].Fields, dsl.Field{ID: "phone", Name: "電話番号", Type: dsl.FieldString})
	if miss := checkMatching(map[string]any{"matchSafe": true}, app); len(miss) != 1 {
		t.Fatalf("a phone on the profile must miss: %v", miss)
	}
	if miss := checkMatching(map[string]any{"matchProfile": false}, parseExample(t, "kakeibo")); len(miss) != 0 {
		t.Fatal(miss)
	}
}
