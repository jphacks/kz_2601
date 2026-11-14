package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Notification.alarm expect keys, checked by checkAlarm:
//
//	alarm       (bool)  some notification (app.notifications or a
//	                    scheduleNotification button) has alarm: true
//	alarmRecord (bool)  one of them takes its time from a record (entity + field)
//
// Cases: internal/ai/eval/alarm_cases.json (-cases).

func alarmNotifications(a *dsl.App) []*dsl.Notification {
	var out []*dsl.Notification
	add := func(n *dsl.Notification) {
		if n != nil && n.Alarm {
			out = append(out, n)
		}
	}
	for i := range a.App.Notifications {
		add(&a.App.Notifications[i])
	}
	act := func(x *dsl.Action) {
		if x != nil && x.Type == dsl.ActionScheduleNotif {
			add(x.Notification)
		}
	}
	for _, s := range a.Screens {
		for i := range s.Toolbar {
			act(&s.Toolbar[i].Action)
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				act(x.Action)
			}
		}
	}
	return out
}

func checkAlarm(exp map[string]any, a *dsl.App) []string {
	alarms := alarmNotifications(a)
	var miss []string
	if want, ok := exp["alarm"].(bool); ok && (len(alarms) > 0) != want {
		miss = append(miss, fmt.Sprintf("alarm=%d", len(alarms)))
	}
	if want, ok := exp["alarmRecord"].(bool); ok {
		record := false
		for _, n := range alarms {
			if n.Entity != "" {
				record = true
			}
		}
		if record != want {
			miss = append(miss, fmt.Sprintf("alarmRecord=%v", record))
		}
	}
	return miss
}
