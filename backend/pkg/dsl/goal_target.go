package dsl

import (
	"fmt"
	"strings"
)

// Literal goal on a bar: a progressBar target (or a gauge max) is a number
// literal while the app keeps the goal in its own entity, so editing the
// goal changes nothing on the bar (nav_study: target 120 next to
// goal.target_minutes). A progressBar target can read the goal with
// {"setting"} (progress_target.go); gauge.max is literal-only, so a stat with
// format percent over {"setting"} / agg latest shows the rate instead.
//
// A goal entity is a setting entity (settingEntities) or any entity whose id /
// name says goal / target / budget and that no ref points at (a master such
// as per-category budgets is not one number). Its numeric, non-computed
// field is the goal. A bar is checked only against a goal about its log
// (goalFor), and not against goals kept per key (perKey).

// goalWords mark an entity or field as the number a bar fills to.
var goalWords = []string{"goal", "target", "budget", "目標", "予算"}

func goalLike(s string) bool {
	s = strings.ToLower(s)
	for _, w := range goalWords {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

type goalField struct{ entity, field string }

// perKey: a non-single goal entity keyed by something (an outgoing ref, or an
// enum / string field the bar's log also has, e.g. budget per category) holds
// many goals, not the one number the bar fills to.
func (l *linter) perKey(goal, log string) bool {
	g, e := l.entities[goal], l.entities[log]
	if g == nil || g.Single {
		return false
	}
	for _, f := range g.Fields {
		switch f.Type {
		case FieldRef, FieldRefs:
			return true
		case FieldEnum, FieldString:
			if e != nil && e.FieldByID(f.ID) != nil {
				return true
			}
		}
	}
	return false
}

// goalPairs: "goal entity|field" -> log entities it meets in one expression
// (a stat value or a computed field), e.g. sub(latest goal.yen, sum saving).
func (l *linter) goalPairs() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	visit := func(x *Expr, row string) {
		var goals []string
		logs := map[string]bool{}
		if row != "" {
			logs[row] = true
		}
		walkExpr(x, func(e *Expr) {
			switch {
			case e.Kind == ExprAgg && e.Agg != nil:
				logs[e.Agg.Entity] = true
				goals = append(goals, e.Agg.Entity+"|"+e.Agg.Field)
			case e.Kind == ExprSetting:
				for _, id := range l.app.SingleEntities() {
					if s := l.entities[id]; s != nil && s.FieldByID(e.Setting) != nil {
						goals = append(goals, id+"|"+e.Setting)
						break
					}
				}
			}
		})
		for _, g := range goals {
			if out[g] == nil {
				out[g] = map[string]bool{}
			}
			for log := range logs {
				out[g][log] = true
			}
		}
	}
	for _, e := range l.app.Entities {
		for _, f := range e.Fields {
			visit(f.Computed, e.ID)
		}
	}
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if c.Type == CompStat {
				visit(c.Value, "")
			}
		})
	}
	return out
}

// goalFor: the goal is about the bar's log when their field ids share a word
// (target_minutes / minutes) or an expression reads both.
func goalFor(g goalField, log, field string, pairs map[string]map[string]bool) bool {
	gf, lf := strings.ToLower(g.field), strings.ToLower(field)
	if lf != "" && (strings.Contains(gf, lf) || strings.Contains(lf, gf)) {
		return true
	}
	return pairs[g.entity+"|"+g.field][log]
}

func (l *linter) goalFields() []goalField {
	settings := l.settingEntities()
	refd := map[string]bool{}
	for _, e := range l.app.Entities {
		for _, f := range e.Fields {
			if f.Type == FieldRef || f.Type == FieldRefs {
				refd[f.RefEntity] = true
			}
		}
	}
	var out []goalField
	for _, e := range l.app.Entities {
		named := goalLike(e.ID) || goalLike(e.Name)
		if refd[e.ID] || (!settings[e.ID] && !named) {
			continue
		}
		for _, f := range e.Fields {
			numeric := f.Type == FieldInteger || f.Type == FieldNumber || f.Type == FieldCurrency
			if numeric && f.Computed == nil && (named || goalLike(f.ID) || goalLike(f.Name)) {
				out = append(out, goalField{e.ID, f.ID})
			}
		}
	}
	return out
}

func (l *linter) goalTarget() {
	goals := l.goalFields()
	if len(goals) == 0 {
		return
	}
	pairs := l.goalPairs()
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(w string, c *Component) {
			var key string
			var lit *float64
			switch c.Type {
			case CompProgressBar:
				if v, ok := c.Target.Number(); ok {
					key, lit = "target", &v
				}
			case CompGauge:
				key, lit = "max", c.Max
			default:
				return
			}
			if c.Entity == "" || lit == nil {
				return
			}
			for _, g := range goals {
				if g.entity == c.Entity || l.perKey(g.entity, c.Entity) || !goalFor(g, c.Entity, c.Field, pairs) {
					continue
				}
				e := l.entities[g.entity]
				single := e != nil && e.Single
				if c.Type == CompProgressBar {
					fix := fmt.Sprintf(`set target to {"setting":%q}`, g.field)
					if !single {
						fix = fmt.Sprintf(`make %q "single": true and %s`, g.entity, fix)
					}
					l.add(`screen %q %s: progressBar target is the literal %v while the goal is kept in %s.%s, so changing the goal never moves the bar; %s`,
						s.ID, w, *lit, g.entity, g.field, fix)
					return
				}
				read := goalReadFix(g.entity, g.field, single)
				l.add(`screen %q %s: %s %s is the literal %v while the goal is kept in %s.%s, so changing the goal never moves the bar (%s takes only a number); show the rate as a stat {"format":"percent","value":{"op":"div","args":[<the %s total over %q>,<goal>]}} where <goal> is %s`,
					s.ID, w, c.Type, key, *lit, g.entity, g.field, key, c.Type, c.Entity, read)
				return
			}
		})
	}
}
