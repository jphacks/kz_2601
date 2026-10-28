package ai

import (
	"strings"
	"testing"
)

// #260: chat requests pick chatThread, and chat is no longer named as a
// missing part to substitute.
func TestChatThreadRulesInPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, chatThreadRules) {
		t.Fatal("system prompt lacks the chatThread rules")
	}
	if strings.Contains(systemPrompt, "タイマー・チャット・グラフ") {
		t.Fatal("prompt still lists chat as a part the catalog lacks")
	}
	if !strings.Contains(chatCapabilityIndex, "chatThread") {
		t.Fatal("capability index lacks chatThread")
	}
}
