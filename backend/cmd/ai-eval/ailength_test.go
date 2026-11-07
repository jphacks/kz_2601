package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// aiLength: some aiButton (also inside a stack) has that length; unset = short.
func TestCheckAILength(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "suggest", Output: "s"},
		{Type: dsl.CompVStack, Children: []dsl.Component{
			{Type: dsl.CompAiButton, Task: "generate", Prompt: "手紙", Length: "long"},
		}},
	}}}}
	for _, want := range []string{"long", "short"} {
		if miss := checkAIGenerate(map[string]any{"aiLength": want}, app); miss != nil {
			t.Errorf("%s: %v", want, miss)
		}
	}
	short := &dsl.App{Screens: []dsl.Screen{{ID: "f", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "summarize", Output: "s", Length: "short"},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiLength": "long"}, short); miss == nil {
		t.Error("long should miss")
	}
	if bad := validateExpect(map[string]any{"aiLength": "medium"}); len(bad) != 1 {
		t.Errorf("bad length: %v", bad)
	}
}
