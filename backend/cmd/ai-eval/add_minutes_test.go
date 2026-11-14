package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAddMinutes(t *testing.T) {
	expr := dsl.Expr{Kind: dsl.ExprOp, Op: "addMinutes", Args: []dsl.Expr{{Kind: dsl.ExprRef, Ref: "arrive"}, {Kind: dsl.ExprLiteral, Lit: -30.0}}}
	app := &dsl.App{Entities: []dsl.Entity{{ID: "trip", Fields: []dsl.Field{{ID: "depart", Type: dsl.FieldString, Computed: &expr}}}}}
	if miss := checkAddMinutes(map[string]any{"addMinutes": true}, app); miss != nil {
		t.Fatalf("uses addMinutes: %v", miss)
	}
	if miss := checkAddMinutes(map[string]any{"addMinutes": false}, app); miss == nil {
		t.Fatal("addMinutes=false must miss")
	}
	if miss := checkAddMinutes(map[string]any{"addMinutes": true}, &dsl.App{}); miss == nil {
		t.Fatal("no addMinutes must miss")
	}
	if bad := validateExpect(map[string]any{"addMinutes": "yes"}); len(bad) != 1 {
		t.Fatalf("validateExpect = %v", bad)
	}
}
