package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

func calendarApp(act Action) App {
	app := Initial()
	app.Entities = []Entity{{ID: "event", Name: "予定", TitleField: "name", Fields: []Field{
		{ID: "name", Name: "名前", Type: FieldString},
		{ID: "day", Name: "日", Type: FieldDate},
		{ID: "start_at", Name: "開始", Type: FieldDatetime},
		{ID: "end_at", Name: "終了", Type: FieldDatetime},
		{ID: "place", Name: "場所", Type: FieldString},
		{ID: "memo", Name: "メモ", Type: FieldText},
		{ID: "kind", Name: "種類", Type: FieldEnum, Options: []string{"授業", "バイト"}},
		{ID: "fee", Name: "会費", Type: FieldInteger},
	}}}
	app.Screens[0].Components = []Component{{Type: CompButton, ID: "cal", Label: "カレンダーに追加", Action: &act}}
	return app
}

func intp(n int) *int { return &n }

func TestCalendarActionValid(t *testing.T) {
	for _, ev := range []*CalendarEvent{
		{StartField: "day"},
		{StartField: "start_at", EndField: "end_at", TitleField: "kind", LocationField: "place", NotesField: "memo", AlertMinutes: intp(0)},
		{StartField: "start_at", DurationMinutes: MaxCalendarMinutes, AlertMinutes: intp(1440)},
	} {
		app := calendarApp(Action{Type: ActionAddCalendar, Entity: "event", CalendarEvent: ev})
		if err := app.Validate(); err != nil {
			t.Errorf("%+v rejected: %v", ev, err)
		}
	}
}

func TestCalendarActionErrors(t *testing.T) {
	for want, act := range map[string]Action{
		`entity "nope" does not exist`:    {Type: ActionAddCalendar, Entity: "nope", CalendarEvent: &CalendarEvent{StartField: "day"}},
		"needs calendarEvent{startField}": {Type: ActionAddCalendar, Entity: "event"},
		"startField is required":          {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{}},
		`startField "name" is string`:     {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{StartField: "name"}},
		`endField "x" not in entity`:      {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{StartField: "day", EndField: "x"}},
		"endField is the same as":         {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{StartField: "day", EndField: "day"}},
		`titleField "fee" is integer`:     {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{StartField: "day", TitleField: "fee"}},
		`notesField "kind" is enum`:       {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{StartField: "day", NotesField: "kind"}},
		"durationMinutes must be 1-10080": {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{StartField: "start_at", DurationMinutes: -1}},
		"alertMinutes must be 0-10080":    {Type: ActionAddCalendar, Entity: "event", CalendarEvent: &CalendarEvent{StartField: "start_at", AlertMinutes: intp(MaxCalendarMinutes + 1)}},
		`unknown action type "addCalenda`: {Type: "addCalendar"},
	} {
		app := calendarApp(act)
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
}

// alertMinutes: 0 (at the start) survives a round trip; absent stays absent.
func TestCalendarEventJSON(t *testing.T) {
	raw := `{"type":"addToCalendar","entity":"event","calendarEvent":{"startField":"start_at","alertMinutes":0}}`
	var act Action
	if err := json.Unmarshal([]byte(raw), &act); err != nil {
		t.Fatal(err)
	}
	if act.CalendarEvent == nil || act.CalendarEvent.AlertMinutes == nil || *act.CalendarEvent.AlertMinutes != 0 {
		t.Fatalf("alertMinutes 0 lost: %+v", act.CalendarEvent)
	}
	out, _ := json.Marshal(act)
	if !strings.Contains(string(out), `"alertMinutes":0`) {
		t.Errorf("round trip lost alertMinutes: %s", out)
	}
	act.CalendarEvent.AlertMinutes = nil
	out, _ = json.Marshal(act)
	if strings.Contains(string(out), "alertMinutes") {
		t.Errorf("absent alertMinutes written: %s", out)
	}
}
