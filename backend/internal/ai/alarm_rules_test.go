package ai

import (
	"strings"
	"testing"
)

func TestPromptTeachesAlarm(t *testing.T) {
	for _, want := range []string{
		"**alarm: true**",
		"string + timePicker の \"HH:mm\"",
		"when(条件の通知)には付けない",
		"add /app/notifications/<i>/alarm",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, alarmChatRule) {
			t.Errorf("%s lacks the alarm chat rule", name)
		}
		if !strings.Contains(p, repeatChatRule) {
			t.Errorf("%s lost the repeat chat rule", name)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
}
