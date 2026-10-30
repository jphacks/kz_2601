package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckExport(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{
		ID:      "home",
		Toolbar: []dsl.ToolbarButton{{Label: "PDF", Action: dsl.Action{Type: dsl.ActionExport, Entity: "e", Format: "pdf"}}},
		Components: []dsl.Component{
			{Type: dsl.CompHStack, Children: []dsl.Component{
				{Type: dsl.CompButton, Action: &dsl.Action{Type: dsl.ActionExport, Entity: "e", Format: "csv"}},
			}},
		},
	}}}
	for _, want := range []string{"csv", "pdf", "both"} {
		if miss := checkExport(map[string]any{"export": want}, app); miss != nil {
			t.Errorf("%s: %v", want, miss)
		}
	}
	if miss := checkExport(map[string]any{}, app); miss != nil {
		t.Errorf("no expect key: %v", miss)
	}
	plain := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompShareButton, Text: "x"},
	}}}}
	if miss := checkExport(map[string]any{"export": "csv"}, plain); miss == nil {
		t.Error("a shareButton must not count as export")
	}
}
