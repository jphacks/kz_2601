package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestAISaveToRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, "## AI が作った項目を 1 件ずつ記録に足す(aiButton saveTo)") {
		t.Fatal("saveTo rules are not in the system prompt")
	}
}

// The prompt's examples (quiz questions into flashcards, from notes) are valid DSL.
func TestAISaveToRulesExampleValid(t *testing.T) {
	raw := `{"schemaVersion":1,"app":{"name":"英単語","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"card","name":"カード","titleField":"question","fields":[
  {"id":"question","name":"問題","type":"string","required":true},
  {"id":"answer","name":"答え","type":"text"},
  {"id":"learned","name":"覚えた","type":"boolean"}]},
 {"id":"note","name":"ノート","fields":[{"id":"body","name":"本文","type":"text"}]}],
"screens":[
 {"id":"home","title":"英単語","components":[
  {"type":"aiButton","label":"問題を作る","task":"generate","prompt":"中学英語の単語の問題(英単語の意味を問う)","shape":"list","count":5,"saveTo":{"entity":"card","field":"question"},"area":"A1:F6"},
  {"type":"aiButton","id":"from_note","label":"ノートから問題","task":"generate","entity":"note","prompt":"{{input}} から確認の問題","shape":"list","saveTo":{"entity":"card","field":"question"},"area":"A7:F12"},
  {"type":"flashcards","entity":"card","front":"question","back":"answer","toggle":"learned","area":"A13:F20"}]}]}`
	if _, err := dsl.Parse([]byte(raw)); err != nil {
		t.Fatalf("example rejected: %v", err)
	}
}
