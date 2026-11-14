package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// ExprShapeProblems checks an expression without an app around it: op names
// and arity (Ops), agg names, agg field / by, filter ops, period filters
// taking null, nextOccurrence's unit and nesting depth. Names of entities /
// fields / settings and the types they give are not resolved (there is no
// app), so it is never stricter than Parse and never a substitute for it: it
// is for snippets such as the examples the AI prompts teach.
func ExprShapeProblems(e *Expr) []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	var walk func(where string, e *Expr, depth int)
	walkFilter := func(where string, f *Filter, depth int) {
		// Parse rejects these inside agg (component filters only).
		if strings.HasPrefix(f.Field, RecordMarkPrefix) || f.Field == RecordAuthor || isMeValue(f.Value) || isMyFollowsValue(f.Value) {
			add(`%s: %q / {"me":true} / {"myFollows":true} work only as a component's filter, not inside agg`, where, f.Field)
			return
		}
		if !slices.Contains(FilterOps, f.Op) {
			add("%s: filter op %q is not one of %v", where, f.Op, FilterOps)
		}
		if IsPeriodFilterOp(f.Op) {
			if f.Value != nil {
				add("%s: filter op %s takes value null", where, f.Op)
			}
			return
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
			a := e.Agg
			if !slices.Contains(AggregateFuncs, a.Agg) {
				add("%s: agg %q is not one of %v", where, a.Agg, AggregateFuncs)
			}
			if a.Agg != "count" && a.Field == "" {
				add("%s: agg %s needs a field (only count goes without)", where, a.Agg)
			}
			if a.By != "" && !IsPickAgg(a.Agg) {
				add("%s: by is only for agg latest/earliest", where)
			}
			for i := range a.Filter {
				walkFilter(fmt.Sprintf("%s.filter[%d]", where, i), &a.Filter[i], depth)
			}
		case ExprOp:
			spec, ok := Ops[e.Op]
			if !ok {
				add("%s: unknown op %q", where, e.Op)
				return
			}
			n := len(e.Args)
			if msg := spec.arityProblem(e.Op, n); msg != "" {
				add("%s: %s", where, msg)
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
