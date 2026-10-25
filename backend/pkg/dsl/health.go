package dsl

// healthStat / Field.health (#167): device health data. Steps come from the
// iPhone pedometer (CoreMotion), weight from HealthKit; neither needs an entity.

func checkHealthStat(where string, c *Component, add func(string, ...any)) {
	if !HealthMetrics[c.Metric] {
		add("%s: healthStat metric must be steps or weight", where)
	}
	if !HealthPeriods[c.Period] {
		add("%s: healthStat period must be today or week", where)
	}
	if c.Label == "" {
		add("%s: healthStat needs label", where)
	}
	if c.Target != nil && c.Metric != "steps" {
		add("%s: healthStat target is only for metric steps", where)
	}
	if c.Target != nil && *c.Target <= 0 {
		add("%s: healthStat target must be > 0", where)
	}
	if c.Entity != "" || c.Field != "" {
		add("%s: healthStat reads device data; remove entity/field", where)
	}
}

func checkHealthField(where string, f *Field, add func(string, ...any)) {
	if f.Health == "" {
		return
	}
	if !HealthFieldKinds[f.Health] {
		add("%s: health must be weight", where)
		return
	}
	if (f.Type != FieldNumber && f.Type != FieldInteger) || f.Computed != nil {
		add("%s: health weight needs a stored number field (kg)", where)
	}
	if f.Unit != "" && f.Unit != "kg" {
		add("%s: health weight field unit must be kg", where)
	}
}
