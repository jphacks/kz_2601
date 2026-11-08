package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// Server rules that read more than the trigger record (#340, decisions #336
// 「後で」: agg in conditions / values, a recount on a time trigger):
//
//   - agg in if / values / by / notify.with: the records of an entity of the
//     same app on the rule's side (private with private, shared / public
//     with shared / public).
//   - on.each (time trigger): the rule runs once per record of an entity
//     (that record is the row), e.g. a reminder per task due in 3 days or a
//     rank written to each player.
//   - notify.with: named number / date values inserted as {name}, e.g. the
//     weekly total of a time-triggered summary.
//
// Reads cost Firestore reads in proportion to the records, so the runner caps
// them per run and per app per day (quota RunReads* / AppReads*) and stops
// the run (never computes on part of the records) when a cap is hit.

// Read limits checked at generation time.
const (
	// MaxRuleReadEntities: entities one rule lists (each + agg entities).
	MaxRuleReadEntities = 2
	// MaxNotifyWith: named values of one notification.
	MaxNotifyWith = 3
)

// RowEntity is the entity of the rule's row: the trigger entity, or on.each
// ("" for a time trigger without each).
func (r *ServerRule) RowEntity() string {
	if r.On.Each != "" {
		return r.On.Each
	}
	return r.On.Entity
}

// checkServerAgg validates agg inside a server rule: the entity must be on the
// rule's side; it is recorded as read (checkAgg validates the rest).
func (c *exprChecker) checkServerAgg(where string, a *AggExpr) {
	if a == nil {
		return
	}
	e := c.entities[a.Entity]
	if e == nil {
		return // reported by checkAgg
	}
	if e.IsShared() != (c.serverScope != ScopePrivate) {
		c.add("%s.agg: entity %q is %s but the rule works on %s data; agg reads only records on the same side (private with private, shared / public with shared / public)", where, e.ID, e.EffectiveScope(), c.serverScope)
	}
	if c.serverReads != nil {
		c.serverReads[e.ID] = true
	}
}

// checkNotifyWith validates notify.with: at most MaxNotifyWith names, each an
// id that is not a field of the row, a number or date value, used in the
// title or body.
func checkNotifyWith(where string, rc *ruleCheck, n *RuleNotify) {
	if len(n.With) == 0 {
		return
	}
	add := rc.add
	if len(n.With) > MaxNotifyWith {
		add("%s.with: too many values (%d > %d)", where, len(n.With), MaxNotifyWith)
	}
	used := map[string]bool{}
	for _, s := range []string{n.Title, n.Body} {
		for _, m := range notifyPlaceholderRe.FindAllStringSubmatch(s, -1) {
			used[m[1]] = true
		}
	}
	for _, name := range sortedKeys(n.With) {
		w := fmt.Sprintf("%s.with.%s", where, name)
		switch {
		case !idRe.MatchString(name):
			add("%s: %q is not a valid name (lowercase id)", w, name)
		case rc.actRow() != nil && rc.actRow().FieldByID(name) != nil:
			add("%s: %q is a field of entity %q; give the value another name", w, name, rc.actRow().ID)
		case !used[name]:
			add("%s: {%s} is not used in title or body", w, name)
		}
		v := n.With[name]
		ch := rc.expr()
		got := ch.check(w, &v, rc.actRow(), 1)
		checkNoComputed(w, ch, add)
		if g := widen(got); g != tNum && g != tDate {
			add("%s: must be a number or a date (got %s); insert record text with {<field>} instead", w, got)
		}
	}
}

// ReadEntities returns the entities whose records the rule lists (on.each,
// every agg, the lookup, similar and pick), sorted and unique.
func (r *ServerRule) ReadEntities() []string {
	seen := map[string]bool{}
	if r.On.Each != "" {
		seen[r.On.Each] = true
	}
	if r.Lookup != nil && r.Lookup.Entity != "" {
		seen[r.Lookup.Entity] = true
	}
	if r.Similar != nil && r.Similar.Entity != "" {
		seen[r.Similar.Entity] = true
	}
	if r.Pick != nil && r.Pick.Entity != "" {
		seen[r.Pick.Entity] = true
		if r.Pick.Exclude != nil && r.Pick.Exclude.Entity != "" {
			seen[r.Pick.Exclude.Entity] = true
		}
	}
	r.walkExprs(func(e *Expr) {
		if e.Kind == ExprAgg && e.Agg != nil && e.Agg.Entity != "" {
			seen[e.Agg.Entity] = true
		}
	})
	return sortedKeys(seen)
}

// AggEntities returns the entities the rule's aggs read, sorted and unique.
func (r *ServerRule) AggEntities() []string {
	seen := map[string]bool{}
	r.walkExprs(func(e *Expr) {
		if e.Kind == ExprAgg && e.Agg != nil && e.Agg.Entity != "" {
			seen[e.Agg.Entity] = true
		}
	})
	return sortedKeys(seen)
}

// walkExprs calls fn on every expression of the rule (if, values, by,
// notify.with) and every sub-expression, agg filter values included (they
// are evaluated in the row's context).
func (r *ServerRule) walkExprs(fn func(*Expr)) {
	walk := func(e *Expr) { visitExpr(e, fn) }
	walk(r.If)
	if r.Pick != nil {
		for _, f := range r.Pick.Filter { // values read the row, like agg filters
			if v, err := exprFromValue(f.Value); err == nil && v != nil && v.Kind != ExprLiteral {
				walk(v)
			}
		}
	}
	for _, act := range r.Do {
		switch {
		case act.Notify != nil:
			for _, k := range sortedKeys(act.Notify.With) {
				v := act.Notify.With[k]
				walk(&v)
			}
		case act.CreateRecord != nil:
			for _, k := range sortedKeys(act.CreateRecord.Values) {
				v := act.CreateRecord.Values[k]
				walk(&v)
			}
		case act.UpdateRecord != nil:
			for _, k := range sortedKeys(act.UpdateRecord.Values) {
				v := act.UpdateRecord.Values[k]
				walk(&v)
			}
		case act.Increment != nil:
			walk(act.Increment.By)
		}
	}
}

func visitExpr(e *Expr, fn func(*Expr)) {
	if e == nil {
		return
	}
	fn(e)
	for i := range e.Args {
		visitExpr(&e.Args[i], fn)
	}
	visitExpr(e.If, fn)
	visitExpr(e.Then, fn)
	visitExpr(e.Else, fn)
	if e.Kind == ExprAgg && e.Agg != nil {
		for _, f := range e.Agg.Filter {
			if v, err := exprFromValue(f.Value); err == nil && v != nil && v.Kind != ExprLiteral {
				visitExpr(v, fn)
			}
		}
	}
}

// NotifyPlaceholderNames returns the {…} names of a notification template.
func NotifyPlaceholderNames(s string) []string {
	var out []string
	for _, m := range notifyPlaceholderRe.FindAllStringSubmatch(s, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// describeReads is the plain sentence part for a rule that reads records
// ("「支出」を集計して"), "" when it reads none beyond the row.
func (a *App) describeReads(r *ServerRule) string {
	ids := r.AggEntities()
	if len(ids) == 0 {
		return ""
	}
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = id
		if e := a.EntityByID(id); e != nil {
			names[i] = e.Name
		}
		names[i] = "「" + names[i] + "」"
	}
	return strings.Join(names, "・") + "を集計して"
}
