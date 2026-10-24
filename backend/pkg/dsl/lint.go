package dsl

import (
	"fmt"
	"math"
	"strings"
)

// Lint reports content problems in a document that already passes Validate:
// the app renders, but a value is wrong or a feature is a dead end (quality
// audit #107 / #87). They are warnings, not errors, so stored documents keep
// parsing; the AI pipeline feeds them back for one regeneration.
//
//   - date filter with a literal: gt/gte/lt/lte on a date/datetime field
//     against "now" or a fixed string (iOS compares numbers -> always empty)
//   - entity without an add form (or the form is unreachable) / without a list
//   - expressions that are always a constant (mul by 0, x*0+0-0)
//   - stat label says 今月/今日/this month/today but the filter is missing or
//     another period; 今週/今年 have no filter op
func (a *App) Lint() []string {
	l := &linter{app: a, entities: map[string]*Entity{}}
	for i := range a.Entities {
		l.entities[a.Entities[i].ID] = &a.Entities[i]
	}
	l.computed()
	l.screens()
	l.entityReach()
	return l.out
}

type linter struct {
	app      *App
	entities map[string]*Entity
	out      []string
}

func (l *linter) add(format string, args ...any) { l.out = append(l.out, fmt.Sprintf(format, args...)) }

func (l *linter) computed() {
	for i := range l.app.Entities {
		e := &l.app.Entities[i]
		for _, f := range e.Fields {
			if f.Computed == nil {
				continue
			}
			where := fmt.Sprintf("entity %q field %q computed", e.ID, f.ID)
			l.constExpr(where, f.Computed)
			l.exprFilters(where, f.Computed)
		}
	}
}

func (l *linter) screens() {
	for _, s := range l.app.Screens {
		for i := range s.Components {
			c := &s.Components[i]
			where := fmt.Sprintf("screen %q components[%d]", s.ID, i)
			l.component(where, c)
			for j := range c.Children {
				l.component(fmt.Sprintf("%s.children[%d]", where, j), &c.Children[j])
			}
		}
	}
}

func (l *linter) component(where string, c *Component) {
	if c.Filter != nil {
		l.dateLiteral(where+".filter", c.Filter, l.entities[c.Entity])
	}
	if c.Type == CompStat && c.Value != nil {
		l.constExpr(where+".value", c.Value)
		l.exprFilters(where+".value", c.Value)
	}
	switch c.Type {
	case CompStat, CompProgressBar, CompGauge:
		l.periodLabel(where, c)
	}
}

// ---- date filter literals -------------------------------------------------

const dateFilterHint = `use an expression instead: {"op":"today","args":[]} (today), {"op":"addDays","args":[{"op":"today","args":[]},-6]} (the last 7 days), or a computed integer field such as days_left = {"op":"daysUntil","args":[{"ref":"<date field>"}]} filtered with {"field":"days_left","op":"lte","value":0} (due today or overdue)`

func (l *linter) dateLiteral(where string, f *Filter, e *Entity) {
	if e == nil {
		return
	}
	switch f.Op {
	case "gt", "gte", "lt", "lte":
	default:
		return
	}
	fld := e.FieldByID(f.Field)
	if fld == nil || (fld.Type != FieldDate && fld.Type != FieldDatetime) {
		return
	}
	switch v := f.Value.(type) {
	case string, float64:
		l.add("%s: %s on date field %q compares with the literal %v, which iOS cannot compare (the list/value is always empty); %s", where, f.Op, f.Field, quoteLit(v), dateFilterHint)
	}
}

