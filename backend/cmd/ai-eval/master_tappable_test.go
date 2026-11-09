package main

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckMasterTappable(t *testing.T) {
	form := dsl.Screen{ID: "member_form", Entity: "members"}
	app := func(home ...dsl.Component) *dsl.App {
		return &dsl.App{InitialScreen: "home", Screens: []dsl.Screen{{ID: "home", Components: home}, form}}
	}
	tap := &dsl.Action{Type: dsl.ActionNavigate, Screen: "member_form"}
	list := dsl.Component{Type: dsl.CompList, Entity: "members", OnItemTap: tap}
	table := dsl.Component{Type: dsl.CompTable, Entity: "members"}
	exp := map[string]any{"masterTappable": []any{"members"}}

	if miss := checkMasterTappable(exp, app(table, list)); miss != nil {
		t.Errorf("table + list with onItemTap: %v", miss)
	}
	if miss := checkMasterTappable(map[string]any{"masterTappable": []any{"people|members"}}, app(list)); miss != nil {
		t.Errorf("alternative ids: %v", miss)
	}
	if miss := checkMasterTappable(exp, app(dsl.Component{Type: dsl.CompSection, Children: []dsl.Component{list}})); miss != nil {
		t.Errorf("list in a section: %v", miss)
	}
	if miss := checkMasterTappable(exp, app(table)); len(miss) != 1 {
		t.Errorf("table only should miss: %v", miss)
	}
	if miss := checkMasterTappable(exp, app(dsl.Component{Type: dsl.CompList, Entity: "members"})); len(miss) != 1 {
		t.Errorf("list without onItemTap should miss: %v", miss)
	}
	if miss := checkMasterTappable(map[string]any{}, app(table)); miss != nil {
		t.Errorf("no expect key: %v", miss)
	}
}

// The checker must agree with dsl lint savedRecordReach (#766) on what opens a
// saved record.
func TestCheckMasterTappable_MatchesLint(t *testing.T) {
	form := dsl.Screen{ID: "member_form", Entity: "members"}
	tap := &dsl.Action{Type: dsl.ActionNavigate, Screen: "member_form", Presentation: "push"}
	exp := map[string]any{"masterTappable": []any{"members"}}
	home := func(cs ...dsl.Component) *dsl.App {
		return &dsl.App{InitialScreen: "home", Entities: []dsl.Entity{{ID: "members"}}, Screens: []dsl.Screen{{ID: "home", Components: cs}, form}}
	}

	for _, typ := range []dsl.ComponentType{dsl.CompRandomPicker, dsl.CompMatchList} {
		if miss := checkMasterTappable(exp, home(dsl.Component{Type: typ, Entity: "members", OnItemTap: tap})); miss != nil {
			t.Errorf("%s with onItemTap opens the form (lint accepts it): %v", typ, miss)
		}
	}
	prefill := &dsl.Action{Type: dsl.ActionNavigate, Screen: "member_form", PrefillRef: "member"}
	if miss := checkMasterTappable(exp, home(dsl.Component{Type: dsl.CompList, Entity: "members", OnItemTap: prefill})); len(miss) != 1 {
		t.Errorf("a prefillRef tap opens a new record, not the saved one: %v", miss)
	}
	list := dsl.Component{Type: dsl.CompList, Entity: "members", OnItemTap: tap}
	// list on the second tab, reached only through app.tabs
	tabs := home()
	tabs.App.Tabs = []dsl.Tab{{Screen: "home"}, {Screen: "people"}}
	tabs.Screens = append(tabs.Screens, dsl.Screen{ID: "people", Components: []dsl.Component{list}})
	if miss := checkMasterTappable(exp, tabs); miss != nil {
		t.Errorf("list on a tab: %v", miss)
	}
	// the same list on a screen nothing navigates to
	orphan := home()
	orphan.Screens = append(orphan.Screens, dsl.Screen{ID: "people", Components: []dsl.Component{list}})
	if miss := checkMasterTappable(exp, orphan); len(miss) != 1 {
		t.Errorf("unreachable list should miss: %v", miss)
	}
	if miss := checkMasterTappable(map[string]any{"masterTappable": []any{"people | members"}}, home(list)); miss != nil {
		t.Errorf("spaced aliases: %v", miss)
	}
	// none of the ids exists: say so, so a naming miss is not read as a reach miss
	miss := checkMasterTappable(map[string]any{"masterTappable": []any{"people|person"}}, home(list))
	if len(miss) != 1 || !strings.Contains(miss[0], "no entity") {
		t.Errorf("absent entity should be reported as such: %v", miss)
	}
}
