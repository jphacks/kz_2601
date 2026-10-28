package ai

import (
	"strings"
	"testing"
)

// #423: the rules reach the generation prompt and the schema-built catalog /
// chat index list the new keys and actions.
func TestLiveActivityRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## ロック画面・Dynamic Island(ライブアクティビティ・#423)", `"liveActivity": true`, "type: endActivity", "画面を出したまま"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for _, want := range []string{"- startActivity{activity", "- endActivity", "liveActivity", "- activity{title"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
	for _, a := range []string{"startActivity", "endActivity"} {
		if !strings.Contains(chatCapabilityIndex, a) {
			t.Errorf("chat capability index lacks %s", a)
		}
	}
	for _, l := range chatLimits {
		if strings.Contains(l.What, "ウィジェット") && !strings.Contains(l.Instead, "自動") {
			t.Errorf("limit %q says widgets are impossible; stats show up there automatically", l.What)
		}
	}
}
