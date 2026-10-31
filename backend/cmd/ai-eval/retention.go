package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Record retention (#482) expect keys, checked by checkRetention:
//   - retention (number): some entity has retentionDays equal to this; 0 =
//     no entity has retentionDays (removed / never set).
//   - retentionOnRef (bool): an entity that a ref / refs field points to (a
//     master list such as categories or members) has retentionDays.
//
// Cases: internal/ai/eval/retention_cases.json (-cases) and
// retention_chat_cases.json (-chat -cases).

// refTargets lists the entities some ref / refs field points to.
func refTargets(a *dsl.App) map[string]bool {
	out := map[string]bool{}
	for i := range a.Entities {
		for _, f := range a.Entities[i].Fields {
			if (f.Type == dsl.FieldRef || f.Type == dsl.FieldRefs) && f.RefEntity != "" {
				out[f.RefEntity] = true
			}
		}
	}
	return out
}

func checkRetention(exp map[string]any, a *dsl.App) []string {
	var miss []string
	var days []int
	onRef := false
	targets := refTargets(a)
	for i := range a.Entities {
		if d := a.Entities[i].RetentionDays; d > 0 {
			days = append(days, d)
			onRef = onRef || targets[a.Entities[i].ID]
		}
	}
	if want, ok := exp["retention"].(float64); ok {
		got := false
		if want == 0 {
			got = len(days) == 0
		}
		for _, d := range days {
			got = got || float64(d) == want
		}
		if !got {
			miss = append(miss, fmt.Sprintf("retention=%v", days))
		}
	}
	if want, ok := exp["retentionOnRef"].(bool); ok && onRef != want {
		miss = append(miss, fmt.Sprintf("retentionOnRef=%v", onRef))
	}
	return miss
}
