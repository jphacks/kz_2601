package dsl

import "slices"

// addToCalendar: put the current record into the iPhone's own calendar
// (Calendar app / Google Calendar synced to it). iOS shows the system "new
// event" sheet filled from the record (EventKitUI, iOS 17+): the user taps
// Add, so the app needs no calendar permission, entitlement or Info.plist key.

// MaxCalendarMinutes caps durationMinutes / alertMinutes at one week.
const MaxCalendarMinutes = 7 * 24 * 60

// CalendarEvent maps a record's fields onto a calendar event.
type CalendarEvent struct {
	StartField      string `json:"startField"`                // date (all-day) or datetime
	EndField        string `json:"endField,omitempty"`        // date / datetime; empty value -> durationMinutes
	DurationMinutes int    `json:"durationMinutes,omitempty"` // default 60 (timed events)
	TitleField      string `json:"titleField,omitempty"`      // default entity.titleField, then the first string
	LocationField   string `json:"locationField,omitempty"`   // string / text / enum (place name)
	NotesField      string `json:"notesField,omitempty"`      // string / text
	AlertMinutes    *int   `json:"alertMinutes,omitempty"`    // alert N minutes before; 0 = at the start
}

var (
	calendarDateTypes = []FieldType{FieldDate, FieldDatetime}
	calendarTextTypes = []FieldType{FieldString, FieldText, FieldEnum}
)

func checkCalendarAction(where string, act *Action, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[act.Entity]
	if e == nil {
		add("%s: addToCalendar entity %q does not exist; set entity to the entity whose record becomes the event", where, act.Entity)
		return
	}
	ev := act.CalendarEvent
	if ev == nil {
		add("%s: addToCalendar needs calendarEvent{startField}", where)
		return
	}
	field := func(key, id string, types []FieldType, required bool) {
		if id == "" {
			if required {
				add("%s: addToCalendar calendarEvent.%s is required (a date / datetime field of %q)", where, key, e.ID)
			}
			return
		}
		f := e.FieldByID(id)
		switch {
		case f == nil:
			add("%s: addToCalendar calendarEvent.%s %q not in entity %q (fields: %s)", where, key, id, e.ID, fieldIDs(e))
		case !slices.Contains(types, f.Type):
			add("%s: addToCalendar calendarEvent.%s %q is %s; use a %v field", where, key, id, f.Type, types)
		}
	}
	field("startField", ev.StartField, calendarDateTypes, true)
	field("endField", ev.EndField, calendarDateTypes, false)
	field("titleField", ev.TitleField, calendarTextTypes, false)
	field("locationField", ev.LocationField, calendarTextTypes, false)
	field("notesField", ev.NotesField, []FieldType{FieldString, FieldText}, false)
	if ev.EndField != "" && ev.EndField == ev.StartField {
		add("%s: addToCalendar calendarEvent.endField is the same as startField; omit it and use durationMinutes", where)
	}
	if ev.DurationMinutes < 0 || ev.DurationMinutes > MaxCalendarMinutes {
		add("%s: addToCalendar calendarEvent.durationMinutes must be 1-%d (got %d); omit it for 60", where, MaxCalendarMinutes, ev.DurationMinutes)
	}
	if m := ev.AlertMinutes; m != nil && (*m < 0 || *m > MaxCalendarMinutes) {
		add("%s: addToCalendar calendarEvent.alertMinutes must be 0-%d minutes before the start (got %d)", where, MaxCalendarMinutes, *m)
	}
}
