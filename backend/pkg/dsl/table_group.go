package dsl

// Grouped table (#261): table.groupBy + per-column aggregate turns the table
// into a per-group summary (one row per shop with avg score and count). iOS
// groups on the client, like chart.groupBy.

// TableAggregates is the closed set of TableColumn.aggregate values.
var TableAggregates = map[string]bool{"count": true, "sum": true, "avg": true, "min": true, "max": true}

// TableGroupByTypes are the field types a table can be grouped by: values
// that repeat across records. datetime / number / text are (almost) unique
// per record; photo / video / location / refs are not group keys.
var TableGroupByTypes = map[FieldType]bool{
	FieldString: true, FieldEnum: true, FieldRef: true, FieldBoolean: true, FieldInteger: true, FieldDate: true,
}

// checkTableGroup validates groupBy / columns[].aggregate / sort of a table.
// Field existence of columns and sort is checked by checkTable.
func checkTableGroup(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		return
	}
	if c.GroupBy == "" {
		for i, col := range c.Columns {
			if col.Aggregate != "" {
				add("%s: columns[%d] aggregate %q needs table groupBy (the field to group rows by, e.g. the shop); remove aggregate or add groupBy", where, i, col.Aggregate)
			}
		}
		return
	}
	g := e.FieldByID(c.GroupBy)
	if g == nil {
		add("%s: table groupBy %q not in entity %q", where, c.GroupBy, e.ID)
		return
	}
	if !TableGroupByTypes[g.Type] {
		add("%s: table groupBy %q is %s; group by a string / enum / ref / boolean / integer / date field whose value repeats (e.g. the shop name)", where, c.GroupBy, g.Type)
	}
	hasKey, aggCols := false, 0
	for i, col := range c.Columns {
		f := e.FieldByID(col.Field)
		switch {
		case col.Aggregate == "":
			if col.Field == c.GroupBy {
				hasKey = true
			} else {
				add("%s: columns[%d] field %q needs aggregate (count/sum/avg/min/max) in a table grouped by %q; only the groupBy column shows a plain value", where, i, col.Field, c.GroupBy)
			}
		case !TableAggregates[col.Aggregate]:
			add("%s: columns[%d] aggregate %q must be one of count/sum/avg/min/max", where, i, col.Aggregate)
		default:
			aggCols++
			if col.Aggregate != "count" && f != nil && fieldExprType(f.Type) != tNum {
				add("%s: columns[%d] %s needs a numeric field but %q is %s (use count, or a number / integer / currency field)", where, i, col.Aggregate, col.Field, f.Type)
			}
		}
	}
	if !hasKey {
		add("%s: table grouped by %q needs a column {\"field\":%q} to show the group name", where, c.GroupBy, c.GroupBy)
	}
	if aggCols == 0 {
		add("%s: table grouped by %q needs at least one aggregate column (e.g. {\"field\":%q,\"aggregate\":\"count\"})", where, c.GroupBy, c.GroupBy)
	}
	if c.Sort != nil && c.Sort.Field != c.GroupBy && !hasAggColumn(c.Columns, c.Sort.Field) {
		add("%s: sort field %q of a grouped table must be an aggregate column's field or the groupBy field %q", where, c.Sort.Field, c.GroupBy)
	}
}

func hasAggColumn(cols []TableColumn, field string) bool {
	for _, col := range cols {
		if col.Aggregate != "" && col.Field == field {
			return true
		}
	}
	return false
}
