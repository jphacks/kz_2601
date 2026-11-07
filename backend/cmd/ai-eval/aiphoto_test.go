package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// aiInputPhoto: some aiButton (also inside a stack) sends a photo field.
func TestCheckAIInputPhoto(t *testing.T) {
	with := &dsl.App{Screens: []dsl.Screen{{ID: "f", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "summarize", Output: "s"},
		{Type: dsl.CompVStack, Children: []dsl.Component{
			{Type: dsl.CompAiButton, Task: "extract", InputPhoto: "rx", Output: "note"},
		}},
	}}}}
	without := &dsl.App{Screens: []dsl.Screen{{ID: "f", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "summarize", Output: "s"},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiInputPhoto": true}, with); miss != nil {
		t.Errorf("with: %v", miss)
	}
	if miss := checkAIGenerate(map[string]any{"aiInputPhoto": false}, without); miss != nil {
		t.Errorf("without: %v", miss)
	}
	if miss := checkAIGenerate(map[string]any{"aiInputPhoto": true}, without); miss == nil {
		t.Error("without should miss")
	}
	if bad := validateExpect(map[string]any{"aiInputPhoto": "yes"}); len(bad) != 1 {
		t.Errorf("bad kind: %v", bad)
	}
}