func quoteLit(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// exprFilters checks the filters inside every agg of an expression.
func (l *linter) exprFilters(where string, e *Expr) {
	walkExpr(e, func(x *Expr) {
		if x.Kind != ExprAgg || x.Agg == nil {
			return
		}
		for i := range x.Agg.Filter {
			l.dateLiteral(fmt.Sprintf("%s agg(%s).filter[%d]", where, x.Agg.Entity, i), &x.Agg.Filter[i], l.entities[x.Agg.Entity])
		}
	})
}

func walkExpr(e *Expr, fn func(*Expr)) {
	if e == nil {
		return
	}
	fn(e)
	for i := range e.Args {
		walkExpr(&e.Args[i], fn)
	}
	walkExpr(e.If, fn)
	walkExpr(e.Then, fn)
	walkExpr(e.Else, fn)
}

// ---- constant expressions -------------------------------------------------

const constHint = `do not fill a value you cannot calculate with a meaningless expression: make it a normal input field (or drop it) and end app.description with "○○は△△で代替しました。"`

func (l *linter) constExpr(where string, e *Expr) {
	zeroMul := false
	walkExpr(e, func(x *Expr) {
		if x.Kind == ExprOp && x.Op == "mul" {
			for i := range x.Args {
				if v, ok := constValue(&x.Args[i]); ok && v == 0 {
					zeroMul = true
				}
			}
		}
	})
	if e.Kind == ExprLiteral {
		return
	}
	if v, ok := constValue(e); ok {
		l.add("%s: the expression is always %v (it does not depend on any record); %s", where, v, constHint)
		return
	}
	if zeroMul {
		l.add("%s: multiplies by 0, so that part is always 0; %s", where, constHint)
	}
}

// constValue folds an expression that does not depend on records, today or
// fields. mul with a constant 0 is 0 whatever the other factors are.
func constValue(e *Expr) (float64, bool) {
	switch e.Kind {
	case ExprLiteral:
		v, ok := e.Lit.(float64)
		return v, ok
	case ExprIf:
		return 0, false
	case ExprOp:
	default:
		return 0, false
	}
	vals := make([]float64, len(e.Args))
	all := true
	for i := range e.Args {
		v, ok := constValue(&e.Args[i])
		if e.Op == "mul" && ok && v == 0 {
			return 0, true
		}
		vals[i], all = v, all && ok
	}
	if !all || len(vals) == 0 {
		return 0, false
	}
	switch e.Op {
	case "add":
		s := 0.0
		for _, v := range vals {
			s += v
		}
		return s, true
	case "mul":
		p := 1.0
		for _, v := range vals {
			p *= v
		}
		return p, true
	case "sub":
		if len(vals) == 2 {
			return vals[0] - vals[1], true
		}
	case "div":
		if len(vals) == 2 && vals[1] != 0 {
			return vals[0] / vals[1], true
		}
	case "neg":
		return -vals[0], true
	case "abs":
		return math.Abs(vals[0]), true
	case "round", "floor", "ceil", "toNumber", "clamp", "mod", "pow", "sqrt":
		// constant either way; the value is only used in the message
		return vals[0], true
	case "min", "max":
		r := vals[0]
		for _, v := range vals[1:] {
			if (e.Op == "min" && v < r) || (e.Op == "max" && v > r) {
				r = v
			}
		}
		return r, true
	}
	return 0, false
}

// ---- period labels --------------------------------------------------------

type period struct {
	op    string // filter op that implements it ("" = none exists)
	words []string
}

var periods = []period{
	{"thisMonth", []string{"今月", "this month", "monthly total"}},
	{"today", []string{"今日", "本日", "today"}},
	{"", []string{"今週", "this week", "今年", "this year", "年間"}},
}

// sincePhrases mean "up to now" (all-time), not the period itself.
var sincePhrases = strings.NewReplacer("今日まで", "", "本日まで", "", "until today", "", "to date", "")

func labelPeriod(label string) (period, string, bool) {
	low := sincePhrases.Replace(strings.ToLower(label))
	for _, p := range periods {
		for _, w := range p.words {
			if strings.Contains(low, w) {
				return p, w, true
			}
		}
	}
	return period{}, "", false
}

// periodLabel: a 今月/今日 label needs a filter, and not one of the other
// period; 今週/今年 need an expression date filter (no op exists).
func (l *linter) periodLabel(where string, c *Component) {
	label := c.Label
	if label == "" {
		label = c.Title
	}
	p, word, ok := labelPeriod(label)
	if !ok {
		return
	}
	var filters []Filter
	if c.Filter != nil {
		filters = append(filters, *c.Filter)
	}
	if c.Type == CompStat && c.Value != nil {
		walkExpr(c.Value, func(x *Expr) {
			if x.Kind == ExprAgg && x.Agg != nil {
				filters = append(filters, x.Agg.Filter...)
			}
		})
	}
	if p.op == "" {
		for _, f := range filters {
			if f.Op != "thisMonth" && f.Op != "today" && f.Value != nil {
				if ve, err := exprFromValue(f.Value); err == nil && ve != nil && ve.Kind != ExprLiteral {
					return // e.g. date >= addDays(today,-6)
				}
			}
		}
		l.add(`%s: label %q says %q but there is no filter op for weeks/years; either filter by an expression ({"field":"<date>","op":"gte","value":{"op":"addDays","args":[{"op":"today","args":[]},-6]}}) and relabel it 直近7日 / last 7 days, or relabel it to what it really shows (全期間 / 合計 / total) and end app.description with "○○は△△で代替しました。"`, where, label, word)
		return
	}
	if len(filters) == 0 {
		l.add(`%s: label %q says %q but the value has no filter, so it is the all-time total; add {"field":"<date field>","op":"%s"} (inside the agg when using value) or relabel it (合計 / total)`, where, label, word, p.op)
		return
	}
	other := map[string]string{"thisMonth": "today", "today": "thisMonth"}[p.op]
	for _, f := range filters {
		if f.Op == p.op {
			return
		}
	}
	for _, f := range filters {
		if f.Op == other {
			l.add(`%s: label %q says %q but the filter is %s; use op %s or fix the label`, where, label, word, other, p.op)
			return
		}
	}
}

// ---- entity reachability --------------------------------------------------

// listTypes show an entity's records one by one.
var listTypes = map[ComponentType]bool{
	CompList: true, CompTable: true, CompTimeline: true, CompCalendar: true, CompMapView: true, CompSettlement: true,
}

func (l *linter) entityReach() {
	forms := map[string][]string{} // entity -> form screen ids
	for _, s := range l.app.Screens {
		if s.Entity != "" {
			forms[s.Entity] = append(forms[s.Entity], s.ID)
		}
	}
	targets := map[string]bool{l.app.InitialScreen: true}
	for _, t := range l.app.App.Tabs {
		targets[t.Screen] = true
	}
	listed := map[string]bool{}
	nav := func(a *Action) {
		if a != nil && a.Type == ActionNavigate {
			targets[a.Screen] = true
		}
	}
	visit := func(c *Component) {
		nav(c.Action)
		nav(c.OnItemTap)
		nav(c.OnDateTap)
		if listTypes[c.Type] {
			listed[c.Entity] = true
		}
	}
	for _, s := range l.app.Screens {
		for i := range s.Toolbar {
			nav(&s.Toolbar[i].Action)
		}
		for i := range s.Components {
			visit(&s.Components[i])
			for j := range s.Components[i].Children {
				visit(&s.Components[i].Children[j])
			}
		}
	}
	for _, e := range l.app.Entities {
		switch fs := forms[e.ID]; {
		case len(fs) == 0:
			l.add(`entity %q has no add form: add a form screen (screen.entity %q with inputs and a saveRecord toolbar button) and a navigate button to it, or remove the entity (a master list such as members must be addable, then referenced with type ref)`, e.ID, e.ID)
		case !anyOf(fs, targets):
			l.add(`entity %q: form screen %q is not reachable; add a toolbar/button navigate to it`, e.ID, fs[0])
		}
		if !listed[e.ID] {
			l.add(`entity %q has no list: show its records with a list or table (with onItemTap to the form) so what users add is visible`, e.ID)
		}
	}
}

func anyOf(ids []string, set map[string]bool) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}
