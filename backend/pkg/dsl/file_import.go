package dsl

import (
	"fmt"
	"slices"
)

// fileImport: a button that opens the iOS file picker and adds records to
// entity from the chosen file, on the device (the file itself is never sent
// or kept; only the added records sync as usual).
//   - csv: one record per row. The header row is matched to fields by map,
//     then by field name / id (a CSV written by export reads back as is).
//   - text / pdf: one record whose field gets the body text (PDF text layer,
//     or on-device OCR of scanned pages); the entity's titleField gets the
//     file name.
// iOS shows how many rows will be added / fail before adding anything.

// FileImportAccepts is the closed set of fileImport.accept values.
var FileImportAccepts = []string{"csv", "text", "pdf"}

// MaxImportMap bounds fileImport.map.
const MaxImportMap = 50

// importFillsByDefault: field types iOS fills when the file has no value
// (false / the first option / now), as on a new form.
var importFillsByDefault = map[FieldType]bool{FieldBoolean: true, FieldEnum: true, FieldDate: true, FieldDatetime: true}

func checkFileImport(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: fileImport entity %q does not exist", where, c.Entity)
		return
	}
	switch {
	case e.EffectiveScope() == ScopePublic:
		add("%s: fileImport cannot add to public entity %q (a file of rows would be posted to everyone); use a private or shared entity", where, e.ID)
	case e.Single:
		add("%s: fileImport cannot add to single entity %q (it holds one settings record)", where, e.ID)
	case e.Match != "":
		add("%s: fileImport cannot add to match entity %q", where, e.ID)
	}
	csv, body := checkImportAccept(where, c.Accept, add)
	if body {
		checkImportBody(where, c, e, add)
	} else if c.Field != "" {
		add("%s: fileImport field %q is only for accept text / pdf (the body text goes there); csv maps columns by header or map", where, c.Field)
	}
	if len(c.Map) > 0 && !csv {
		add("%s: fileImport map is only for accept csv", where)
	}
	checkImportMap(where, c.Map, e, add)
}

// checkImportAccept reports whether accept has csv and text / pdf.
func checkImportAccept(where string, accept []string, add func(string, ...any)) (csv, body bool) {
	if len(accept) == 0 {
		add("%s: fileImport needs accept (one or more of csv, text, pdf)", where)
	}
	seen := map[string]bool{}
	for _, a := range accept {
		switch {
		case !slices.Contains(FileImportAccepts, a):
			add("%s: fileImport accept %q must be csv, text or pdf", where, a)
		case seen[a]:
			add("%s: fileImport accept %q is listed twice", where, a)
		case a == "csv":
			csv = true
		default:
			body = true
		}
		seen[a] = true
	}
	return csv, body
}

// checkImportBody: text / pdf put the body into a stored string / text field
// and the file name into titleField; every other required field must fill
// itself.
func checkImportBody(where string, c *Component, e *Entity, add func(string, ...any)) {
	f := e.FieldByID(c.Field)
	switch {
	case c.Field == "":
		add("%s: fileImport with accept text / pdf needs field (a text field of entity %q for the body)", where, e.ID)
	case f == nil:
		add("%s: fileImport field %q not in entity %q (fields: %s)", where, c.Field, e.ID, fieldIDs(e))
	case f.Computed != nil:
		add("%s: fileImport field %q is computed (read-only); point it at a stored text field", where, f.ID)
	case f.Type != FieldText && f.Type != FieldString:
		add("%s: fileImport field %q must be text (up to 20000 characters) or string (4000), got %s", where, f.ID, f.Type)
	}
	title := e.FieldByID(e.TitleField)
	for i := range e.Fields {
		x := &e.Fields[i]
		if !x.Required || x.Computed != nil || x.DefaultValue != nil || importFillsByDefault[x.Type] || x.ID == c.Field {
			continue
		}
		if title != nil && x.ID == title.ID && (x.Type == FieldString || x.Type == FieldText) {
			continue // the file name
		}
		add("%s: fileImport text / pdf fills only field %q and the title; required field %q of entity %q would stay empty (drop required or give it a defaultValue)", where, c.Field, x.ID, e.ID)
	}
}

func checkImportMap(where string, m []ImportColumn, e *Entity, add func(string, ...any)) {
	if len(m) > MaxImportMap {
		add("%s: fileImport map has %d entries (max %d)", where, len(m), MaxImportMap)
	}
	columns, fields := map[string]bool{}, map[string]bool{}
	for i, col := range m {
		at := fmt.Sprintf("%s.map[%d]", where, i)
		f := e.FieldByID(col.Field)
		switch {
		case col.Column == "":
			add("%s: column is empty (the CSV header text)", at)
		case columns[col.Column]:
			add("%s: column %q is listed twice", at, col.Column)
		}
		columns[col.Column] = true
		switch {
		case f == nil:
			add("%s: field %q not in entity %q (fields: %s)", at, col.Field, e.ID, fieldIDs(e))
		case f.Computed != nil:
			add("%s: field %q is computed (read-only)", at, f.ID)
		case f.Type == FieldPhoto || f.Type == FieldVideo || f.Type == FieldAudio:
			add("%s: field %q is %s; a CSV cell cannot hold it", at, f.ID, f.Type)
		case f.Type == FieldMember || f.Type == FieldMembers:
			add("%s: field %q is %s (participant uids); a CSV cell cannot hold it", at, f.ID, f.Type)
		case fields[f.ID]:
			add("%s: field %q is mapped twice", at, f.ID)
		}
		fields[col.Field] = true
	}
}
