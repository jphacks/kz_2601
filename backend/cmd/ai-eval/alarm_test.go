package main

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestCheckAlarm(t *testing.T) {
	rec := &dsl.App{App: dsl.AppMeta{Notifications: []dsl.Notification{
		{ID: "n", Title: "t", Repeat: "daily", Time: "09:00"},
		{ID: "a", Title: "t", Repeat: "daily", Entity: "alarm", Field: "time", Alarm: true},
	}}}
	if miss := checkAlarm(map[string]any{"alarm": true, "alarmRecord": true}, rec); miss != nil {
		t.Errorf("want pass, got %v", miss)
	}
	if miss := checkAlarm(map[string]any{}, rec); miss != nil {
		t.Errorf("no keys: %v", miss)
	}
	button := &dsl.App{Screens: []dsl.Screen{{ID: "s", Components: []dsl.Component{
		{Type: dsl.CompButton, Action: &dsl.Action{Type: dsl.ActionScheduleNotif, Notification: &dsl.Notification{ID: "b", Title: "t", Repeat: "once", Time: "07:00", Alarm: true}}},
	}}}}
	if miss := checkAlarm(map[string]any{"alarm": true, "alarmRecord": true}, button); len(miss) != 1 {
		t.Errorf("button alarm, fixed time: want 1 miss, got %v", miss)
	}
	plain := &dsl.App{App: dsl.AppMeta{Notifications: []dsl.Notification{{ID: "n", Title: "t", Repeat: "daily", Time: "09:00"}}}}
	if miss := checkAlarm(map[string]any{"alarm": true}, plain); len(miss) != 1 {
		t.Errorf("plain notification: want 1 miss, got %v", miss)
	}
	if miss := checkAlarm(map[string]any{"alarm": false}, plain); miss != nil {
		t.Errorf("reminder should not be an alarm: %v", miss)
	}
}
