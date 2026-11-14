package main

import (
	"fmt"
	"slices"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// countWhere expect key, checked by checkCountWhere:
//
//	countWhere (bool)  some computed field counts only the children of this
//	                   record with a given value: agg count whose AND filter
//	                   matches {"ref":"id"} on a ref / refs to this entity and
//	                   compares another field with a literal
//
// Cases: internal/ai/eval/schedule_poll_intent_cases.json (-cases).
func init() {
	expectKeys["countWhere"] = expectBool
}

// countWhereFields lists "entity.field" of computed fields that count a
// parent's children narrowed by a value (docs/dsl/examples/schedule_poll.json).
func countWhereFields(a *dsl.App) []string {
	var out []string
	for i := range a.Entities {
		e := &a.Entities[i]
		for _, f := range e.Fields {
			if f.Computed != nil && hasCountWhere(a, e.ID, f.Computed) {
				out = append(out, e.ID+"."+f.ID)
			}
		}
	}
	return out
}

func hasCountWhere(a *dsl.App, owner string, x *dsl.Expr) bool {
	if x == nil {
		return false
	}
	switch x.Kind {
	case dsl.ExprAgg:
		return x.Agg != nil && x.Agg.Agg == "count" && countFilterNarrowed(a.EntityByID(x.Agg.Entity), owner, x.Agg.Filter)
	case dsl.ExprOp:
		for i := range x.Args {
			if hasCountWhere(a, owner, &x.Args[i]) {
				return true
			}
		}
	case dsl.ExprIf:
		return hasCountWhere(a, owner, x.If) || hasCountWhere(a, owner, x.Then) || hasCountWhere(a, owner, x.Else)
	}
	return false
}

// valueOps compare a field with a value (not a date window like today).
var valueOps = []string{"eq", "neq", "gt", "gte", "lt", "lte"}

// countFilterNarrowed: the AND list ties the counted records to the owner's
// row (a ref / refs to owner matched with {"ref":"id"}) and compares another
// field with a literal.
func countFilterNarrowed(child *dsl.Entity, owner string, fs []dsl.Filter) bool {
	if child == nil {
		return false
	}
	parent := map[string]bool{}
	for _, f := range fs {
		v, ok := f.Value.(map[string]any)
		if !ok || len(v) != 1 || v["ref"] != "id" {
			continue
		}
		if fld := child.FieldByID(f.Field); fld != nil && (fld.Type == dsl.FieldRef || fld.Type == dsl.FieldRefs) && fld.RefEntity == owner {
			parent[f.Field] = true
		}
	}
	if len(parent) == 0 {
		return false
	}
	for _, f := range fs {
		switch f.Value.(type) {
		case string, float64, bool:
			if !parent[f.Field] && slices.Contains(valueOps, f.Op) {
				return true
			}
		}
	}
	return false
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
