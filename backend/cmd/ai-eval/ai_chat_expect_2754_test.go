package main

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func chatApp2754(chats ...dsl.Component) *dsl.App {
	return &dsl.App{Screens: []dsl.Screen{{ID: "home", Components: chats}}}
}

func TestCheckAIChatInput2754(t *testing.T) {
	reading := dsl.Component{Type: dsl.CompAiChat, Entity: "turn", InputEntity: "memo"}
	plain := dsl.Component{Type: dsl.CompAiChat, Entity: "turn"}
	nested := dsl.Component{Type: dsl.CompList, Children: []dsl.Component{reading}}
	tests := []struct {
		name string
		exp  map[string]any
		app  *dsl.App
		want string // substring of the miss, "" = pass
	}{
		{"expected and present", map[string]any{"aiChatInput": true}, chatApp2754(reading), ""},
		{"expected and present in children", map[string]any{"aiChatInput": true}, chatApp2754(nested), ""},
		{"missing input", map[string]any{"aiChatInput": true}, chatApp2754(plain), "aiChatInput=0/1 aiChat"},
		{"no aiChat at all", map[string]any{"aiChatInput": true}, chatApp2754(), "aiChatInput=0/0 aiChat"},
		{"unexpected input", map[string]any{"aiChatInput": false}, chatApp2754(plain, reading), "aiChatInput=1/2 aiChat"},
		{"expected none and none", map[string]any{"aiChatInput": false}, chatApp2754(plain), ""},
		{"wrong type is ignored", map[string]any{"aiChatInput": "yes"}, chatApp2754(plain), ""},
		{"no key", map[string]any{}, chatApp2754(plain), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			miss := checkAIChatInput(tc.exp, tc.app)
			if tc.want == "" {
				if miss != nil {
					t.Fatalf("want pass, got %v", miss)
				}
				return
			}
			if len(miss) != 1 || !strings.Contains(miss[0], tc.want) {
				t.Fatalf("want miss containing %q, got %v", tc.want, miss)
			}
		})
	}
}

func TestCheckAIChatSaveTo2754(t *testing.T) {
	saving := dsl.Component{Type: dsl.CompAiChat, Entity: "turn", SaveTo: &dsl.AISaveTo{Entity: "memo", Fields: []string{"text"}}}
	noFields := dsl.Component{Type: dsl.CompAiChat, Entity: "turn", SaveTo: &dsl.AISaveTo{Entity: "memo"}}
	plain := dsl.Component{Type: dsl.CompAiChat, Entity: "turn"}
	nested := dsl.Component{Type: dsl.CompList, Children: []dsl.Component{saving}}
	tests := []struct {
		name string
		exp  map[string]any
		app  *dsl.App
		want string
	}{
		{"expected and present", map[string]any{"aiChatSaveTo": true}, chatApp2754(saving), ""},
		{"expected and present in children", map[string]any{"aiChatSaveTo": true}, chatApp2754(nested), ""},
		{"missing saveTo", map[string]any{"aiChatSaveTo": true}, chatApp2754(plain), "aiChatSaveTo=0/1 aiChat"},
		{"saveTo entity without fields", map[string]any{"aiChatSaveTo": true}, chatApp2754(noFields), "aiChatSaveTo=0/1 aiChat"},
		{"unexpected saveTo", map[string]any{"aiChatSaveTo": false}, chatApp2754(saving, plain), "aiChatSaveTo=1/2 aiChat"},
		{"expected none and none", map[string]any{"aiChatSaveTo": false}, chatApp2754(plain), ""},
		{"wrong type is ignored", map[string]any{"aiChatSaveTo": 1}, chatApp2754(plain), ""},
		{"no key", map[string]any{}, chatApp2754(saving), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			miss := checkAIChatSaveTo(tc.exp, tc.app)
			if tc.want == "" {
				if miss != nil {
					t.Fatalf("want pass, got %v", miss)
				}
				return
			}
			if len(miss) != 1 || !strings.Contains(miss[0], tc.want) {
				t.Fatalf("want miss containing %q, got %v", tc.want, miss)
			}
		})
	}
}

func TestAIChatExpectKeysKnown2754(t *testing.T) {
	for _, k := range []string{"aiChatInput", "aiChatSaveTo"} {
		if bad := validateExpect(map[string]any{k: true}); len(bad) != 0 {
			t.Errorf("%s should be a known key: %v", k, bad)
		}
	}
}
