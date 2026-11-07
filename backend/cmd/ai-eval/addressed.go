package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Addressed messages (#271) expect keys, checked by checkAddressed:
//   - addressed (bool): some entity has addressedTo.
//   - addressedGroup (bool): some addressed entity's recipients are type members.
//   - addressedUnread (bool): some addressed entity has a mark a list shows (item.mark).
//
// Cases: internal/ai/eval/addressed_cases.json (-cases) and
// addressed_chat_cases.json (-chat -cases).
func checkAddressed(exp map[string]any, a *dsl.App) []string {
	var miss []string
	addressed, group, unread := false, false, false
	shown := map[string]bool{} // entity/mark ids that a list item shows
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Item != nil && x.Item.Mark != "" {
					shown[x.Entity+"/"+x.Item.Mark] = true
				}
			}
		}
	}
	for i := range a.Entities {
		e := &a.Entities[i]
		if !e.IsAddressed() {
			continue
		}
		addressed = true
		if f := e.FieldByID(e.AddressedTo); f != nil && f.Type == dsl.FieldMembers {
			group = true
		}
		for _, m := range e.Marks {
			unread = unread || shown[e.ID+"/"+m.ID]
		}
	}
	for key, got := range map[string]bool{"addressed": addressed, "addressedGroup": group, "addressedUnread": unread} {
		if want, ok := exp[key].(bool); ok && got != want {
			miss = append(miss, fmt.Sprintf("%s=%v", key, got))
		}
	}
	return miss
}
