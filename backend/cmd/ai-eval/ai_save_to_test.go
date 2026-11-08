package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAISaveTo(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompVStack, Children: []dsl.Component{
			{Type: dsl.CompAiButton, Task: "generate", Prompt: "問題", Shape: "list", SaveTo: &dsl.AISaveTo{Entity: "card", Field: "q"}},
		}},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiSaveTo": true}, app); miss != nil {
		t.Error(miss)
	}
	if miss := checkAIGenerate(map[string]any{"aiSaveTo": false}, app); miss == nil {
		t.Error("false should miss")
	}
	plain := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "generate", Prompt: "案", Shape: "list"},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiSaveTo": true}, plain); miss == nil {
		t.Error("true should miss without saveTo")
	}
	if bad := validateExpect(map[string]any{"aiSaveTo": "yes"}); len(bad) != 1 {
		t.Errorf("bad type: %v", bad)
	}
}
