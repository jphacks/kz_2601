package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestAIChatInputRulesReachPrompt(t *testing.T) {
	for _, want := range []string{"## 自分の記録を読んで話す(aiChat inputEntity)", "」は aiChat に inputEntity", "hidden の field・写真・位置は渡らない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if strings.Index(systemPrompt, aiChatRules) > strings.Index(systemPrompt, aiChatInputRules) {
		t.Fatal("inputEntity rules must follow the aiChat rules")
	}
}

// The shapes the rules teach (diary consultation, study questions) parse and
// validate.
func TestAIChatInputRulesShapesValidate(t *testing.T) {
	for name, raw := range map[string]string{
		"diary": `{
  "schemaVersion": 1,
  "app": {"name": "日記相談", "icon": "book", "voiceEntry": "none"},
  "initialScreen": "talk",
  "entities": [
    {"id": "turn", "name": "会話", "fields": [
      {"id": "text", "name": "本文", "type": "text", "required": true},
      {"id": "role", "name": "話した人", "type": "enum", "options": ["user", "assistant"]}]},
    {"id": "diary", "name": "日記", "titleField": "date", "fields": [
      {"id": "date", "name": "日付", "type": "date", "required": true, "defaultValue": "now"},
      {"id": "mood", "name": "気分", "type": "enum", "options": ["よい", "ふつう", "つらい"]},
      {"id": "body", "name": "本文", "type": "text", "required": true}]}
  ],
  "screens": [
    {"id": "talk", "title": "相談", "components": [
      {"type": "aiChat", "area": "A1:F12", "entity": "turn", "textField": "text", "roleField": "role",
       "persona": "日記を読んで気持ちに寄り添う相談相手。記録に書かれた出来事に触れて 1 つずつ質問する",
       "inputEntity": "diary", "limit": 10}]},
    {"id": "log", "title": "日記", "components": [
      {"type": "list", "area": "A1:F12", "entity": "diary", "item": {"title": "date", "subtitle": "body"}}]}
  ]
}`,
		"study": `{
  "schemaVersion": 1,
  "app": {"name": "問題の家庭教師", "icon": "graduationcap", "voiceEntry": "none"},
  "initialScreen": "talk",
  "entities": [
    {"id": "turn", "name": "会話", "fields": [
      {"id": "text", "name": "本文", "type": "text", "required": true},
      {"id": "role", "name": "話した人", "type": "enum", "options": ["user", "assistant"]}]},
    {"id": "problem", "name": "問題", "titleField": "question", "fields": [
      {"id": "subject", "name": "教科", "type": "enum", "options": ["数学", "英語", "理科"]},
      {"id": "question", "name": "問題", "type": "text", "required": true},
      {"id": "answer", "name": "答え", "type": "string", "hidden": true}]}
  ],
  "screens": [
    {"id": "talk", "title": "質問", "components": [
      {"type": "aiChat", "area": "A1:F12", "entity": "turn", "textField": "text", "roleField": "role",
       "persona": "保存した問題の家庭教師。答えをすぐ言わずヒントを出す", "inputEntity": "problem"}]},
    {"id": "list", "title": "問題", "components": [
      {"type": "list", "area": "A1:F12", "entity": "problem", "item": {"title": "question", "subtitle": "subject"}}]}
  ]
}`,
	} {
		if _, err := dsl.Parse([]byte(raw)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
