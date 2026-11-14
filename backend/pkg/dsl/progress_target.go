package dsl

// progressBar target (entity mode denominator): a number > 0, or
// {"setting": "<fieldId>"} so a goal the user edits (a single entity's
// field, #267) moves the bar. iOS shows "—" while the setting is empty or
// <= 0 (no divide by zero).

// checkProgressTarget validates progressBar.target in entity mode.
func checkProgressTarget(where string, t *Expr, entities map[string]*Entity, add func(string, ...any)) {
	if t == nil {
		add("%s: progressBar entity mode needs target > 0", where)
		return
	}
	if v, ok := t.Number(); ok {
		if v <= 0 {
			add("%s: progressBar entity mode needs target > 0", where)
		}
		return
	}
	if t.Kind != ExprSetting {
		add(`%s: progressBar target must be a number or {"setting":"<fieldId>"}`, where)
		return
	}
	_, f, n := SettingField(entities, t.Setting)
	switch {
	case n == 0:
		add(`%s: progressBar target {"setting":%q} is not a field of a single entity`, where, t.Setting)
	case n > 1:
		add(`%s: progressBar target {"setting":%q} is ambiguous: more than one single entity has field %q`, where, t.Setting, t.Setting)
	case !numericField(f.Type):
		add(`%s: progressBar target {"setting":%q} must be an integer, number or currency field`, where, t.Setting)
	}
}

func numericField(t FieldType) bool {
	return t == FieldInteger || t == FieldNumber || t == FieldCurrency
}

// progressTargetDefault lints a setting target whose field has no default
// > 0: the bar shows "—" until the user saves the goal.
func (l *linter) progressTargetDefault() {
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(w string, c *Component) {
			if c.Type != CompProgressBar || c.Target == nil || c.Target.Kind != ExprSetting {
				return
			}
			_, f, n := SettingField(l.entities, c.Target.Setting)
			if n != 1 || f.Computed != nil {
				return
			}
			if v, ok := f.DefaultValue.(float64); ok && v > 0 {
				return
			}
			l.add(`screen %q %s: progressBar target {"setting":%q} has no defaultValue > 0, so the bar shows "—" until the goal is saved; give field %q a defaultValue (e.g. 8000)`,
				s.ID, w, c.Target.Setting, f.ID)
		})
	}
}
