package dsl

// flashcards (#264): one record of entity per card. Tap flips front / back;
// swiping right (or the 「覚えた」 button) saves true into the boolean toggle
// field and left (「まだ」) saves false, through the same record update as
// list.item.toggle. filter (e.g. toggle == false) narrows the deck to the
// cards not learned yet. Progress and the deck position live in the iOS view
// state only.

func checkFlashcards(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: flashcards entity %q does not exist", where, c.Entity)
		return
	}
	for _, k := range []struct{ name, id string }{{"front", c.Front}, {"back", c.Back}} {
		switch {
		case k.id == "":
			add("%s: flashcards needs %s (a field of entity %q)", where, k.name, e.ID)
		case e.FieldByID(k.id) == nil:
			add("%s: flashcards %s %q not in entity %q (fields: %s)", where, k.name, k.id, e.ID, fieldIDs(e))
		}
	}
	if c.Front != "" && c.Front == c.Back {
		add("%s: flashcards front and back are the same field %q; the back must show the answer (e.g. the meaning)", where, c.Front)
	}
	if c.Toggle != "" {
		f := e.FieldByID(c.Toggle)
		switch {
		case f == nil:
			add("%s: flashcards toggle %q not in entity %q (fields: %s)", where, c.Toggle, e.ID, fieldIDs(e))
		case f.Type != FieldBoolean:
			add("%s: flashcards toggle %q must be a boolean field (got %s); it stores learned / not yet", where, f.ID, f.Type)
		case f.Computed != nil:
			add("%s: flashcards toggle %q is computed (read-only); point it at a stored boolean field", where, f.ID)
		}
	}
	if c.Filter != nil && !hasFilterField(e, c.Filter.Field) {
		add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
	}
}
