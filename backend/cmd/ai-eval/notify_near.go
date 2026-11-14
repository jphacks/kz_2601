package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Notification.near / jitterMinutes expect keys, checked by checkNotifyNear:
//   - notifyNear (bool): some app.notifications has near on a location field.
//   - notifyNearRepeat (string): that notification's repeat (once / daily).
//   - jitter (bool): some app.notifications has jitterMinutes.
//
// Cases: internal/ai/eval/notify_near_cases.json (-cases).
func checkNotifyNear(exp map[string]any, a *dsl.App) []string {
	var miss []string
	var near *dsl.Notification
	jitter := false
	for i := range a.App.Notifications {
		n := &a.App.Notifications[i]
		if n.Near != nil && near == nil {
			if e := a.EntityByID(n.Near.Entity); e != nil {
				if f := e.FieldByID(n.Near.LocationField); f != nil && f.Type == dsl.FieldLocation {
					near = n
				}
			}
		}
		jitter = jitter || n.JitterMinutes > 0
	}
	if want, ok := exp["notifyNear"].(bool); ok && (near != nil) != want {
		miss = append(miss, fmt.Sprintf("notifyNear=%v", near != nil))
	}
	if want, ok := exp["notifyNearRepeat"].(string); ok {
		got := ""
		if near != nil {
			got = near.Repeat
		}
		if got != want {
			miss = append(miss, fmt.Sprintf("notifyNearRepeat=%q", got))
		}
	}
	if want, ok := exp["jitter"].(bool); ok && jitter != want {
		miss = append(miss, fmt.Sprintf("jitter=%v", jitter))
	}
	return miss
}
