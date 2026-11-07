package ai

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestTextHighlightRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, "## 文の大事な語に印をつける(text.highlight)") {
		t.Fatal("text.highlight rules are not in the system prompt")
	}
}

// The prompt's example (jphacks/E_2202: error text -> search query and the
// important words highlighted) is a valid DSL.
func TestTextHighlightRulesExampleValid(t *testing.T) {
	raw := `{"schemaVersion":1,"app":{"name":"エラー検索","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"error","name":"エラー","titleField":"query","fields":[
 {"id":"message","name":"エラー文","type":"text","required":true},
 {"id":"keywords","name":"大事な語","type":"string"},
 {"id":"query","name":"検索の言葉","type":"string"}]}],
"screens":[
 {"id":"home","title":"エラー","components":[
  {"type":"list","entity":"error","item":{"title":"query","subtitle":"message"},"onItemTap":{"type":"navigate","screen":"form"}},
  {"type":"button","label":"追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}]},
 {"id":"form","title":"エラー文","entity":"error","components":[
  {"type":"textField","field":"message","multiline":true},
  {"type":"aiButton","label":"大事な語を出す","task":"extract","input":"message","output":"keywords","prompt":"{{message}} から大事な語を 5 個まで、「,」区切りで"},
  {"type":"aiButton","label":"検索の言葉を出す","task":"suggest","input":"message","output":"query","prompt":"{{message}} を調べる検索の言葉を 1 行で"},
  {"type":"text","text":"{{message}}","highlight":{"field":"keywords"}},
  {"type":"button","label":"保存","action":{"type":"saveRecord","entity":"error"}}]}]}`
	if _, err := dsl.Parse([]byte(raw)); err != nil {
		t.Fatalf("example rejected: %v", err)
	}
}
