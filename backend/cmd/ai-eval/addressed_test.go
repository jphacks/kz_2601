package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestAddressedMetrics(t *testing.T) {
	app := parseExample(t, "warikan")
	all := map[string]any{"addressed": true, "addressedGroup": true, "addressedUnread": true}
	if miss := checkAddressed(all, app); len(miss) != 3 {
		t.Fatalf("no addressed entity must miss all: %v", miss)
	}
	app.Entities = append(app.Entities, dsl.Entity{ID: "dm", Name: "DM", Scope: dsl.ScopeShared, AddressedTo: "to",
		Marks:  []dsl.Mark{{ID: "read", Icon: "eye", Label: "既読"}},
		Fields: []dsl.Field{{ID: "to", Name: "宛先", Type: dsl.FieldMembers}, {ID: "body", Name: "本文", Type: dsl.FieldText}}})
	if miss := checkAddressed(map[string]any{"addressed": true, "addressedGroup": true, "addressedUnread": false}, app); len(miss) != 0 {
		t.Fatal(miss)
	}
	app.Screens[0].Components = append(app.Screens[0].Components, dsl.Component{Type: dsl.CompList, Entity: "dm", Item: &dsl.ListItem{Title: "body", Mark: "read"}})
	if miss := checkAddressed(all, app); len(miss) != 0 {
		t.Fatal(miss)
	}
}
