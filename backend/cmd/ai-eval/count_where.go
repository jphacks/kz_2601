package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// countWhere expect key, checked by checkCountWhere:
//
//	countWhere (bool)  some computed field counts only the children of this
//	                   record with a given value: agg count whose AND filter
//	                   has {"ref":"id"} and at least one literal condition
//
// Cases: internal/ai/eval/schedule_poll_intent_cases.json (-cases).
func init() {
	expectKeys["countWhere"] = expectBool
}

// countWhereFields lists "entity.field" of computed fields that count a
// parent's children narrowed by a value (docs/dsl/examples/schedule_poll.json).
func countWhereFields(a *dsl.App) []string {
	var out []string
	for _, e := range a.Entities {
		for _, f := range e.Fields {
			if f.Computed != nil && hasCountWhere(f.Computed) {
				out = append(out, e.ID+"."+f.ID)
			}
		}
	}
	return out
}

func hasCountWhere(x *dsl.Expr) bool {
	if x == nil {
		return false
	}
	switch x.Kind {
	case dsl.ExprAgg:
		return x.Agg != nil && x.Agg.Agg == "count" && countFilterNarrowed(x.Agg.Filter)
	case dsl.ExprOp:
		for i := range x.Args {
			if hasCountWhere(&x.Args[i]) {
				return true
			}
		}
	case dsl.ExprIf:
		return hasCountWhere(x.If) || hasCountWhere(x.Then) || hasCountWhere(x.Else)
	}
	return false
}

// countFilterNarrowed: the AND list has the row-ref condition and a literal one.
func countFilterNarrowed(fs []dsl.Filter) bool {
	var row, lit bool
	for _, f := range fs {
		switch v := f.Value.(type) {
		case map[string]any:
			if len(v) == 1 && v["ref"] == "id" {
				row = true
			}
		case string, float64, bool:
			lit = true
		}
	}
	return row && lit
}

func checkCountWhere(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["countWhere"].(bool)
	if !ok {
		return nil
	}
	got := countWhereFields(a)
	if (len(got) > 0) != want {
		return []string{fmt.Sprintf("countWhere=%v", got)}
	}
	return nil
}
