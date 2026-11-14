package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckPlaceSearch(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{
		{ID: "form", Entity: "spot", Components: []dsl.Component{
			{Type: dsl.CompPlaceSearch, Category: "cafe", NameField: "name", LocationField: "place"},
			{Type: dsl.CompSection, Children: []dsl.Component{{Type: dsl.CompLocationButton, Field: "place", AddressField: "address"}}},
		}},
	}}
	for _, exp := range []map[string]any{
		{"placeSearch": "category"},
		{"placeSearch": "any", "locationAddress": true},
		{},
	} {
		if miss := checkPlaceSearch(exp, app); miss != nil {
			t.Errorf("%v: %v", exp, miss)
		}
	}
	for _, exp := range []map[string]any{
		{"placeSearch": "query"},
		{"placeSearch": "nearby"},
		{"locationAddress": false},
	} {
		if miss := checkPlaceSearch(exp, app); miss == nil {
			t.Errorf("%v should miss", exp)
		}
	}
	// a placeSearch that fills only the address does not count
	addrOnly := &dsl.App{Screens: []dsl.Screen{{ID: "f", Entity: "spot", Components: []dsl.Component{
		{Type: dsl.CompPlaceSearch, AddressField: "address"},
	}}}}
	if miss := checkPlaceSearch(map[string]any{"placeSearch": "any"}, addrOnly); miss == nil {
		t.Error("address-only placeSearch should miss")
	}
	if bad := validateExpect(map[string]any{"placeSearch": "free", "locationAddress": true}); len(bad) != 0 {
		t.Errorf("known keys: %v", bad)
	}
}
