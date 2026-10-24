package ai

import (
	"encoding/json"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// addMissingAddButtons is a deterministic fix for a dead end seen in the demo
// pass (#164: 割り勘 dropped its "支出を追加" button, so expenses could only be
// edited from the list, never added). For every entity whose form no button
// or toolbar on a reachable screen opens (a list's onItemTap only
// edits), it appends a toolbar button that presents the form as a sheet to
// the screen listing the entity (else initialScreen). Toolbar buttons take no
// grid cell, so the layout is untouched. Returns raw unchanged when nothing is
// missing or the result does not validate.
func addMissingAddButtons(raw []byte, app *dsl.App, loc Locale) []byte {
	reach := app.Reachable()
	opened := map[string]bool{} // form screen ids opened by a button / toolbar
	for _, s := range app.Screens {
		if !reach[s.ID] {
			continue
		}
		for _, tb := range s.Toolbar {
			if tb.Action.Type == dsl.ActionNavigate {
				opened[tb.Action.Screen] = true
			}
		}
		eachComponent(s.Components, func(c *dsl.Component) {
			if c.Action != nil && c.Action.Type == dsl.ActionNavigate {
				opened[c.Action.Screen] = true
			}
		})
	}
	type need struct{ entity, form, label string }
	var needs []need
	for _, e := range app.Entities {
		form := formOf(app, e.ID)
		if form == "" || opened[form] {
			continue
		}
		label := e.Name + "を追加"
		if loc == LocaleEN {
			label = "Add " + e.Name
		}
		needs = append(needs, need{e.ID, form, label})
	}
	if len(needs) == 0 {
		return raw
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	screens, _ := doc["screens"].([]any)
	for _, n := range needs {
		host := listingScreen(screens, reach, n.form, app.InitialScreen)
		if host == nil {
			continue
		}
		tb, _ := host["toolbar"].([]any)
		host["toolbar"] = append(tb, map[string]any{
			"label":  n.label,
			"icon":   "plus",
			"action": map[string]any{"type": "navigate", "screen": n.form, "presentation": "sheet"},
		})
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

// listingScreen picks the reachable non-form screen that already links to
// form (its list's onItemTap), falling back to initialScreen.
func listingScreen(screens []any, reach map[string]bool, form, initial string) map[string]any {
	var fallback map[string]any
	for _, s := range screens {
		sm, _ := s.(map[string]any)
		id, _ := sm["id"].(string)
		if e, _ := sm["entity"].(string); e != "" || !reach[id] {
			continue
		}
		if navigatesTo(sm, form) {
			return sm
		}
		if id == initial {
			fallback = sm
		}
	}
	return fallback
}

func eachComponent(comps []dsl.Component, fn func(c *dsl.Component)) {
	for i := range comps {
		fn(&comps[i])
		eachComponent(comps[i].Children, fn)
	}
}
