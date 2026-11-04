package dsl

import "strings"

// checkWhenNotification validates Notification.when (#265): a boolean
// expression over the app's records (no row: agg, not ref). It is its own
// trigger, so it excludes the time trigger (time / weekday, repeat other than
// once) and the record trigger (entity / field / ...).
func checkWhenNotification(where string, n *Notification, entities map[string]*Entity, add func(string, ...any)) {
	if n.When == nil {
		return
	}
	var extra []string
	for _, kv := range []struct {
		key string
		set bool
	}{
		{"time", n.Time != ""}, {"weekday", n.Weekday != ""}, {"entity", n.Entity != ""}, {"field", n.Field != ""},
		{"offsetMinutes", n.OffsetMinutes != 0}, {"titleField", n.TitleField != ""}, {"enabledField", n.EnabledField != ""},
	} {
		if kv.set {
			extra = append(extra, kv.key)
		}
	}
	if len(extra) > 0 {
		add("%s: a when notification fires when the condition turns true; remove %s (when is its own trigger, not combined with a time or a record date)", where, strings.Join(extra, "/"))
	}
	if n.Repeat != "once" {
		add("%s: a when notification needs repeat once (it fires once each time the condition turns true; got %q)", where, n.Repeat)
	}
	ch := &exprChecker{entities: entities, add: add}
	if got := ch.check(where+".when", n.When, nil, 1); !accepts(tBool, got) {
		add("%s.when: condition must be true/false (got %s); compare with lt / gt / gte / eq, e.g. {\"op\":\"lt\",\"args\":[{\"agg\":\"sum\",\"entity\":\"stock\",\"field\":\"qty\"},5]}", where, got)
	}
}
