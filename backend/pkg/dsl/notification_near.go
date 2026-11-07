package dsl

import "strings"

// Notification.near / jitterMinutes limits.
const (
	DefaultNearRadiusM = 200
	MinNearRadiusM     = 100
	MaxNearRadiusM     = 2000
	MaxJitterMinutes   = 60
)

// checkNearNotification validates Notification.near (arrive near a record's
// place) and jitterMinutes (random shift of a time trigger) on
// app.notifications. near is its own trigger like when: no time, record date,
// condition or alarm with it.
func checkNearNotification(where string, n *Notification, entities map[string]*Entity, add func(string, ...any)) {
	if n.JitterMinutes != 0 {
		if n.JitterMinutes < 1 || n.JitterMinutes > MaxJitterMinutes {
			add("%s: jitterMinutes %d out of range (1-%d)", where, n.JitterMinutes, MaxJitterMinutes)
		}
		if n.When != nil || n.Near != nil {
			add("%s: jitterMinutes shifts a time (time / record date); remove it from a when / near notification", where)
		}
	}
	if n.Near == nil {
		return
	}
	var extra []string
	for _, kv := range []struct {
		key string
		set bool
	}{
		{"time", n.Time != ""}, {"weekday", n.Weekday != ""}, {"entity", n.Entity != ""}, {"field", n.Field != ""},
		{"offsetMinutes", n.OffsetMinutes != 0}, {"titleField", n.TitleField != ""}, {"enabledField", n.EnabledField != ""},
		{"when", n.When != nil}, {"alarm", n.Alarm},
	} {
		if kv.set {
			extra = append(extra, kv.key)
		}
	}
	if len(extra) > 0 {
		add("%s: a near notification fires on arriving at the place; remove %s (near is its own trigger; the places are near.entity's records)", where, strings.Join(extra, "/"))
	}
	if n.Repeat != "once" && n.Repeat != "daily" {
		add("%s: a near notification needs repeat once (first arrival only) or daily (every arrival); got %q", where, n.Repeat)
	}
	nr := n.Near
	if r := nr.RadiusM; r != 0 && (r < MinNearRadiusM || r > MaxNearRadiusM) {
		add("%s.near: radiusM %d out of range (%d-%d)", where, r, MinNearRadiusM, MaxNearRadiusM)
	}
	e := entities[nr.Entity]
	if e == nil {
		add("%s.near: entity %q does not exist", where, nr.Entity)
		return
	}
	switch f := e.FieldByID(nr.LocationField); {
	case f == nil:
		add("%s.near: locationField %q does not exist in entity %q", where, nr.LocationField, nr.Entity)
	case f.Type != FieldLocation:
		add("%s.near: locationField %q must be a location field (got %s)", where, nr.LocationField, f.Type)
	}
}

// checkNearAction: near / jitterMinutes belong in app.notifications, not in a
// scheduleNotification button.
func checkNearAction(where string, n *Notification, add func(string, ...any)) {
	if n.Near != nil {
		add("%s: near notification belongs in app.notifications, not scheduleNotification", where)
	}
	if n.JitterMinutes != 0 {
		add("%s: jitterMinutes belongs in app.notifications, not scheduleNotification", where)
	}
}
