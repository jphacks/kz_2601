package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// TestHasCountWhere: one row per Expr kind and nesting; the app is the
// days/answers one of count_where_test.go.
func TestHasCountWhere_3192(t *testing.T) {
	a := countWhereApp(t, "days", `1`)
	rowRef := dsl.Filter{Field: "day", Op: "eq", Value: map[string]any{"ref": "id"}}
	byValue := dsl.Filter{Field: "status", Op: "eq", Value: "参加できる"}
	hit := dsl.Expr{Kind: dsl.ExprAgg, Agg: &dsl.AggExpr{Agg: "count", Entity: "answers", Filter: []dsl.Filter{rowRef, byValue}}}
	parentOnly := dsl.Expr{Kind: dsl.ExprAgg, Agg: &dsl.AggExpr{Agg: "count", Entity: "answers", Filter: []dsl.Filter{rowRef}}}
	sum := dsl.Expr{Kind: dsl.ExprAgg, Agg: &dsl.AggExpr{Agg: "sum", Entity: "answers", Field: "status", Filter: []dsl.Filter{rowRef, byValue}}}
	unknownEntity := dsl.Expr{Kind: dsl.ExprAgg, Agg: &dsl.AggExpr{Agg: "count", Entity: "nope", Filter: []dsl.Filter{rowRef, byValue}}}
	lit := dsl.Expr{Kind: dsl.ExprLiteral, Lit: 1.0}
	ref := dsl.Expr{Kind: dsl.ExprRef, Ref: "date"}
	setting := dsl.Expr{Kind: dsl.ExprSetting, Setting: "x"}
	cp := func(e dsl.Expr) *dsl.Expr { return &e }

	for _, tc := range []struct {
		name string
		x    *dsl.Expr
		want bool
	}{
		{"nil", nil, false},
		{"literal", &lit, false},
		{"ref", &ref, false},
		{"setting", &setting, false},
		{"agg without Agg", &dsl.Expr{Kind: dsl.ExprAgg}, false},
		{"count with row ref and value", &hit, true},
		{"count with row ref only", &parentOnly, false},
		{"sum is not count", &sum, false},
		{"count of an unknown entity", &unknownEntity, false},
		{"op: hit in the first arg", &dsl.Expr{Kind: dsl.ExprOp, Op: "add", Args: []dsl.Expr{hit, lit}}, true},
		{"op: hit in the last arg", &dsl.Expr{Kind: dsl.ExprOp, Op: "add", Args: []dsl.Expr{lit, parentOnly, hit}}, true},
		{"op: no hit", &dsl.Expr{Kind: dsl.ExprOp, Op: "add", Args: []dsl.Expr{lit, parentOnly}}, false},
		{"op: no args", &dsl.Expr{Kind: dsl.ExprOp, Op: "add"}, false},
		{"op nested in op", &dsl.Expr{Kind: dsl.ExprOp, Op: "mul", Args: []dsl.Expr{lit, {Kind: dsl.ExprOp, Op: "add", Args: []dsl.Expr{hit}}}}, true},
		{"if: hit in the condition", &dsl.Expr{Kind: dsl.ExprIf, If: &hit, Then: &lit, Else: &lit}, true},
		{"if: hit in then", &dsl.Expr{Kind: dsl.ExprIf, If: &lit, Then: &hit, Else: &lit}, true},
		{"if: hit in else", &dsl.Expr{Kind: dsl.ExprIf, If: &lit, Then: &lit, Else: &hit}, true},
		{"if: no hit", &dsl.Expr{Kind: dsl.ExprIf, If: &lit, Then: &parentOnly, Else: &lit}, false},
		{"if: only the condition", &dsl.Expr{Kind: dsl.ExprIf, If: cp(lit)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasCountWhere(a, "days", tc.x); got != tc.want {
				t.Fatalf("hasCountWhere = %v, want %v", got, tc.want)
			}
		})
	}
	// the same count seen from the other entity is not "children of this row"
	if hasCountWhere(a, "answers", &hit) {
		t.Fatal("owner answers: the ref field points at days, not answers")
	}
	// a nil child entity (countFilterNarrowed)
	if countFilterNarrowed(nil, "days", []dsl.Filter{rowRef, byValue}) {
		t.Fatal("nil child must not match")
	}
}
