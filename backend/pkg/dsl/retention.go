package dsl

// Record retention (#482): Entity.retentionDays makes iOS delete records
// older than that many days. The schema bounds it too; this keeps a DSL that
// skipped schema validation (patches, old saves) from carrying 0 or a negative
// value that iOS would read as "delete everything".

// MaxRetentionDays caps Entity.retentionDays (about ten years).
const MaxRetentionDays = 3650

func checkEntityRetention(e *Entity, add func(string, ...any)) {
	if e.RetentionDays == 0 {
		return
	}
	if e.RetentionDays < 1 || e.RetentionDays > MaxRetentionDays {
		add("entity %q retentionDays %d must be between 1 and %d (omit it to keep records forever)", e.ID, e.RetentionDays, MaxRetentionDays)
	}
}

// RetentionEntities lists entity ids with a retention period, in order.
func (a *App) RetentionEntities() []string {
	var out []string
	for i := range a.Entities {
		if a.Entities[i].RetentionDays > 0 {
			out = append(out, a.Entities[i].ID)
		}
	}
	return out
}
