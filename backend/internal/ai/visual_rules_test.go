package ai

import (
	"strings"
	"testing"
)

// #140: the visual rules reach the generation prompt (create and edit share systemPrompt).
func TestSystemPromptHasVisualRules(t *testing.T) {
	for _, want := range []string{"A11:B12", "4 行以上", "比率(0〜1)", "item.title に date", `tone "positive"`, `tone "negative"`} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "tone") {
		t.Error("catalog (from schema x-usage) should mention progressBar tone")
	}
}
