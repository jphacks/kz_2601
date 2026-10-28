package ai

import (
	"strings"
	"testing"
)

// #420: the audio rules reach the generation prompt, and the catalog / chat
// index (both built from the schema) describe audioRecorder and the audio type.
func TestAudioRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 録音・ボイスメモ(#420)", "**audioRecorder**{field", "voiceInput にしない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- audioRecorder: field") {
		t.Error("catalog lacks the audioRecorder line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "audioRecorder") {
		t.Error("chat capability index lacks audioRecorder (chat would answer 「録音はできない」)")
	}
	if !strings.Contains(catalog, "audio") {
		t.Error("catalog lacks the audio field type")
	}
}

// The chat never shows the type name to the user.
func TestAudioRecorderPlainTerm(t *testing.T) {
	got := plainTerms("フォームに audioRecorder を足します", LocaleJA)
	if strings.Contains(got, "audioRecorder") || !strings.Contains(got, "録音の欄") {
		t.Errorf("plainTerms = %q", got)
	}
}
