package main

import (
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// masterTappable expect key ([entity id, ...]): each entity ("a|b" = either
// id, since the model names the master) has a reachable record tap that opens
// its form, so saved records can be opened (a table or timeline alone cannot).
// Uses dsl.App.OpenedEntities, the same test as lint savedRecordReach.
// Cases: internal/ai/eval/master_reach_intent_cases.json.
func init() {
	expectKeys["masterTappable"] = expectStrings
}

func checkMasterTappable(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["masterTappable"].([]any)
	if !ok {
		return nil
	}
	got := a.OpenedEntities()
	var miss []string
	for _, w := range want {
		raw, _ := w.(string)
		ids := strings.Split(raw, "|")
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		switch {
		case slices.ContainsFunc(ids, func(id string) bool { return got[id] }):
		case !slices.ContainsFunc(ids, func(id string) bool { return a.EntityByID(id) != nil }):
			miss = append(miss, "masterTappable!"+raw+"(no entity)")
		default:
			miss = append(miss, "masterTappable!"+raw)
		}
	}
	return miss
}
