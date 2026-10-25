package dsl

import "slices"

// gridTable (#234): a timetable that places entity records on cells by two
// fields (columnField = x, e.g. weekday enum; rowField = y, e.g. period integer).

// Bounds keep the grid readable on a phone.
const (
	MaxGridTableColumns = 14
	MaxGridTableRows    = 24
)

func checkGridTable(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: gridTable entity %q does not exist", where, c.Entity)
		return
	}
	for _, k := range []struct{ name, id string }{{"titleField", c.TitleField}, {"subtitleField", c.SubtitleField}} {
		if k.id != "" && e.FieldByID(k.id) == nil {
			add("%s: gridTable %s %q not in entity %q", where, k.name, k.id, e.ID)
		}
	}
	if c.TitleField == "" {
		add("%s: gridTable needs titleField", where)
	}
	if c.ColumnField != "" && c.ColumnField == c.RowField {
		add("%s: gridTable columnField and rowField must differ", where)
	}

	// columns: enum (options / subset) or string (explicit values)
	if f := e.FieldByID(c.ColumnField); f == nil {
		add("%s: gridTable columnField %q not in entity %q", where, c.ColumnField, e.ID)
	} else {
		switch f.Type {
		case FieldEnum:
			checkGridValues(where, "columnValues", c.ColumnValues, f, MaxGridTableColumns, add)
		case FieldString:
			if len(c.ColumnValues) == 0 {
				add("%s: gridTable columnField %q is string; list the columns in columnValues", where, f.ID)
			}
			checkGridValues(where, "columnValues", c.ColumnValues, nil, MaxGridTableColumns, add)
		default:
			add("%s: gridTable columnField %q must be enum or string (got %s)", where, f.ID, f.Type)
		}
	}

	// rows: integer (rowRange) or enum / string (rowValues)
	f := e.FieldByID(c.RowField)
	if f == nil {
		add("%s: gridTable rowField %q not in entity %q", where, c.RowField, e.ID)
		return
	}
	switch f.Type {
	case FieldInteger:
		if len(c.RowValues) > 0 {
			add("%s: gridTable rowValues is for enum/string rowField; use rowRange for integer %q", where, f.ID)
		}
		if r := c.RowRange; r != nil {
			if r.To < r.From {
				add("%s: gridTable rowRange to (%d) must be >= from (%d)", where, r.To, r.From)
			} else if r.To-r.From+1 > MaxGridTableRows {
				add("%s: gridTable rowRange has %d rows (max %d)", where, r.To-r.From+1, MaxGridTableRows)
			}
		}
	case FieldEnum, FieldString:
		if c.RowRange != nil {
			add("%s: gridTable rowRange is only for integer rowField; %q is %s", where, f.ID, f.Type)
		}
		if f.Type == FieldString {
			if len(c.RowValues) == 0 {
				add("%s: gridTable rowField %q is string; list the rows in rowValues", where, f.ID)
			}
			checkGridValues(where, "rowValues", c.RowValues, nil, MaxGridTableRows, add)
		} else {
			checkGridValues(where, "rowValues", c.RowValues, f, MaxGridTableRows, add)
		}
	default:
		add("%s: gridTable rowField %q must be integer, enum or string (got %s)", where, f.ID, f.Type)
	}
}

// checkGridValues rejects blanks / duplicates / too many headers and, for an
// enum field, values outside its options (the cell would never match).
func checkGridValues(where, key string, vals []string, enum *Field, max int, add func(string, ...any)) {
	n := len(vals)
	if n == 0 && enum != nil {
		n = len(enum.Options)
	}
	if n > max {
		add("%s: gridTable has %d %s (max %d)", where, n, key, max)
	}
	seen := map[string]bool{}
	for _, v := range vals {
		switch {
		case v == "":
			add("%s: gridTable %s has an empty value", where, key)
		case seen[v]:
			add("%s: gridTable %s has duplicate %q", where, key, v)
		case enum != nil && !slices.Contains(enum.Options, v):
			add("%s: gridTable %s %q is not an option of %q", where, key, v, enum.ID)
		}
		seen[v] = true
	}
}
