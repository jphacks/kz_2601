package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestNotifyWhenMetrics(t *testing.T) {
	app := parseExample(t, "warikan")
	if miss := checkNotifyWhen(map[string]any{"notifyWhen": true, "notifyWhenOp": "gt"}, app); len(miss) != 2 {
		t.Fatalf("no when must miss both: %v", miss)
	}
	if miss := checkNotifyWhen(map[string]any{"notifyWhen": false}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	sum := dsl.Expr{Kind: dsl.ExprAgg, Agg: &dsl.AggExpr{Agg: "sum", Entity: "expenses", Field: "amount"}}
	app.App.Notifications = append(app.App.Notifications, dsl.Notification{ID: "over", Title: "t", Repeat: "once",
		When: &dsl.Expr{Kind: dsl.ExprOp, Op: "gt", Args: []dsl.Expr{sum, {Kind: dsl.ExprLiteral, Lit: 50000.0}}}})
	if miss := checkNotifyWhen(map[string]any{"notifyWhen": true, "notifyWhenOp": "gt"}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	if miss := checkNotifyWhen(map[string]any{"notifyWhenOp": "lt"}, app); len(miss) != 1 {
		t.Fatalf("wrong op must miss: %v", miss)
	}
}
