package dsl

// Logic v2 (#108 list.item.toggle, #109 thisWeek/thisYear, hoursBetween).

// PeriodFilterOps are the Filter.op values that select a calendar period
// relative to the device's local "now" (value must be null). thisWeek starts
// on Monday on iOS (see docs/dsl/expressions.md).
var PeriodFilterOps = []string{"today", "thisWeek", "thisMonth", "thisYear"}

// IsPeriodFilterOp reports whether op is one of PeriodFilterOps.
func IsPeriodFilterOp(op string) bool {
	for _, p := range PeriodFilterOps {
		if op == p {
			return true
		}
	}
	return false
}

// checkPeriodFilter: a period filter takes value null and a date/datetime field.
func checkPeriodFilter(where string, f *Filter, e *Entity, add func(string, ...any)) {
	if f.Value != nil {
		add("%s: filter op %s takes value null", where, f.Op)
	}
	if e == nil {
		return
	}
	if fld := e.FieldByID(f.Field); fld != nil && fld.Type != FieldDate && fld.Type != FieldDatetime {
		add("%s: filter op %s needs a date or datetime field but %q is %s", where, f.Op, fld.ID, fld.Type)
	}
}

// checkComponentPeriodFilter applies checkPeriodFilter to a screen component's
// own filter (list / stat / chart / table / ...). Unknown entities are
// reported by the component's own check.
func checkComponentPeriodFilter(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	if c.Filter == nil || !IsPeriodFilterOp(c.Filter.Op) {
		return
	}
	checkPeriodFilter(where+".filter", c.Filter, entities[c.Entity], add)
}

// checkListToggle: list.item.toggle must name a stored boolean field of the
// list's entity (iOS flips and saves it from the row).
func checkListToggle(where string, item *ListItem, e *Entity, add func(string, ...any)) {
	if item == nil || item.Toggle == "" {
		return
	}
	f := e.FieldByID(item.Toggle)
	switch {
	case f == nil:
		add("%s: list item.toggle %q not in entity %q (fields: %s)", where, item.Toggle, e.ID, fieldIDs(e))
	case f.Type != FieldBoolean:
		add("%s: list item.toggle %q must be a boolean field (got %s); add a boolean field such as done", where, f.ID, f.Type)
	case f.Computed != nil:
		add("%s: list item.toggle %q is computed (read-only); point it at a stored boolean field", where, f.ID)
	}
}

// checkHoursBetweenArg: hoursBetween takes "HH:mm" time strings (timePicker
// string fields) or datetimes, never numbers — toNumber("09:30") is not 9.5.
func (c *exprChecker) checkHoursBetweenArg(where string, got exprType) {
	if got == tNum || got == tBool {
		c.add("%s: hoursBetween expects a \"HH:mm\" time string (or a datetime field) but got a %s; pass the time fields directly, e.g. {\"op\":\"hoursBetween\",\"args\":[{\"ref\":\"start\"},{\"ref\":\"end\"}]}", where, got)
	}
}
