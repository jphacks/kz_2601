package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckTextHighlight(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{
		{ID: "form", Entity: "error", Components: []dsl.Component{{Type: dsl.CompSection, Children: []dsl.Component{
			{Type: dsl.CompText, Text: "{{message}}", Highlight: &dsl.TextHighlight{Field: "keywords"}},
		}}}},
	}}
	for _, want := range []string{"field", "any"} {
		if miss := checkTextHighlight(map[string]any{"textHighlight": want}, app); miss != nil {
			t.Errorf("textHighlight=%s: %v", want, miss)
		}
	}
	if miss := checkTextHighlight(map[string]any{"textHighlight": "words"}, app); miss == nil {
		t.Error("no text uses fixed words")
	}
	if miss := checkTextHighlight(map[string]any{}, app); miss != nil {
		t.Errorf("no expect key: %v", miss)
	}
	if miss := checkTextHighlight(map[string]any{"textHighlight": "any"}, &dsl.App{}); miss == nil {
		t.Error("no highlight at all should miss")
	}
	if miss := checkTextHighlight(map[string]any{"textHighlight": "color"}, app); miss == nil {
		t.Error("an unknown value should miss")
	}
	if bad := validateExpect(map[string]any{"textHighlight": "field"}); len(bad) != 0 {
		t.Errorf("textHighlight is a known key: %v", bad)
	}
}
