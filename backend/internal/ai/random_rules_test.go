package ai

import (
	"strings"
	"testing"
)

// #263: the random rules reach the generation prompt, and the catalog / chat
// index (both built from the schema) describe randomPicker.
func TestRandomRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## ランダムに決める(#263)", "list だけ"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- randomPicker: entity, titleField") {
		t.Error("catalog lacks the randomPicker line with its required keys")
	}
	// The shape lives in the catalog's 使い方 (the rules point to it).
	for _, want := range []string{"randomPicker を上に A1:F5", "titleField に name"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
	if !strings.Contains(chatCapabilityIndex, "randomPicker") {
		t.Error("chat capability index lacks randomPicker (chat would answer 「乱数は無い」)")
	}
}
