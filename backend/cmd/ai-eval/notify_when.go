package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Condition notifications (#265) expect keys, checked by checkNotifyWhen:
//   - notifyWhen (bool): some app.notifications has when.
//   - notifyWhenOp (string): that when's top-level op (lt / gt / gte ...).
//
// Cases: internal/ai/eval/notify_when_cases.json (-cases) and
// notify_when_chat_cases.json (-chat -cases).
func checkNotifyWhen(exp map[string]any, a *dsl.App) []string {
	var miss []string
	var hit *dsl.Notification
	for i := range a.App.Notifications {
		if a.App.Notifications[i].When != nil {
			hit = &a.App.Notifications[i]
			break
		}
	}
	if want, ok := exp["notifyWhen"].(bool); ok && (hit != nil) != want {
		miss = append(miss, fmt.Sprintf("notifyWhen=%v", hit != nil))
	}
	if want, ok := exp["notifyWhenOp"].(string); ok {
		got := ""
		if hit != nil && hit.When.Kind == dsl.ExprOp {
			got = hit.When.Op
		}
		if got != want {
			miss = append(miss, fmt.Sprintf("notifyWhenOp=%q", got))
		}
	}
	return miss
}
