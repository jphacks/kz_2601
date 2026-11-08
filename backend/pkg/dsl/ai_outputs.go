package dsl

import (
	"sort"
	"strings"
)

// aiButton outputs: task generate shape fields writes each item into its own
// form field of screen.entity (poster text → title / date / place, an answer
// → score). iOS converts the text to the field type (number / date / enum
// option) and leaves the field empty when it cannot; the user still saves.

// aiOutputTypes are the field types an outputs item can fill.
var aiOutputTypes = map[FieldType]bool{
	FieldString: true, FieldText: true, FieldInteger: true, FieldNumber: true,
	FieldDate: true, FieldDatetime: true, FieldEnum: true,
}

func checkAIOutputs(where string, c *Component, form *Entity, add func(string, ...any)) {
	if c.Outputs == nil {
		return
	}
	if c.Task != "generate" || c.Shape != "fields" {
		add("%s: aiButton outputs is only for task generate with shape fields (one form field per item)", where)
		return
	}
	if c.Output != "" {
		add("%s: aiButton outputs and output are exclusive (outputs fills one field per item)", where)
	}
	if len(c.Outputs) == 0 || len(c.Outputs) > MaxAIItems {
		add("%s: aiButton outputs needs 1..%d items", where, MaxAIItems)
		return
	}
	if form == nil {
		add("%s: aiButton outputs needs screen.entity (it fills the form draft)", where)
		return
	}
	items := map[string]bool{}
	for _, it := range c.Items {
		items[strings.TrimSpace(it)] = true
	}
	names := make([]string, 0, len(c.Outputs))
	for name := range c.Outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	used := map[string]string{}
	for _, name := range names {
		id := c.Outputs[name]
		if !items[strings.TrimSpace(name)] {
			add("%s: aiButton outputs key %q is not one of items %q", where, name, c.Items)
		}
		f := form.FieldByID(id)
		switch {
		case f == nil:
			add("%s: aiButton outputs %q → field %q not in entity %q (fields: %s)", where, name, id, form.ID, fieldIDs(form))
			continue
		case f.Computed != nil:
			add("%s: aiButton outputs %q → field %q is computed (read-only)", where, name, id)
		case !aiOutputTypes[f.Type]:
			add("%s: aiButton outputs %q → field %q must be string, text, integer, number, date, datetime or enum (got %s)", where, name, id, f.Type)
		case f.Hidden:
			add("%s: aiButton outputs %q → field %q is hidden (an answer key the AI must not fill)", where, name, id)
		case f.AdminOnly():
			add("%s: aiButton outputs %q → field %q is editableBy admins; members could not save it", where, name, id)
		}
		if prev, ok := used[id]; ok {
			add("%s: aiButton outputs %q and %q both fill field %q", where, prev, name, id)
		}
		used[id] = name
		if c.Input != "" && c.Input == id {
			add("%s: aiButton input and outputs field %q must differ", where, id)
		}
	}
}
