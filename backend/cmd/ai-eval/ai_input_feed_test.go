package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAIInputFeeds(t *testing.T) {
	app := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompFeed, ID: "hpa", Provider: dsl.FeedPressure, Region: "170000"},
		{Type: dsl.CompVStack, Children: []dsl.Component{
			{Type: dsl.CompAiButton, Task: "generate", Prompt: "体調", InputFeeds: []string{"hpa"}},
		}},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiInputFeeds": true}, app); miss != nil {
		t.Error(miss)
	}
	if miss := checkAIGenerate(map[string]any{"aiInputFeeds": false}, app); miss == nil {
		t.Error("false should miss")
	}
	plain := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompAiButton, Task: "generate", Prompt: "案"},
	}}}}
	if miss := checkAIGenerate(map[string]any{"aiInputFeeds": true}, plain); miss == nil {
		t.Error("true should miss without inputFeeds")
	}
	if bad := validateExpect(map[string]any{"aiInputFeeds": "yes"}); len(bad) != 1 {
		t.Errorf("bad type: %v", bad)
	}
}
