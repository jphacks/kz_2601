package ai

import (
	"strings"
	"testing"
)

// The when-not line follows the outputs section and routes to aiButton.
func TestAIOutputsWhenRuleInPrompt(t *testing.T) {
	want := aiOutputsRules + "\n" + aiOutputsWhenRule
	if strings.Count(systemPrompt, want) != 1 {
		t.Fatal("when-not line does not follow the outputs section")
	}
	for _, s := range []string{"表示するだけ)なら outputs を付けない", "」は aiButton の saveTo(shape: list)"} {
		if !strings.Contains(aiOutputsWhenRule, s) {
			t.Errorf("line lacks %q", s)
		}
	}
}
