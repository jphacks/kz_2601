package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// addToCalendar expect keys, checked by checkCalendarAdd:
//
//	addToCalendar (bool)  a button / toolbar / onItemTap runs addToCalendar
//	calendarAlert (bool)  one of them sets calendarEvent.alertMinutes
//
// Cases: internal/ai/eval/calendar_cases.json (-cases).

func calendarActions(a *dsl.App) []*dsl.Action {
	var out []*dsl.Action
	add := func(act *dsl.Action) {
		if act != nil && act.Type == dsl.ActionAddCalendar {
			out = append(out, act)
		}
	}
	for _, s := range a.Screens {
		for i := range s.Toolbar {
			add(&s.Toolbar[i].Action)
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				add(x.Action)
				add(x.OnItemTap)
			}
		}
	}
	return out
}

func checkCalendarAdd(exp map[string]any, a *dsl.App) []string {
	acts := calendarActions(a)
	var miss []string
	if want, ok := exp["addToCalendar"].(bool); ok && (len(acts) > 0) != want {
		miss = append(miss, fmt.Sprintf("addToCalendar=%d", len(acts)))
	}
	if want, ok := exp["calendarAlert"].(bool); ok {
		alert := false
		for _, act := range acts {
			if act.CalendarEvent != nil && act.CalendarEvent.AlertMinutes != nil {
				alert = true
			}
		}
		if alert != want {
			miss = append(miss, fmt.Sprintf("calendarAlert=%v", alert))
		}
	}
	return miss
}
