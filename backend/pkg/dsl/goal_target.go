package dsl

import (
	"fmt"
	"strings"
)

// Literal goal on a bar: a progressBar target (or a gauge max) is a number
// literal while the app keeps the goal in its own entity, so editing the
// goal changes nothing on the bar (nav_study: target 120 next to
// goal.target_minutes). progressBar.target / gauge.max are literal-only
// (expressions.md: value/target as Expr deferred), so the bar cannot follow
// the goal; a stat with format percent over {"setting"} / agg latest can.
//
// A goal entity is a setting entity (settingEntities) or any entity whose id /
// name says goal / target / budget and that no ref points at (a master such
// as per-category budgets is not one number). Its numeric, non-computed
// field is the goal.

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
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(w string, c *Component) {
			var key string
			var lit *float64
			switch c.Type {
			case CompProgressBar:
				key, lit = "target", c.Target
			case CompGauge:
				key, lit = "max", c.Max
			default:
				return
			}
			if c.Entity == "" || lit == nil {
				return
			}
			for _, g := range goals {
				if g.entity == c.Entity {
					continue
				}
				read := fmt.Sprintf(`{"setting":%q}`, g.field)
				if e := l.entities[g.entity]; e == nil || !e.Single {
					read = fmt.Sprintf(`{"agg":"latest","entity":%q,"field":%q,"by":"<date field>"} (or make %q "single": true and use {"setting":%q})`, g.entity, g.field, g.entity, g.field)
				}
				l.add(`screen %q %s: %s %s is the literal %v while the goal is kept in %s.%s, so changing the goal never moves the bar (%s takes only a number); show the rate as a stat {"format":"percent","value":{"op":"div","args":[<the %s total over %q>,<goal>]}} where <goal> is %s`,
					s.ID, w, c.Type, key, *lit, g.entity, g.field, key, c.Type, c.Entity, read)
				return
			}
		})
	}
}
