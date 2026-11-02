package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestRepeatMetrics(t *testing.T) {
	app := parseExample(t, "warikan")
	if miss := checkRepeat(map[string]any{"repeat": "monthly", "nextOccurrence": true}, app); len(miss) != 2 {
		t.Fatalf("no repeat / nextOccurrence must miss both: %v", miss)
	}
	app.App.Notifications = append(app.App.Notifications,
		dsl.Notification{ID: "fixed", Title: "t", Repeat: "monthly"}, // not record-based: ignored
		dsl.Notification{ID: "renew", Title: "t", Repeat: "monthly", Entity: "expenses", Field: "date", OffsetMinutes: -1440},
	)
	app.Entities[0].Fields = append(app.Entities[0].Fields, dsl.Field{ID: "next", Type: dsl.FieldDate, Computed: &dsl.Expr{
		Kind: dsl.ExprOp, Op: "nextOccurrence", Args: []dsl.Expr{{Kind: dsl.ExprLiteral, Lit: "2026-10-30"}, {Kind: dsl.ExprLiteral, Lit: "monthly"}},
	}})
	if miss := checkRepeat(map[string]any{"repeat": "monthly", "repeatOffset": -1440.0, "nextOccurrence": true}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkRepeat(map[string]any{"repeat": "monthly", "repeatOffset": -4320.0}, app); len(miss) != 1 {
		t.Fatalf("wrong offset must miss: %v", miss)
	}
	if miss := checkRepeat(map[string]any{"repeat": "yearly"}, app); len(miss) != 1 {
		t.Fatalf("no yearly must miss: %v", miss)
	}
}
