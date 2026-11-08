package main

import (
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// masterTappable expect key ([entity id, ...]): each entity ("a|b" = either
// id, since the model names the master) is shown on a
// reachable screen by a list / calendar / mapView / gridTable whose tap
// navigates to that entity's form, so saved records can be opened (a table or
// timeline alone cannot). Mirrors dsl lint savedRecordReach.
// Cases: internal/ai/eval/master_reach_intent_cases.json.
func init() {
	expectKeys["masterTappable"] = expectStrings
}

var recordTapTypes = map[dsl.ComponentType]bool{
	dsl.CompList: true, dsl.CompCalendar: true, dsl.CompMapView: true, dsl.CompGridTable: true,
}

// tappableEntities: entities a reachable record tap opens the form of.
func tappableEntities(a *dsl.App) map[string]bool {
	formOf := map[string]string{}
	for _, s := range a.Screens {
		if s.Entity != "" {
			formOf[s.ID] = s.Entity
		}
	}
	reach := a.Reachable()
	out := map[string]bool{}
	for _, s := range a.Screens {
		if !reach[s.ID] {
			continue
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if !recordTapTypes[x.Type] || x.Entity == "" {
					continue
				}
				tap := x.OnItemTap
				if x.Type == dsl.CompCalendar {
					tap = x.OnDateTap
				}
				if tap != nil && tap.Type == dsl.ActionNavigate && formOf[tap.Screen] == x.Entity {
					out[x.Entity] = true
				}
			}
		}
	}
	return out
}

func checkMasterTappable(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["masterTappable"].([]any)
	if !ok {
		return nil
	}
	got := tappableEntities(a)
	var miss []string
	for _, w := range want {
		ids, _ := w.(string)
		if !slices.ContainsFunc(strings.Split(ids, "|"), func(id string) bool { return got[id] }) {
			miss = append(miss, "masterTappable!"+ids)
		}
	}
	return miss
}
