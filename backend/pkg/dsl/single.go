package dsl

import (
	"fmt"
	"sort"
	"strings"
)

// Single entities (#267): Entity.single holds the app's one-of settings (a
// cycle length, a goal, a budget). It always has one record: the form screen
// edits it (no add, no delete), lists never show it, and expressions read a
// field with {"setting": "<fieldId>"}. Before the first save the record is
// empty and a setting reads the field's defaultValue.
//
// Storage: iOS creates the record with the fixed clientId SingleClientID, so
// appconfig stores it under one document id per user (records.go upserts).

// SingleClientID is the clientId (and the source of the fixed record id) of a
// single entity's record.
const SingleClientID = "single"

// SingleEntities lists the ids of single entities, in order.
func (a *App) SingleEntities() []string {
	var out []string
	for i := range a.Entities {
		if a.Entities[i].Single {
			out = append(out, a.Entities[i].ID)
		}
	}
	return out
}

// SettingField resolves {"setting": id}: the single entity that has the field
// and the field. n is how many single entities have it (only 1 is valid).
func SettingField(entities map[string]*Entity, id string) (e *Entity, f *Field, n int) {
	ids := make([]string, 0, len(entities))
	for k := range entities {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	for _, k := range ids {
		ent := entities[k]
		if !ent.Single {
			continue
		}
		if fld := ent.FieldByID(id); fld != nil {
			if n == 0 {
				e, f = ent, fld
			}
			n++
		}
	}
	return e, f, n
}

func (c *exprChecker) checkSetting(where, id string) exprType {
	e, f, n := SettingField(c.entities, id)
	switch {
	case n == 0:
		var fields []string
		for _, ent := range c.entities {
			if ent.Single {
				for _, fl := range ent.Fields {
					fields = append(fields, fl.ID)
				}
			}
		}
		sort.Strings(fields)
		if len(fields) == 0 {
			c.add(`%s: {"setting":%q} needs an entity with "single": true that has field %q (no single entity in this app)`, where, id, id)
		} else {
			c.add(`%s: {"setting":%q} is not a field of a single entity (settings: %s)`, where, id, strings.Join(fields, ", "))
		}
		return tAny
	case n > 1:
		c.add(`%s: {"setting":%q} is ambiguous: more than one single entity has field %q; give the fields different ids`, where, id, id)
	}
	c.dep(e, f.ID)
	return fieldExprType(f.Type)
}

// checkSingleEntities rejects the uses that would show or add more than one
// record of a single entity.
func checkSingleEntities(a *App, entities map[string]*Entity, add func(string, ...any)) {
	single := map[string]bool{}
	for i := range a.Entities {
		e := &a.Entities[i]
		if !e.Single {
			continue
		}
		single[e.ID] = true
		if e.IsShared() {
			add("entity %q: single (one record of settings) is only for a private entity; remove scope %q", e.ID, e.Scope)
		}
		if e.RetentionDays > 0 {
			add("entity %q: single keeps its one record; remove retentionDays", e.ID)
		}
		if e.Match != "" {
			add("entity %q: single cannot be a matching entity; remove match", e.ID)
		}
		for _, f := range e.Fields {
			if f.Hidden {
				add("entity %q field %q: a single entity's fields cannot be hidden", e.ID, f.ID)
			}
		}
	}
	if len(single) == 0 {
		return
	}
	hint := `a single entity has one record: edit it on its form screen (screen.entity) and read values with {"setting":"<fieldId>"}`
	for i := range a.Entities {
		for _, f := range a.Entities[i].Fields {
			if (f.Type == FieldRef || f.Type == FieldRefs) && single[f.RefEntity] {
				add("entity %q field %q: refEntity %q is single; %s", a.Entities[i].ID, f.ID, f.RefEntity, hint)
			}
		}
	}
	checkAct := func(where string, act *Action) {
		if act == nil || !single[act.Entity] {
			return
		}
		switch act.Type {
		case ActionDeleteRecord:
			add("%s: deleteRecord on single entity %q; its one record cannot be deleted (remove the button)", where, act.Entity)
		case ActionExport:
			add("%s: export of single entity %q; %s", where, act.Entity, hint)
		}
	}
	for _, s := range a.Screens {
		for i, tb := range s.Toolbar {
			checkAct(fmt.Sprintf("screen %q toolbar[%d]", s.ID, i), &tb.Action)
		}
		eachScreenComponent(s.Components, func(where string, c *Component) {
			where = fmt.Sprintf("screen %q %s", s.ID, where)
			if single[c.Entity] {
				add("%s: %s over single entity %q; %s (e.g. stat.value / progressBar.value / computed)", where, c.Type, c.Entity, hint)
			}
			checkAct(where, c.Action)
			checkAct(where+".onItemTap", c.OnItemTap)
			checkAct(where+".onDateTap", c.OnDateTap)
		})
	}
}
