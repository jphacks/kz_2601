package dsl

// randomPicker (#263): a button that picks one record of entity at random
// (optionally narrowed by filter) and shows its titleField. The pick lives in
// the iOS view state only: nothing is saved and re-rendering keeps it, so the
// randomness never leaks into Expr (which must stay deterministic).
// Shared fields: Entity / Title / TitleField / SubtitleField / Filter / Label / OnItemTap.

func checkRandomPicker(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: randomPicker entity %q does not exist", where, c.Entity)
		return
	}
	if c.TitleField == "" {
		add("%s: randomPicker needs titleField", where)
	}
	for _, k := range []struct{ name, id string }{{"titleField", c.TitleField}, {"subtitleField", c.SubtitleField}} {
		if k.id != "" && e.FieldByID(k.id) == nil {
			add("%s: randomPicker %s %q not in entity %q", where, k.name, k.id, e.ID)
		}
	}
	if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
		add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
	}
}
