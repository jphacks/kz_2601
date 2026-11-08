package dsl

// bracket: a single-elimination tournament drawn from the records of entity
// (one record per entrant). iOS pairs round 1 by seedField (low first) or
// creation order, with byes for the top seeds up to the next power of two
// (MaxBracketEntrants at most), and advances whoever has won more rounds:
// winsField holds the number of rounds an entrant has won. Tapping an
// entrant in a match writes that number through the same record update as
// list.item.toggle, so only those who may edit the records set winners. The
// pairing itself is never stored.

// MaxBracketEntrants bounds the drawn entrants (6 rounds); iOS leaves out the rest.
const MaxBracketEntrants = 64

func checkBracket(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: bracket entity %q does not exist", where, c.Entity)
		return
	}
	if e.Single {
		add("%s: bracket entity %q is single (one record); entrants need one record each", where, e.ID)
	}
	switch {
	case c.TitleField == "":
		add("%s: bracket needs titleField (the entrant's name field of %q)", where, e.ID)
	case e.FieldByID(c.TitleField) == nil:
		add("%s: bracket titleField %q not in entity %q (fields: %s)", where, c.TitleField, e.ID, fieldIDs(e))
	}
	switch f := e.FieldByID(c.WinsField); {
	case c.WinsField == "":
		add("%s: bracket needs winsField (an integer field of %q holding the rounds won, defaultValue 0)", where, e.ID)
	case f == nil:
		add("%s: bracket winsField %q not in entity %q (fields: %s)", where, c.WinsField, e.ID, fieldIDs(e))
	case f.Type != FieldInteger:
		add("%s: bracket winsField %q must be an integer field (got %s); it stores the rounds won", where, f.ID, f.Type)
	case f.Computed != nil:
		add("%s: bracket winsField %q is computed (read-only); point it at a stored integer field", where, f.ID)
	}
	if c.SeedField != "" {
		switch f := e.FieldByID(c.SeedField); {
		case f == nil:
			add("%s: bracket seedField %q not in entity %q (fields: %s)", where, c.SeedField, e.ID, fieldIDs(e))
		case f.Type != FieldInteger && f.Type != FieldNumber:
			add("%s: bracket seedField %q must be integer or number (got %s); omit it to seed by creation order", where, f.ID, f.Type)
		}
	}
	if c.WinsField != "" && c.WinsField == c.SeedField {
		add("%s: bracket winsField and seedField are the same field %q", where, c.WinsField)
	}
}
