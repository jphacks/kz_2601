package ai

import (
	"strings"
	"testing"
)

// #418: the photo reading rules reach the generation prompt, and the catalog /
// chat index (both built from the schema) say photoField can read photos.
func TestPhotoAIRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 写真を AI で読み取る・骨格(#418)", "{kind: amount, into: amount}", "jointAngle"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "analyze") || !strings.Contains(catalog, "kind:jointAngle") {
		t.Error("catalog lacks photoField.analyze and its usage")
	}
	idx := chatCapabilityIndex[strings.Index(chatCapabilityIndex, "  - photoField:"):]
	idx = idx[:strings.Index(idx, "\n")]
	if !strings.Contains(idx, "文字・金額") {
		t.Errorf("chat capability index photoField line does not mention reading: %s", idx)
	}
	if !strings.Contains(chatLimitsText(), "リアルタイム") {
		t.Error("chat limits lack the live-camera counting limit")
	}
}
