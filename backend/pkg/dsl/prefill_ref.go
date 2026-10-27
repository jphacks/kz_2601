package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// Record taps and parent -> child records (#256).
//
// list / mapView / gridTable onItemTap and calendar.onDateTap hand the tapped
// record's id to the navigate target, which opens its form in edit mode. When
// that form edits another entity the id matches nothing (quiz: tapping a
// question opened the answer form "editing" the question id), so Validate
// rejects it.
//
// navigate.prefillRef opens the target form as a new record with one of its
// ref fields set to the current record: the tapped row of a list / mapView,
// or the record shown on the form screen holding the button / toolbar. That
// is how a comment, answer or RSVP is created from its parent.

// recordSource is where a navigate gets its "current record" from.
type recordSource struct {
	entity     string // entity of the current record ("" = none)
	tap        bool   // an item tap that passes the tapped record's id
	prefillsOK bool   // prefillRef is supported here
}

func checkRecordNav(a *App, entities map[string]*Entity, add func(string, ...any)) {
	screens := map[string]*Screen{}
	for i := range a.Screens {
		screens[a.Screens[i].ID] = &a.Screens[i]
	}
	for _, s := range a.Screens {
		onForm := recordSource{entity: s.Entity, prefillsOK: true}
		for i := range s.Toolbar {
			checkRecordAction(fmt.Sprintf("screen %q toolbar[%d]", s.ID, i), &s.Toolbar[i].Action, onForm, screens, entities, add)
		}
		eachScreenComponent(s.Components, func(where string, c *Component) {
			where = fmt.Sprintf("screen %q %s", s.ID, where)
			checkRecordAction(where, c.Action, onForm, screens, entities, add)
			switch c.Type {
			case CompList, CompMapView:
				checkRecordAction(where+".onItemTap", c.OnItemTap, recordSource{entity: c.Entity, tap: true, prefillsOK: true}, screens, entities, add)
			case CompGridTable:
				checkRecordAction(where+".onItemTap", c.OnItemTap, recordSource{entity: c.Entity, tap: true}, screens, entities, add)
			case CompCalendar:
				checkRecordAction(where+".onDateTap", c.OnDateTap, recordSource{entity: c.Entity, tap: true}, screens, entities, add)
			}
		})
	}
}

func checkRecordAction(where string, act *Action, src recordSource, screens map[string]*Screen, entities map[string]*Entity, add func(string, ...any)) {
	if act == nil {
		return
	}
	if act.PrefillRef != "" && act.Type != ActionNavigate {
		add("%s: prefillRef is only allowed on navigate", where)
		return
	}
	if act.Type != ActionNavigate {
		return
	}
	target := screens[act.Screen]
	if target == nil {
		return // reported by Validate
	}
	child := entities[target.Entity]
	if act.PrefillRef == "" {
		if src.tap && child != nil && src.entity != "" && target.Entity != src.entity && entities[src.entity] != nil {
			add("%s: tapping a %q record opens screen %q, the form of entity %q, in edit mode with the %q record's id, which no %q record has. "+
				"Point it at the form of %q%s, or to add a new %q for the tapped %q set prefillRef to %s",
				where, src.entity, target.ID, child.ID, src.entity, child.ID,
				src.entity, formHint(screens, src.entity), child.ID, src.entity, refFieldHint(child, src.entity))
		}
		return
	}
	if !src.prefillsOK {
		add("%s: prefillRef is not supported here (calendar / gridTable taps already fill the date / cell); "+
			"put a button on the form screen of %q that navigates with prefillRef instead", where, src.entity)
		return
	}
	if child == nil {
		add("%s: prefillRef needs a form screen (screen.entity) as the navigate target; screen %q has no entity", where, target.ID)
		return
	}
	f := child.FieldByID(act.PrefillRef)
	switch {
	case f == nil:
		add("%s: prefillRef %q is not a field of entity %q; use %s", where, act.PrefillRef, child.ID, refFieldHint(child, src.entity))
		return
	case f.Type != FieldRef || f.Computed != nil:
		add("%s: prefillRef %q must be a stored ref field of entity %q (it is %s)", where, act.PrefillRef, child.ID, f.Type)
		return
	}
	if src.entity == "" {
		add("%s: prefillRef needs a current record to put in %q: move this button to the form screen of %q (screen.entity %q), "+
			"or use it on a %q list's onItemTap", where, f.ID, f.RefEntity, f.RefEntity, f.RefEntity)
		return
	}
	if f.RefEntity != src.entity {
		add("%s: prefillRef %q points to %q, but the current record here is a %q; use %s", where, f.ID, f.RefEntity, src.entity, refFieldHint(child, src.entity))
		return
	}
	if !hasPicker(target, f.ID) {
		add("%s: screen %q must keep a picker for %q so the parent stays visible and can be changed (prefillRef only sets its first value)", where, target.ID, f.ID)
	}
}

// refFieldHint names the ref fields of child that point to parent ("" = any).
func refFieldHint(child *Entity, parent string) string {
	var ids []string
	for _, f := range child.Fields {
		if f.Type == FieldRef && (parent == "" || f.RefEntity == parent) && f.Computed == nil {
			ids = append(ids, fmt.Sprintf("%q", f.ID))
		}
	}
	if len(ids) == 0 && parent == "" {
		return fmt.Sprintf("a ref field that you add to entity %q", child.ID)
	}
	if len(ids) == 0 {
		return fmt.Sprintf("a ref field (refEntity %q) that you add to entity %q", parent, child.ID)
	}
	return "one of " + strings.Join(ids, ", ")
}

// formHint lists the form screens of entity as " (screen ids: a, b)".
func formHint(screens map[string]*Screen, entity string) string {
	var ids []string
	for id, s := range screens {
		if s.Entity == entity {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	slices.Sort(ids)
	return " (screen ids: " + strings.Join(ids, ", ") + ")"
}

func hasPicker(s *Screen, field string) bool {
	found := false
	eachScreenComponent(s.Components, func(_ string, c *Component) {
		if c.Type == CompPicker && c.Field == field {
			found = true
		}
	})
	return found
}
