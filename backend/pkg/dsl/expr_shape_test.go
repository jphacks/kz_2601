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
