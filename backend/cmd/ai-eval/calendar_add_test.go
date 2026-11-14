package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckCalendarAdd(t *testing.T) {
	alert := 1440
	app := &dsl.App{Screens: []dsl.Screen{{ID: "form", Components: []dsl.Component{
		{Type: dsl.CompButton, Action: &dsl.Action{Type: dsl.ActionAddCalendar, Entity: "e", CalendarEvent: &dsl.CalendarEvent{StartField: "d", AlertMinutes: &alert}}},
	}}}}
	if miss := checkCalendarAdd(map[string]any{"addToCalendar": true, "calendarAlert": true}, app); miss != nil {
		t.Errorf("want pass, got %v", miss)
	}
	if miss := checkCalendarAdd(map[string]any{}, app); miss != nil {
		t.Errorf("no keys: %v", miss)
	}
	plain := &dsl.App{Screens: []dsl.Screen{{ID: "form", Toolbar: []dsl.ToolbarButton{{Label: "x", Action: dsl.Action{Type: dsl.ActionAddCalendar, CalendarEvent: &dsl.CalendarEvent{StartField: "d"}}}}}}}
	if miss := checkCalendarAdd(map[string]any{"addToCalendar": true, "calendarAlert": true}, plain); len(miss) != 1 {
		t.Errorf("toolbar counts, no alert: want 1 miss, got %v", miss)
	}
	if miss := checkCalendarAdd(map[string]any{"addToCalendar": true}, &dsl.App{}); len(miss) != 1 {
		t.Errorf("empty app: want 1 miss, got %v", miss)
	}
}
