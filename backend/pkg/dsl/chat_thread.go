package dsl

// Chat (#260): chatThread shows an entity's records as chat bubbles, oldest
// at the top, mine on the right (Record.author == the signed-in uid) and the
// others on the left with $author. Its pinned input bar saves one record per
// message, so the entity needs no form screen. Order and sender come from the
// record values $createdAt / $author, never from fields.

// checkChatThread: textField is a stored string/text field, photoField a
// stored photo field, and every other required field has a defaultValue
// (messages are saved without a form, so nobody can fill it).
func checkChatThread(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: chatThread entity %q does not exist", where, c.Entity)
		return
	}
	if f := e.FieldByID(c.TextField); f == nil {
		add("%s: chatThread textField %q not in entity %q (fields: %s)", where, c.TextField, e.ID, fieldIDs(e))
	} else if (f.Type != FieldString && f.Type != FieldText) || f.Computed != nil {
		add("%s: chatThread textField %q must be a stored string or text field (the message body)", where, c.TextField)
	}
	if c.PhotoField != "" {
		if f := e.FieldByID(c.PhotoField); f == nil {
			add("%s: chatThread photoField %q not in entity %q (fields: %s)", where, c.PhotoField, e.ID, fieldIDs(e))
		} else if f.Type != FieldPhoto || f.Computed != nil {
			add("%s: chatThread photoField %q must be a stored photo field", where, c.PhotoField)
		}
	}
	for _, f := range e.Fields {
		if f.Required && f.Computed == nil && f.DefaultValue == nil && f.ID != c.TextField && f.ID != c.PhotoField {
			add("%s: chatThread sends messages without a form, so required field %q of entity %q needs a defaultValue (or make it optional; the sender and time are $author / $createdAt, not fields)", where, f.ID, e.ID)
		}
	}
}

// chatEntities: entities a chatThread on a reachable screen writes.
func chatEntities(a *App, reach map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, s := range a.Screens {
		if !reach[s.ID] {
			continue
		}
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if c.Type == CompChatThread {
				out[c.Entity] = true
			}
		})
	}
	return out
}

// privateChat (lint): a chatThread on a private entity only ever shows the
// user's own messages (the others cannot read them).
func (l *linter) privateChat() {
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if c.Type != CompChatThread {
				return
			}
			if e := l.app.EntityByID(c.Entity); e != nil && !e.IsShared() {
				l.add(`screen %q: chatThread entity %q is private, so nobody else sees the messages; set "scope": "public" for a chat with other people`, s.ID, e.ID)
			}
		})
	}
}
