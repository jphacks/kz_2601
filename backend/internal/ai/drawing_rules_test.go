package ai

import (
	"strings"
	"testing"
)

// The drawing rules reach the generation prompt, and the catalog / chat index
// (both built from the schema) describe drawingField.
func TestDrawingRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 手書き・お絵かき(drawingField)", "**drawingField**{field", "type \"photo\""} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- drawingField: field") {
		t.Error("catalog lacks the drawingField line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "drawingField") {
		t.Error("chat capability index lacks drawingField (chat would answer 「手書きはできない」)")
	}
}

// The chat never shows the type name to the user.
func TestDrawingFieldPlainTerm(t *testing.T) {
	got := plainTerms("フォームに drawingField を足します", LocaleJA)
	if strings.Contains(got, "drawingField") || !strings.Contains(got, "手書きの欄") {
		t.Errorf("plainTerms = %q", got)
	}
}
