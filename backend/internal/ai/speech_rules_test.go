package ai

import (
	"strings"
	"testing"
)

// The speech rules reach the generation prompt, and the catalog / chat index
// (both built from the schema) describe transcribeField and speakButton.
func TestSpeechRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 文字起こし・読み上げ", "**transcribeField**{field", "**speakButton**"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "同じ field に textEditor を重ねて置かない") {
		t.Error("catalog lacks the transcribeField/textEditor overlap rule")
	}
	if !strings.Contains(catalog, "\n- transcribeField: field") {
		t.Error("catalog lacks the transcribeField line with its required keys")
	}
	if !strings.Contains(catalog, "\n- speakButton:") {
		t.Error("catalog lacks the speakButton line")
	}
	for _, c := range []string{"transcribeField", "speakButton"} {
		if !strings.Contains(chatCapabilityIndex, c) {
			t.Errorf("chat capability index lacks %s (chat would answer that it is impossible)", c)
		}
	}
	// The audio rules no longer call transcription impossible.
	if strings.Contains(audioRules, "できないこと: 文字起こし") {
		t.Error("audioRules still says transcription is impossible")
	}
}

// The chat never shows the type names to the user.
func TestSpeechPlainTerms(t *testing.T) {
	got := plainTerms("フォームに transcribeField と speakButton を足します", LocaleJA)
	if strings.Contains(got, "transcribeField") || strings.Contains(got, "speakButton") ||
		!strings.Contains(got, "話して入力する欄") || !strings.Contains(got, "読み上げボタン") {
		t.Errorf("plainTerms = %q", got)
	}
}
