package dsl

import (
	"encoding/json"
	"testing"
)

func TestExprShapeProblems(t *testing.T) {
	for _, tc := range []struct {
		src  string
		bad  bool
		name string
	}{
		{`{"op":"addDays","args":[{"ref":"due"},-3]}`, false, "ok"},
		{`{"agg":"sum","entity":"e","field":"a","filter":{"field":"date","op":"thisMonth","value":null}}`, false, "agg filter"},
		{`{"op":"today","args":[]}`, false, "nullary"},
		{`{"op":"addDays","args":[{"ref":"due"}]}`, true, "arity"},
		{`{"op":"subtract","args":[1,2]}`, true, "unknown op"},
		{`{"agg":"total","entity":"e"}`, true, "unknown agg"},
		{`{"agg":"count","entity":"e","filter":{"field":"d","op":"between","value":1}}`, true, "filter op"},
		{`{"agg":"count","entity":"e","filter":{"field":"d","op":"today","value":"x"}}`, true, "period value"},
		{`{"op":"nextOccurrence","args":[{"ref":"d"},"weekly"]}`, true, "repeat unit"},
		{`{"if":{"op":"not","args":[]},"then":1,"else":2}`, true, "nested arity"},
	} {
		var e Expr
		if err := json.Unmarshal([]byte(tc.src), &e); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := ExprShapeProblems(&e); (len(got) > 0) != tc.bad {
			t.Errorf("%s: problems = %v, want bad=%v", tc.name, got, tc.bad)
		}
	}
}

// TestExprShapeAgreesWithChecker: on an app whose names all resolve, the
// shape check is never stricter than the app checker, and the app-independent
// mistakes (arity, op / agg / filter names, agg field / by, agg-only filters)
// are caught by both. Literal types (addDays with a string) are left to Parse.
func TestExprShapeAgreesWithChecker(t *testing.T) {
	e := &Entity{ID: "e", Name: "e", Fields: []Field{
		{ID: "due", Name: "due", Type: FieldDate},
		{ID: "a", Name: "a", Type: FieldNumber},
	}}
	for _, tc := range []struct {
		src string
		bad bool // both checkers must report it
	}{
		{`{"op":"addDays","args":[{"ref":"due"},-3]}`, false},
		{`{"agg":"sum","entity":"e","field":"a","filter":{"field":"due","op":"thisMonth","value":null}}`, false},
		{`{"agg":"latest","entity":"e","field":"a","by":"due"}`, false},
		{`{"agg":"count","entity":"e","filter":[{"field":"a","op":"gte","value":{"op":"add","args":[1,2]}}]}`, false},
		{`{"if":{"op":"gt","args":[{"ref":"a"},0]},"then":1,"else":2}`, false},
		{`{"op":"addDays","args":[{"ref":"due"}]}`, true},
		{`{"op":"add","args":[1]}`, true},
		{`{"op":"subtract","args":[1,2]}`, true},
		{`{"agg":"total","entity":"e","field":"a"}`, true},
		{`{"agg":"sum","entity":"e"}`, true},
		{`{"agg":"sum","entity":"e","field":"a","by":"due"}`, true},
		{`{"agg":"count","entity":"e","filter":{"field":"due","op":"between","value":1}}`, true},
		{`{"agg":"count","entity":"e","filter":{"field":"due","op":"today","value":"x"}}`, true},
		{`{"agg":"count","entity":"e","filter":{"field":"a","op":"eq","value":{"me":true}}}`, true},
		{`{"agg":"count","entity":"e","filter":{"field":"$author","op":"eq","value":"x"}}`, true},
		{`{"op":"nextOccurrence","args":[{"ref":"due"},"weekly"]}`, true},
		{`{"if":{"op":"not","args":[]},"then":1,"else":2}`, true},
	} {
		var x Expr
		if err := json.Unmarshal([]byte(tc.src), &x); err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		var full []string
		ch := &exprChecker{entities: map[string]*Entity{"e": e}, add: func(f string, _ ...any) { full = append(full, f) }, deps: map[fieldKey]bool{}}
		ch.check("expr", &x, e, 1)
		shape := ExprShapeProblems(&x)
		if len(shape) > 0 && len(full) == 0 {
			t.Errorf("%s: shape is stricter than the app checker: %v", tc.src, shape)
		}
		if (len(shape) > 0) != tc.bad || (len(full) > 0) != tc.bad {
			t.Errorf("%s: shape=%v full=%v, want bad=%v", tc.src, shape, full, tc.bad)
		}
	}
}
