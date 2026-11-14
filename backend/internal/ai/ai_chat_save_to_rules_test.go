package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestAIChatSaveToRulesReachPrompt(t *testing.T) {
	for _, want := range []string{"## 話しかけて記録する(aiChat saveTo)", "」は aiChat に saveTo{entity, fields}", "勝手には保存しない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if strings.Index(systemPrompt, aiChatRules) > strings.Index(systemPrompt, aiChatSaveToRules) {
		t.Fatal("saveTo rules must follow the aiChat rules")
	}
}

// The shape the rules teach (turn + meal log, aiChat saveTo, list on another
// tab) parses and validates.
func TestAIChatSaveToRulesShapeValidates(t *testing.T) {
	raw := `{
  "schemaVersion": 1,
  "app": {"name": "食事メモ", "icon": "fork.knife", "voiceEntry": "none"},
  "initialScreen": "talk",
  "entities": [
    {"id": "turn", "name": "会話", "fields": [
      {"id": "text", "name": "本文", "type": "text", "required": true},
      {"id": "role", "name": "話した人", "type": "enum", "options": ["user", "assistant"]}]},
    {"id": "meal", "name": "食事", "titleField": "dish", "fields": [
      {"id": "dish", "name": "料理", "type": "string", "required": true},
      {"id": "date", "name": "日付", "type": "date", "required": true, "defaultValue": "now"},
      {"id": "kind", "name": "区分", "type": "enum", "options": ["朝食", "昼食", "夕食", "間食"]}]}
  ],
  "screens": [
    {"id": "talk", "title": "話す", "components": [
      {"type": "aiChat", "area": "A1:F12", "entity": "turn", "textField": "text", "roleField": "role",
       "persona": "食事の話を聞いて記録を手伝う相棒。食べた物を短く確かめる",
       "saveTo": {"entity": "meal", "fields": ["dish", "date", "kind"]}}]},
    {"id": "log", "title": "記録", "components": [
      {"type": "list", "area": "A1:F12", "entity": "meal", "item": {"title": "dish", "subtitle": "date"}}]}
  ]
}`
	if _, err := dsl.Parse([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}
