package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAIGenerate(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "generate", Shape: "list", Count: 5, Prompt: "罰ゲーム"},
		{Type: dsl.CompVStack, Children: []dsl.Component{
			{Type: dsl.CompAiButton, Task: "generate", Shape: "fields", Items: []string{"主菜"}, Entity: "food", Prompt: "献立"},
		}},
	}}}}
	for _, exp := range []map[string]any{
		{"aiGenerate": "list"}, {"aiGenerate": "fields"}, {"aiGenerate": "any"}, {"aiGenerateEntity": true}, {},
	} {
		if miss := checkAIGenerate(exp, app); miss != nil {
			t.Errorf("%v: %v", exp, miss)
		}
	}
	for _, exp := range []map[string]any{{"aiGenerate": "line"}, {"aiGenerateEntity": false}} {
		if miss := checkAIGenerate(exp, app); miss == nil {
			t.Errorf("%v should miss", exp)
		}
	}
	// a summarize button is not a generate button; empty shape is line
	plain := &dsl.App{Screens: []dsl.Screen{{ID: "f", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "summarize", Output: "s"},
		{Type: dsl.CompAiButton, Task: "generate", Prompt: "一言"},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiGenerate": "line"}, plain); miss != nil {
		t.Errorf("line: %v", miss)
	}
	if miss := checkAIGenerate(map[string]any{"aiGenerate": "list"}, plain); miss == nil {
		t.Error("summarize must not count")
	}
	if bad := validateExpect(map[string]any{"aiGenerate": "table"}); len(bad) != 1 {
		t.Errorf("bad shape: %v", bad)
	}
}
