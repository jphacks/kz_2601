package dsl

import "strings"

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

// ToggleResets are the list.item.resets values (#132).
var ToggleResets = []string{"daily"}

// checkListToggle: list.item.toggle must name a stored boolean field of the
// list's entity (iOS flips and saves it from the row). With resets "daily"
// it names a stored date field instead: iOS saves today's date on check and
// shows the row checked only while that date is today (#132).
func checkListToggle(where string, item *ListItem, e *Entity, add func(string, ...any)) {
	if item == nil {
		return
	}
	daily := item.Resets == "daily"
	if item.Resets != "" && !daily {
		add("%s: list item.resets %q must be one of %s", where, item.Resets, strings.Join(ToggleResets, "|"))
	}
	if item.Toggle == "" {
		if item.Resets != "" {
			add("%s: list item.resets needs item.toggle (a date field that stores the day it was checked)", where)
		}
		return
	}
	f := e.FieldByID(item.Toggle)
	switch {
	case f == nil:
		add("%s: list item.toggle %q not in entity %q (fields: %s)", where, item.Toggle, e.ID, fieldIDs(e))
	case f.Computed != nil:
		add("%s: list item.toggle %q is computed (read-only); point it at a stored field", where, f.ID)
	case daily && f.Type != FieldDate:
		add("%s: list item.toggle %q with resets daily must be a date field (got %s); add a date field such as done_on (no defaultValue) that stores the day it was checked", where, f.ID, f.Type)
	case daily && f.DefaultValue != nil:
		add("%s: list item.toggle %q (resets daily) must not have defaultValue; a new item would start checked", where, f.ID)
	case !daily && f.Type == FieldDate:
		add("%s: list item.toggle %q is a date field; add \"resets\":\"daily\" for a check that comes back unchecked every day, or point it at a boolean field", where, f.ID)
	case !daily && f.Type != FieldBoolean:
		add("%s: list item.toggle %q must be a boolean field (got %s); add a boolean field such as done", where, f.ID, f.Type)
	}
}

// checkHoursBetweenArg: hoursBetween takes "HH:mm" time strings (timePicker
// string fields) or datetimes, never numbers — toNumber("09:30") is not 9.5.
func (c *exprChecker) checkHoursBetweenArg(where string, got exprType) {
	if got == tNum || got == tBool {
		c.add("%s: hoursBetween expects a \"HH:mm\" time string (or a datetime field) but got a %s; pass the time fields directly, e.g. {\"op\":\"hoursBetween\",\"args\":[{\"ref\":\"start\"},{\"ref\":\"end\"}]}", where, got)
	}
}

// MaxDoneInDays bounds doneInDays' n (iOS keeps about a year of history).
const MaxDoneInDays = 366

// DailyToggleFields collects entity id -> field ids used as list.item.toggle
// with resets "daily" on any screen (#132). iOS records the checked days of
// these fields, so streak / doneInDays may read them (#172).
func DailyToggleFields(a *App) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	var visit func(c *Component)
	visit = func(c *Component) {
		if c.Type == CompList && c.Item != nil && c.Item.Resets == "daily" && c.Item.Toggle != "" {
			if out[c.Entity] == nil {
				out[c.Entity] = map[string]bool{}
			}
			out[c.Entity][c.Item.Toggle] = true
		}
		for i := range c.Children {
			visit(&c.Children[i])
		}
	}
	for i := range a.Screens {
		for j := range a.Screens[i].Components {
			visit(&a.Screens[i].Components[j])
		}
	}
	return out
}

// checkDailyHistory: streak / doneInDays read the checked-day history iOS
// keeps for a resets daily toggle, so arg 0 must be a plain ref to that
// field (not a value) and doneInDays' n a number literal 1..366 (#172).
func (c *exprChecker) checkDailyHistory(where string, e *Expr, row *Entity) {
	hint := `e.g. {"op":"streak","args":[{"ref":"taken_on"}]} where taken_on is the list item.toggle with "resets":"daily"`
	if len(e.Args) == 0 || row == nil {
		return // arity / "no current record" already reported
	}
	a := &e.Args[0]
	if a.Kind != ExprRef || a.Ref == "id" || strings.Contains(a.Ref, ".") {
		c.add("%s.%s.args[0]: %s needs {\"ref\": <daily check field>} of this entity (the history of checked days is read from it); %s", where, e.Op, e.Op, hint)
		return
	}
	f := row.FieldByID(a.Ref)
	if f == nil {
		return // reported by the ref check
	}
	switch {
	case f.Type != FieldDate || f.Computed != nil:
		c.add("%s.%s.args[0]: %s needs the stored date field of a daily check (got %q: %s); %s", where, e.Op, e.Op, f.ID, f.Type, hint)
	case c.daily != nil && !c.daily[row.ID][f.ID]:
		c.add("%s.%s.args[0]: %q is not a daily check; %s only works on a field used as list item.toggle with \"resets\":\"daily\" (iOS records the checked days only there). Add that list or drop %s", where, e.Op, f.ID, e.Op, e.Op)
	}
	if e.Op == "doneInDays" && len(e.Args) == 2 {
		if n, ok := e.Args[1].Number(); !ok || n < 1 || n > MaxDoneInDays || n != float64(int(n)) {
			c.add("%s.doneInDays.args[1]: number of days must be an integer literal 1..%d (last 7 days = 7)", where, MaxDoneInDays)
		}
	}
}
