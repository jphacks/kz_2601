package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Live Activity (#423) expect keys, checked by checkLiveActivity:
//
//	liveTimer (bool)       some timerDisplay has liveActivity: true
//	activityButtons (bool) the app has both a startActivity and an endActivity action
//
// Cases: internal/ai/eval/live_activity_cases.json (-cases).

type liveUse struct {
	liveTimers  int
	start, stop int
}

func liveActivityUses(a *dsl.App) liveUse {
	var u liveUse
	count := func(act *dsl.Action) {
		if act == nil {
			return
		}
		switch act.Type {
		case dsl.ActionStartActivity:
			u.start++
		case dsl.ActionEndActivity:
			u.stop++
		}
	}
	for _, s := range a.Screens {
		for _, tb := range s.Toolbar {
			count(&tb.Action)
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompTimerDisplay && x.LiveActivity {
					u.liveTimers++
				}
				count(x.Action)
				count(x.OnItemTap)
			}
		}
	}
	return u
}

func checkLiveActivity(exp map[string]any, a *dsl.App) []string {
	u := liveActivityUses(a)
	var miss []string
	if want, ok := exp["liveTimer"].(bool); ok && (u.liveTimers > 0) != want {
		miss = append(miss, fmt.Sprintf("liveTimer=%d", u.liveTimers))
	}
	if want, ok := exp["activityButtons"].(bool); ok && (u.start > 0 && u.stop > 0) != want {
		miss = append(miss, fmt.Sprintf("activityButtons start=%d end=%d", u.start, u.stop))
	}
	return miss
}
