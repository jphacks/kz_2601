package dsl

import "slices"

// export (#421): a button / toolbar action that writes the records of entity
// (optionally narrowed by filter) to a CSV or a PDF report and opens the iOS
// share sheet. Formatting happens on iOS (same text as list / table).

// ExportFormats is the closed set of Action.format values.
var ExportFormats = []string{"csv", "pdf"}

func checkExportAction(where string, act *Action, entities map[string]*Entity, add func(string, ...any)) {
	if !slices.Contains(ExportFormats, act.Format) {
		add("%s: export format %q must be csv or pdf (csv = spreadsheet / Excel, pdf = report)", where, act.Format)
	}
	e := entities[act.Entity]
	if e == nil {
		add("%s: export entity %q does not exist", where, act.Entity)
		return
	}
	seen := map[string]bool{}
	for _, id := range act.Fields {
		f := e.FieldByID(id)
		switch {
		case f == nil:
			add("%s: export field %q not in entity %q (fields: %s)", where, id, e.ID, fieldIDs(e))
		case f.Type == FieldPhoto || f.Type == FieldVideo:
			add("%s: export field %q is %s; a file cannot hold it, leave it out of fields", where, id, f.Type)
		case seen[id]:
			add("%s: export field %q is listed twice", where, id)
		}
		seen[id] = true
	}
	if act.Filter != nil {
		ch := &exprChecker{entities: entities, add: add}
		ch.checkFilter(where+".filter", act.Filter, e, nil, 1)
	}
}
