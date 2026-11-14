package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAIOutputs(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{ID: "form", Components: []dsl.Component{
		{Type: dsl.CompVStack, Children: []dsl.Component{
			{Type: dsl.CompAiButton, Task: "generate", Prompt: "抜き出す", Shape: "fields", Items: []string{"題名"}, Outputs: map[string]string{"題名": "title"}},
		}},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiOutputs": true}, app); miss != nil {
		t.Error(miss)
	}
	if miss := checkAIGenerate(map[string]any{"aiOutputs": false}, app); miss == nil {
		t.Error("false should miss")
	}
	plain := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "generate", Prompt: "献立", Shape: "fields", Items: []string{"主菜"}},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiOutputs": true}, plain); miss == nil {
		t.Error("true should miss without outputs")
	}
	if bad := validateExpect(map[string]any{"aiOutputs": "yes"}); len(bad) != 1 {
		t.Errorf("bad type: %v", bad)
	}
}
