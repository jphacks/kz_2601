package main

import (
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
