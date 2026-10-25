package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// #228: record-based notifications (alarm time / N minutes before an event).
func TestRecordNotificationValid(t *testing.T) {
	app := memoApp()
	app.App.Notifications = []Notification{
		{ID: "before", Title: "まもなく", Body: "", Repeat: "once", Entity: "memo", Field: "date", OffsetMinutes: -10, TitleField: "note"},
		{ID: "alarm", Title: "起床", Body: "", Repeat: "daily", Entity: "memo", Field: "note"},
	}
	if err := app.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	raw, _ := json.Marshal(app.App.Notifications[0])
	for _, k := range []string{`"entity":"memo"`, `"field":"date"`, `"offsetMinutes":-10`, `"titleField":"note"`} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("json lacks %s: %s", k, raw)
		}
	}
}

func TestRecordNotificationErrors(t *testing.T) {
	app := memoApp()
	app.App.Notifications = []Notification{
		{ID: "a", Title: "t", Repeat: "once", Entity: "nope", Field: "date"},
		{ID: "b", Title: "t", Repeat: "once", Entity: "memo", Field: "elapsed"},
		{ID: "c", Title: "t", Repeat: "once", Entity: "memo", Field: "date", EnabledField: "note"},
		{ID: "d", Title: "t", Repeat: "once", Field: "date"},
		{ID: "e", Title: "t", Repeat: "once", Entity: "memo"},
	}
	app.Screens[0].Components = []Component{{Type: CompButton, Label: "x", Action: &Action{
		Type: ActionScheduleNotif, Notification: &Notification{ID: "f", Title: "t", Repeat: "once", Entity: "memo", Field: "date"},
	}}}
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		`notification entity "nope" does not exist`,
		`notification field "elapsed" must be datetime, date or string`,
		`enabledField "note" must be a boolean field`,
		"field/titleField/enabledField/offsetMinutes need entity",
		"record notification needs field",
		"belongs in app.notifications",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}
