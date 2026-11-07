package dsl

// aiButton saveTo: task generate shape list may save each generated item as
// its own record of entity (quiz questions, flashcards, steps, ideas). iOS
// shows the list first and the user saves all or one by one, through the
// usual record write path (permissions, plan limits, text filter apply).
// At most MaxAIItems records per run (count is capped there).

// AISaveTo names where each list item goes: entity.field (string / text).
type AISaveTo struct {
	Entity string `json:"entity"`
	Field  string `json:"field"`
}

func checkAISaveTo(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	s := c.SaveTo
	if s == nil {
		return
	}
	if c.Task != "generate" || c.Shape != "list" {
		add("%s: aiButton saveTo is only for task generate with shape list (one record per item)", where)
		return
	}
	if c.Output != "" {
		add("%s: aiButton saveTo and output are exclusive (saveTo adds records; output fills the form draft)", where)
	}
	e := entities[s.Entity]
	if e == nil {
		add("%s: aiButton saveTo entity %q does not exist", where, s.Entity)
		return
	}
	switch {
	case e.EffectiveScope() == ScopePublic:
		add("%s: aiButton saveTo cannot add to public entity %q (AI items would be posted to everyone); use a private or shared entity", where, e.ID)
	case e.Single:
		add("%s: aiButton saveTo cannot add to single entity %q (it holds one settings record)", where, e.ID)
	case e.Match != "":
		add("%s: aiButton saveTo cannot add to match entity %q", where, e.ID)
	case e.IsAddressed():
		add("%s: aiButton saveTo cannot add to addressed entity %q (its records need a recipient)", where, e.ID)
	}
	f := e.FieldByID(s.Field)
	switch {
	case s.Field == "":
		add("%s: aiButton saveTo needs field (a string / text field of entity %q)", where, e.ID)
		return
	case f == nil:
		add("%s: aiButton saveTo field %q not in entity %q (fields: %s)", where, s.Field, e.ID, fieldIDs(e))
		return
	case f.Computed != nil:
		add("%s: aiButton saveTo field %q is computed (read-only)", where, f.ID)
	case f.Type != FieldString && f.Type != FieldText:
		add("%s: aiButton saveTo field %q must be string or text (got %s)", where, f.ID, f.Type)
	case f.AdminOnly():
		add("%s: aiButton saveTo field %q is editableBy admins; members could not save it", where, f.ID)
	}
	for i := range e.Fields {
		x := &e.Fields[i]
		if !x.Required || x.Computed != nil || x.DefaultValue != nil || importFillsByDefault[x.Type] || x.ID == s.Field {
			continue
		}
		add("%s: aiButton saveTo fills only field %q; required field %q of entity %q would stay empty (drop required or give it a defaultValue)", where, s.Field, x.ID, e.ID)
	}
}
