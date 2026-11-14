package main

import (
	"fmt"
	"slices"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// rowWindow expect key, checked by checkRowWindow:
//
//	rowWindow (bool)  some computed field aggregates another entity over a
//	                  date window relative to its own row: the agg filter
//	                  compares a date field (gt/gte/lt/lte) with a value that
//	                  refs a date field of the row (e.g. addDays(start,-7))
//
// Cases: internal/ai/eval/adaptive_amount_intent_cases.json (-cases).
func init() {
	expectKeys["rowWindow"] = expectBool
}

// rowWindowFields lists "entity.field" of computed fields with such a window
// (docs/dsl/examples/adaptive_study.json).
func rowWindowFields(a *dsl.App) []string {
	var out []string
	for i := range a.Entities {
		e := &a.Entities[i]
		for _, f := range e.Fields {
			if f.Computed != nil && hasRowWindow(a, e, f.Computed) {
				out = append(out, e.ID+"."+f.ID)
			}
		}
	}
	return out
}

func hasRowWindow(a *dsl.App, owner *dsl.Entity, x *dsl.Expr) bool {
	if x == nil {
		return false
	}
	switch x.Kind {
	case dsl.ExprAgg:
		return x.Agg != nil && aggRowWindow(a.EntityByID(x.Agg.Entity), owner, x.Agg.Filter)
	case dsl.ExprOp:
		for i := range x.Args {
			if hasRowWindow(a, owner, &x.Args[i]) {
				return true
			}
		}
	case dsl.ExprIf:
		return hasRowWindow(a, owner, x.If) || hasRowWindow(a, owner, x.Then) || hasRowWindow(a, owner, x.Else)
	}
	return false
}

var rangeOps = []string{"gt", "gte", "lt", "lte"}

func aggRowWindow(child, owner *dsl.Entity, fs []dsl.Filter) bool {
	if child == nil {
		return false
	}
	for _, f := range fs {
		fld := child.FieldByID(f.Field)
		if fld == nil || !isDateField(fld) || !slices.Contains(rangeOps, f.Op) {
			continue
		}
		if refsOwnDate(owner, f.Value) {
			return true
		}
	}
	return false
}

func isDateField(f *dsl.Field) bool { return f.Type == dsl.FieldDate || f.Type == dsl.FieldDatetime }

// refsOwnDate: the filter value (raw JSON) contains {"ref":"<date field of owner>"}.
func refsOwnDate(owner *dsl.Entity, v any) bool {
	switch t := v.(type) {
	case map[string]any:
		if r, ok := t["ref"].(string); ok {
			if fld := owner.FieldByID(r); fld != nil && isDateField(fld) {
				return true
			}
		}
		for _, c := range t {
			if refsOwnDate(owner, c) {
				return true
			}
		}
	case []any:
		for _, c := range t {
			if refsOwnDate(owner, c) {
				return true
			}
		}
	}
	return false
}

func checkRowWindow(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["rowWindow"].(bool)
	if !ok {
		return nil
	}
	got := rowWindowFields(a)
	if (len(got) > 0) != want {
		return []string{fmt.Sprintf("rowWindow=%v", got)}
	}
	return nil
}
