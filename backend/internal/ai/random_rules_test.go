package ai

import (
	"strings"
	"testing"
)

// #263: the random rules reach the generation prompt, and the catalog / chat
// index (both built from the schema) describe randomPicker.
func TestRandomRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## ランダムに決める(#263)", "randomPicker{entity, titleField: name", "list だけ"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- randomPicker: entity, titleField") {
		t.Error("catalog lacks the randomPicker line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "randomPicker") {
		t.Error("chat capability index lacks randomPicker (chat would answer 「乱数は無い」)")
	}
}
