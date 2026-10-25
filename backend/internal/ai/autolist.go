package ai

import (
	"encoding/json"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// addMissingLists is a deterministic fix for the most common dead end left
// after the lint retry was turned off (#130 a04: study hours were recorded but
// no screen listed them). For every entity whose records no reachable screen
// lists (dsl.UnlistedEntities: settings such as a budget are excluded), it
// appends an area-less section with a list to the screen that opens the
// entity's form (else initialScreen). Area-less components stack below the
// laid-out ones, so the existing layout is untouched. Returns raw unchanged
// when nothing is missing or the result does not validate.
func addMissingLists(raw []byte, app *dsl.App) []byte {
	missing := app.UnlistedEntities()
	if len(missing) == 0 {
		return raw
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	screens, _ := doc["screens"].([]any)
	reach := app.Reachable()
	changed := false
	for _, id := range missing {
		e := app.EntityByID(id)
		form := formOf(app, id)
		if e == nil || form == "" {
			continue // no form: nothing to open from the list; leave it to the lint warning
		}
		host := hostScreen(screens, reach, form, app.InitialScreen)
		if host == nil {
			continue
		}
		comps, _ := host["components"].([]any)
		host["components"] = append(comps, autoListSection(e, form))
		changed = true
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return raw
	}
	if _, err := dsl.Parse(out); err != nil {
		return raw
	}
	return out
}

func formOf(app *dsl.App, entity string) string {
	for _, s := range app.Screens {
		if s.Entity == entity {
			return s.ID
		}
	}
	return ""
}

// hostScreen picks the reachable non-form screen that navigates to form,
// falling back to initialScreen.
func hostScreen(screens []any, reach map[string]bool, form, initial string) map[string]any {
	var fallback map[string]any
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		id, _ := sm["id"].(string)
		if e, _ := sm["entity"].(string); e != "" || !reach[id] {
			continue
		}
		if id == initial {
			fallback = sm
		}
		if navigatesTo(sm, form) {
			return sm
		}
	}
	return fallback
}

func navigatesTo(screen map[string]any, target string) bool {
	found := false
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["type"] == "navigate" && x["screen"] == target {
				found = true
			}
			for _, y := range x {
				walk(y)
			}
		case []any:
			for _, y := range x {
				walk(y)
			}
		}
	}
	walk(screen["toolbar"])
	walk(screen["components"])
	return found
}

// autoListSection builds {section{title: entity name, list}} with a title /
// subtitle (date) / trailing (number) picked from the fields, newest first.
func autoListSection(e *dsl.Entity, form string) map[string]any {
	var title, subtitle, trailing string
	for _, f := range e.Fields {
		switch f.Type {
		case dsl.FieldString, dsl.FieldText, dsl.FieldEnum, dsl.FieldRef:
			if title == "" && f.Computed == nil {
				title = f.ID
			}
		case dsl.FieldDate, dsl.FieldDatetime:
			if subtitle == "" {
				subtitle = f.ID
			}
		case dsl.FieldInteger, dsl.FieldNumber, dsl.FieldCurrency:
			if trailing == "" {
				trailing = f.ID
			}
		}
	}
	if title == "" && len(e.Fields) > 0 {
		title = e.Fields[0].ID
	}
	item := map[string]any{"title": title}
	if subtitle != "" && subtitle != title {
		item["subtitle"] = subtitle
	}
	if trailing != "" && trailing != title {
		item["trailing"] = trailing
	}
	list := map[string]any{
		"type":      "list",
		"entity":    e.ID,
		"item":      item,
		"onItemTap": map[string]any{"type": "navigate", "screen": form, "presentation": "push"},
	}
	if subtitle != "" {
		list["sort"] = map[string]any{"field": subtitle, "direction": "desc"}
	}
	return map[string]any{"type": "section", "title": e.Name, "children": []any{list}}
}
