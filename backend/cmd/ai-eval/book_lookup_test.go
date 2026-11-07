package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckBookLookup(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{ID: "form", Components: []dsl.Component{
		{Type: dsl.CompButton, Action: &dsl.Action{Type: dsl.ActionLookupBook, BookLookup: &dsl.BookLookup{ISBNField: "isbn", TitleField: "title"}}},
	}}}}
	if miss := checkBookLookup(map[string]any{"lookupBook": true}, app); miss != nil {
		t.Errorf("want pass, got %v", miss)
	}
	if miss := checkBookLookup(map[string]any{}, app); miss != nil {
		t.Errorf("no keys: %v", miss)
	}
	if miss := checkBookLookup(map[string]any{"lookupBook": false}, app); len(miss) != 1 {
		t.Errorf("unwanted lookup: want 1 miss, got %v", miss)
	}
	noTitle := &dsl.App{Screens: []dsl.Screen{{ID: "form", Toolbar: []dsl.ToolbarButton{{Label: "x", Action: dsl.Action{Type: dsl.ActionLookupBook, BookLookup: &dsl.BookLookup{ISBNField: "isbn", AuthorsField: "a"}}}}}}}
	if miss := checkBookLookup(map[string]any{"lookupBook": true}, noTitle); len(miss) != 1 {
		t.Errorf("no title mapped: want 1 miss, got %v", miss)
	}
}
