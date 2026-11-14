package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestAIOutputsRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, "## AI の答えを項目ごとに入力欄へ入れる(aiButton outputs)") {
		t.Fatal("outputs rules are not in the system prompt")
	}
}

// The prompt's examples (poster → event form → addToCalendar, scoring an
// answer into an integer field) are valid DSL.
func TestAIOutputsRulesExampleValid(t *testing.T) {
	raw := `{"schemaVersion":1,"app":{"name":"イベント","voiceEntry":"floating"},"initialScreen":"list",
"entities":[
 {"id":"event","name":"イベント","titleField":"title","fields":[
  {"id":"poster","name":"ポスターの文","type":"text"},
  {"id":"title","name":"題名","type":"string","required":true},
  {"id":"start","name":"日時","type":"datetime","required":true},
  {"id":"place","name":"場所","type":"string"}]},
 {"id":"answer","name":"解答","titleField":"question","fields":[
  {"id":"question","name":"問題","type":"string"},
  {"id":"answer","name":"答え","type":"text"},
  {"id":"score","name":"点数","type":"integer"},
  {"id":"comment","name":"講評","type":"string"}]}],
"screens":[
 {"id":"list","title":"イベント","components":[
  {"type":"list","entity":"event","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form"}}],
  "toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form"}},{"label":"採点","action":{"type":"navigate","screen":"score"}}]},
 {"id":"form","title":"イベント","entity":"event","components":[
  {"type":"textField","field":"poster","multiline":true,"area":"A1:F2"},
  {"type":"aiButton","task":"generate","label":"文から入力","input":"poster","prompt":"{{poster}} からイベントの題名・開始日時・場所を抜き出す","shape":"fields","items":["題名","日時","場所"],"outputs":{"題名":"title","日時":"start","場所":"place"},"area":"A3:F8"},
  {"type":"textField","field":"title","area":"A9:F9"},
  {"type":"datePicker","field":"start","area":"A10:F10"},
  {"type":"textField","field":"place","area":"A11:F11"},
  {"type":"button","label":"カレンダーに追加","action":{"type":"addToCalendar","entity":"event","calendarEvent":{"startField":"start","titleField":"title","locationField":"place"}},"area":"A12:F12"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"event"}}]},
 {"id":"score","title":"採点","entity":"answer","components":[
  {"type":"textField","field":"question"},
  {"type":"textField","field":"answer","multiline":true},
  {"type":"aiButton","task":"generate","label":"採点する","prompt":"{{question}} への答え {{answer}} を 100 点満点で採点し、講評を一言","shape":"fields","items":["点数","講評"],"outputs":{"点数":"score","講評":"comment"}},
  {"type":"numberField","field":"score"},
  {"type":"textField","field":"comment"}],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"answer"}}]}]}`
	if _, err := dsl.Parse([]byte(raw)); err != nil {
		t.Fatalf("example rejected: %v", err)
	}
}
