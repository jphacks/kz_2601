package dsl

import (
	"fmt"
	"strings"
)

// Hidden fields (#262): a quiz's answer key. The author enters it in the
// form; lists, tables, charts and computed results never show it, but a
// computed may compare against it (is_correct = eq(choice, question.correct)).
// This is display-level only: the value still reaches every device (strict
// server-side hiding is later work).

// compareOps may read a hidden value: the result says only "matches or not".
var compareOps = map[string]bool{"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true}

const hiddenHint = "show a computed result instead (e.g. is_correct = {\"op\":\"eq\",\"args\":[{\"ref\":\"choice\"},{\"ref\":\"question.correct\"}]})"

func checkHiddenFields(a *App, entities map[string]*Entity, add func(string, ...any)) {
	hidden := func(entityID, fieldID string) bool {
		e := entities[entityID]
		if e == nil {
			return false
		}
		f := e.FieldByID(fieldID)
		return f != nil && f.Hidden
	}
	for i := range a.Entities {
		e := &a.Entities[i]
		for j := range e.Fields {
			f := &e.Fields[j]
			where := fmt.Sprintf("entity %q field %q", e.ID, f.ID)
			if f.Hidden && f.Computed != nil {
				add("%s: a computed field cannot be hidden; hide the stored field it reads (the answer key) instead", where)
			}
			if f.Computed != nil {
				checkHiddenExpr(where, f.Computed, e, entities, false, add)
			}
		}
		if e.TitleField != "" && hidden(e.ID, e.TitleField) {
			add("entity %q titleField %q is hidden; pickers and lists show the titleField, so use a visible field (e.g. the question text)", e.ID, e.TitleField)
		}
	}

	var walk func(where string, c *Component)
	walk = func(where string, c *Component) {
		if c.Entity != "" {
			for _, ref := range hiddenDisplayRefs(c) {
				if hidden(c.Entity, ref.field) {
					add("%s: %s %q of entity %q is hidden (only the author enters it in the form); do not show it to others — %s", where, ref.key, ref.field, c.Entity, hiddenHint)
				}
			}
		}
		if c.Type == CompStat && c.Value != nil {
			checkHiddenExpr(where+".value", c.Value, nil, entities, false, add)
		}
		for i := range c.Children {
			walk(fmt.Sprintf("%s.children[%d]", where, i), &c.Children[i])
		}
	}
	for _, s := range a.Screens {
		for i := range s.Components {
			walk(fmt.Sprintf("screen %q components[%d]", s.ID, i), &s.Components[i])
		}
	}
}

type displayRef struct{ key, field string }

// hiddenDisplayRefs lists the fields a component bound to an entity shows
// (or lets the viewer search / group by). Inputs bind through the form
// screen's entity, not c.Entity, so they are not listed.
func hiddenDisplayRefs(c *Component) []displayRef {
	var out []displayRef
	addRef := func(key, field string) {
		if field != "" {
			out = append(out, displayRef{key, field})
		}
	}
	if c.Item != nil {
		addRef("list item.title", c.Item.Title)
		addRef("list item.subtitle", c.Item.Subtitle)
		addRef("list item.trailing", c.Item.Trailing)
		addRef("list item.image", c.Item.Image)
	}
	for i, col := range c.Columns {
		addRef(fmt.Sprintf("columns[%d].field", i), col.Field)
	}
	addRef("titleField", c.TitleField)
	addRef("subtitleField", c.SubtitleField)
	addRef("searchField", c.SearchField)
	addRef("groupBy", c.GroupBy)
	addRef("nameField", c.NameField)
	addRef("balanceField", c.BalanceField)
	addRef("columnField", c.ColumnField)
	addRef("rowField", c.RowField)
	// stat / chart / progressBar / gauge: the aggregated value (count reads no field).
	if c.Aggregate != "count" {
		switch c.Type {
		case CompStat, CompChart, CompProgressBar, CompGauge:
			addRef("field", c.Field)
		}
	}
	return out
}

// checkHiddenExpr reports a hidden value that flows into a result other than
// a comparison. row is the computed's entity (nil for stat.value).
func checkHiddenExpr(where string, e *Expr, row *Entity, entities map[string]*Entity, inCompare bool, add func(string, ...any)) {
	if e == nil {
		return
	}
	switch e.Kind {
	case ExprRef:
		if inCompare || row == nil {
			return
		}
		if owner, f := resolveRefField(e.Ref, row, entities); f != nil && f.Hidden {
			add("%s: computed shows hidden field %s.%s; a hidden value may only be compared (eq / neq / gt / gte / lt / lte) — %s", where, owner, f.ID, hiddenHint)
		}
	case ExprAgg:
		if e.Agg == nil || e.Agg.Agg == "count" || e.Agg.Field == "" {
			return
		}
		if t := entities[e.Agg.Entity]; t != nil {
			if f := t.FieldByID(e.Agg.Field); f != nil && f.Hidden {
				add("%s: agg %s over hidden field %s.%s shows it; count the matching records instead (filter by the field), or %s", where, e.Agg.Agg, t.ID, f.ID, hiddenHint)
			}
		}
	case ExprOp:
		cmp := inCompare || compareOps[e.Op]
		for i := range e.Args {
			checkHiddenExpr(where, &e.Args[i], row, entities, cmp, add)
		}
	case ExprIf:
		checkHiddenExpr(where, e.If, row, entities, inCompare, add)
		checkHiddenExpr(where, e.Then, row, entities, inCompare, add)
		checkHiddenExpr(where, e.Else, row, entities, inCompare, add)
	}
}

// resolveRefField returns the field a ref reads: "<field>" on row, or
// "<refField>.<field>" on the referenced entity (lookup).
func resolveRefField(ref string, row *Entity, entities map[string]*Entity) (string, *Field) {
	head, tail, dotted := strings.Cut(ref, ".")
	if !dotted {
		return row.ID, row.FieldByID(ref)
	}
	rf := row.FieldByID(head)
	if rf == nil || rf.RefEntity == "" {
		return "", nil
	}
	t := entities[rf.RefEntity]
	if t == nil {
		return "", nil
	}
	return t.ID, t.FieldByID(tail)
}
