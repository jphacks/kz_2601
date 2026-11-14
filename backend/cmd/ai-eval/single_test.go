package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestSingleMetrics(t *testing.T) {
	app := parseExample(t, "warikan")
	if miss := checkSingle(map[string]any{"single": true, "setting": true}, app); len(miss) != 2 {
		t.Fatalf("no single / setting must miss both: %v", miss)
	}
	if miss := checkSingle(map[string]any{"single": false, "setting": false}, app); len(miss) != 0 {
		t.Fatalf("absent and not expected: %v", miss)
	}
	app.Entities = append(app.Entities, dsl.Entity{ID: "settings", Name: "設定", Single: true, Fields: []dsl.Field{{ID: "goal", Name: "目標", Type: dsl.FieldNumber}}})
	app.Entities[0].Fields = append(app.Entities[0].Fields, dsl.Field{ID: "g", Type: dsl.FieldNumber, Computed: &dsl.Expr{Kind: dsl.ExprSetting, Setting: "goal"}})
	if miss := checkSingle(map[string]any{"single": true, "setting": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
}
