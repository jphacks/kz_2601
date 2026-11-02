package ai

import (
	"strings"
	"testing"
)

// #266: subscriptions / birthdays repeat monthly / yearly with nextOccurrence.
func TestPromptTeachesRepeatMonthlyYearly(t *testing.T) {
	for _, want := range []string{
		`repeat: "monthly"(毎月)/ "yearly"(毎年)`,
		"repeat once にしない",
		`{"op":"nextOccurrence","args":[{"ref":"renew_on"},"monthly"]}`,
		"offsetMinutes(当日 0・前日 -1440",
		"日が無い月は月末に寄せる",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, repeatChatRule) {
			t.Errorf("%s lacks the repeat chat rule", name)
		}
		if !strings.Contains(p, retentionChatRule) {
			t.Errorf("%s lost the retention chat rule", name)
		}
	}
	if !strings.HasSuffix(chatCreateSystemPrompt, chatCapabilityIndex) {
		t.Fatal("chat create prompt no longer ends with the capability index")
	}
	if got := plainTerms("nextOccurrence で次の更新日を出します", LocaleJA); strings.Contains(got, "nextOccurrence") {
		t.Fatalf("chat leaks the DSL name: %q", got)
	}
}
