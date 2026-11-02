package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Monthly / yearly record notifications (#266) expect keys, checked by checkRepeat:
//   - repeat (string): some record-based app.notifications has this repeat
//     (monthly / yearly; it needs entity + field).
//   - repeatOffset (number): that notification's offsetMinutes (-1440 = the day before).
//   - nextOccurrence (bool): some expression uses op nextOccurrence.
//
// Cases: internal/ai/eval/repeat_cases.json (-cases) and
// repeat_chat_cases.json (-chat -cases).
func checkRepeat(exp map[string]any, a *dsl.App) []string {
	var miss []string
	if want, ok := exp["repeat"].(string); ok {
		var got []string
		var hit *dsl.Notification
		for i := range a.App.Notifications {
			n := &a.App.Notifications[i]
			got = append(got, n.Repeat)
			if n.Repeat == want && n.Entity != "" && hit == nil {
				hit = n
			}
		}
		if hit == nil {
			miss = append(miss, fmt.Sprintf("repeat=%v", got))
		} else if off, ok := exp["repeatOffset"].(float64); ok && float64(hit.OffsetMinutes) != off {
			miss = append(miss, fmt.Sprintf("repeatOffset=%d", hit.OffsetMinutes))
		}
	}
	if want, ok := exp["nextOccurrence"].(bool); ok {
		raw, _ := json.Marshal(a)
		if got := strings.Contains(string(raw), `"op":"nextOccurrence"`); got != want {
			miss = append(miss, fmt.Sprintf("nextOccurrence=%v", got))
		}
	}
	return miss
}
