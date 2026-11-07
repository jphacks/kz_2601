package ai

import (
	"strings"
	"testing"
)

// addToCalendar: the rules reach the generation prompt and the schema-built
// catalog / chat index list the new action and its keys.
func TestCalendarRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"## 端末のカレンダーに入れる(addToCalendar)", "type: addToCalendar", "alertMinutes", "カレンダーの許可は要らない"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	for _, want := range []string{"- addToCalendar{", "calendarEvent", "startField"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
	if !strings.Contains(chatCapabilityIndex, "addToCalendar") {
		t.Error("chat capability index lacks addToCalendar")
	}
}
