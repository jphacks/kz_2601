package ai

import (
	"strings"
	"testing"
)

// #264: the flashcard rules reach the generation prompt, and the catalog /
// chat index (both built from the schema) describe flashcards.
func TestFlashcardRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 単語カード・暗記カード(#264)", "flashcards{entity, front: term, back: meaning, toggle: learned", "disclosure・list・carousel で代替しない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- flashcards: entity, front, back") {
		t.Error("catalog lacks the flashcards line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "flashcards") {
		t.Error("chat capability index lacks flashcards (chat would answer 「折りたたみで代替」)")
	}
}
