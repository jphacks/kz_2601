package dsl

import "fmt"

// Goal / budget read with max / min: a setting entity (settingEntities) that
// is not single keeps one record per "set the goal" (its button opens a new
// record form), and max / min over them is the largest / smallest value ever
// entered, so lowering the budget from 50000 to 30000 still shows 50000
// (iOS Aggregator / ExprEvaluator). Read it with latest, or make it single.

func (l *linter) settingMaxMin() {
	settings := l.settingEntities()
	opened := newRecordForms(l.app)
	// an entered number only: min of an exam date (or of its computed
	// days_left) is the next exam, not a setting
	bad := func(entity, agg, field string) bool {
		e := l.entities[entity]
		if (agg != "max" && agg != "min") || !settings[entity] || e == nil || e.Single || !opened[entity] {
			return false
		}
		f := e.FieldByID(field)
		return f != nil && f.Computed == nil && (f.Type == FieldInteger || f.Type == FieldNumber || f.Type == FieldCurrency)
	}
	report := func(where, entity, agg string) {
		l.add(`%s: %s of %q is the largest/smallest value ever entered, so lowering (or raising) the goal/budget is ignored; make %q "single": true and read it with {"setting":"<fieldId>"}, or use {"agg":"latest","entity":%q,"field":...,"by":"<date field>"} (aggregate "latest" with by)`,
			where, agg, entity, entity, entity)
	}
	exprs := func(where string, x *Expr) {
		walkExpr(x, func(e *Expr) {
			if e.Kind == ExprAgg && e.Agg != nil && bad(e.Agg.Entity, e.Agg.Agg, e.Agg.Field) {
				report(where, e.Agg.Entity, e.Agg.Agg)
			}
		})
	}
	for _, e := range l.app.Entities {
		for _, f := range e.Fields {
			exprs(fmt.Sprintf("entity %q field %q computed", e.ID, f.ID), f.Computed)
		}
	}
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(w string, c *Component) {
			where := fmt.Sprintf("screen %q %s", s.ID, w)
			if scalarTypes[c.Type] && bad(c.Entity, c.Aggregate, c.Field) {
				report(where+" aggregate", c.Entity, c.Aggregate)
			}
			exprs(where+".value", c.Value)
		})
	}
}

// newRecordForms: entities whose form a button or toolbar navigate opens
// (without a tapped record, so each save adds a record).
func newRecordForms(a *App) map[string]bool {
	forms := map[string]string{}
	for _, s := range a.Screens {
		if s.Entity != "" {
			forms[s.ID] = s.Entity
		}
	}
	out := map[string]bool{}
	mark := func(act *Action) {
		if act != nil && act.Type == ActionNavigate && forms[act.Screen] != "" {
			out[forms[act.Screen]] = true
		}
	}
	for i := range a.Screens {
		s := &a.Screens[i]
		for j := range s.Toolbar {
			mark(&s.Toolbar[j].Action)
		}
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if c.Type == CompButton {
				mark(c.Action)
			}
		})
	}
	return out
}
