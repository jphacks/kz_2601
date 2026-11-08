package dsl

import (
	"fmt"
	"slices"
)

// ExprShapeProblems checks an expression without an app around it: op names
// and arity (Ops), agg names, filter ops, period filters taking null,
// nextOccurrence's unit and nesting depth. Names of entities / fields /
// settings are not resolved (there is no app), so it is for snippets such as
// the examples the AI prompts teach.
func ExprShapeProblems(e *Expr) []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	var walk func(where string, e *Expr, depth int)
	walkFilter := func(where string, f *Filter, depth int) {
		if !slices.Contains(FilterOps, f.Op) {
			add("%s: filter op %q is not one of %v", where, f.Op, FilterOps)
		}
		if IsPeriodFilterOp(f.Op) {
			if f.Value != nil {
				add("%s: filter op %s takes value null", where, f.Op)
			}
			return
		}
		if isMeValue(f.Value) || isMyFollowsValue(f.Value) {
			return // a component filter's {"me":true} / {"myFollows":true}
		}
		ve, err := exprFromValue(f.Value)
		if err != nil {
			add("%s: filter value: %v", where, err)
			return
		}
		if ve != nil && ve.Kind != ExprLiteral {
			walk(where+".value", ve, depth+1)
		}
	}
	walk = func(where string, e *Expr, depth int) {
		if e == nil {
			add("%s: expression is missing", where)
			return
		}
		if depth > MaxExprDepth {
			add("%s: nested deeper than %d levels", where, MaxExprDepth)
			return
		}
		switch e.Kind {
		case ExprAgg:
			if !slices.Contains(AggregateFuncs, e.Agg.Agg) {
				add("%s: agg %q is not one of %v", where, e.Agg.Agg, AggregateFuncs)
			}
			for i := range e.Agg.Filter {
				walkFilter(fmt.Sprintf("%s.filter[%d]", where, i), &e.Agg.Filter[i], depth)
			}
		case ExprOp:
			spec, ok := Ops[e.Op]
			if !ok {
				add("%s: unknown op %q", where, e.Op)
				return
			}
			n := len(e.Args)
			if n < spec.minArgs || (spec.maxArgs >= 0 && n > spec.maxArgs) {
				add("%s: op %s takes %d..%d args (got %d; -1 = any)", where, e.Op, spec.minArgs, spec.maxArgs, n)
			}
			if e.Op == "nextOccurrence" && n == 2 && !IsRepeatUnit(e.Args[1]) {
				add(`%s: nextOccurrence args[1] must be "monthly" or "yearly"`, where)
			}
			for i := range e.Args {
				walk(fmt.Sprintf("%s.%s.args[%d]", where, e.Op, i), &e.Args[i], depth+1)
			}
		case ExprIf:
			walk(where+".if", e.If, depth+1)
			walk(where+".then", e.Then, depth+1)
			walk(where+".else", e.Else, depth+1)
		}
	}
	walk("expr", e, 1)
	return out
}
