package ai

import (
	"strings"
	"testing"
)

// The edit line follows the inputEntity section and routes to aiChat.
func TestAIChatInputEditRuleInPrompt(t *testing.T) {
	if strings.Count(systemPrompt, aiChatInputRules+"\n"+aiChatInputEditRule) != 1 {
		t.Fatal("edit line does not follow the inputEntity section")
	}
	for _, s := range []string{"」は aiChat に inputEntity を足す patch", "は inputEntity を付けない時の話"} {
		if !strings.Contains(aiChatInputEditRule, s) {
			t.Errorf("line lacks %q", s)
		}
	}
}
