package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAIChatSpeak(t *testing.T) {
	speaking := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompAiChat, Entity: "turn", TextField: "text", Persona: "英会話", Speak: true},
	}}}}
	silent := &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: []dsl.Component{
		{Type: dsl.CompAiChat, Entity: "turn", TextField: "text", Persona: "英会話"},
	}}}}
	if miss := checkAIChatSpeak(map[string]any{"aiChatSpeak": true}, speaking); miss != nil {
		t.Errorf("speaking: %v", miss)
	}
	if miss := checkAIChatSpeak(map[string]any{"aiChatSpeak": true}, silent); miss == nil {
		t.Error("silent aiChat should miss")
	}
	if miss := checkAIChatSpeak(map[string]any{"aiChatSpeak": false}, silent); miss != nil {
		t.Errorf("silent want false: %v", miss)
	}
	if miss := checkAIChatSpeak(map[string]any{}, silent); miss != nil {
		t.Errorf("no expect key: %v", miss)
	}
	if bad := validateExpect(map[string]any{"aiChatSpeak": true}); len(bad) != 0 {
		t.Errorf("aiChatSpeak is a known key: %v", bad)
	}
}
