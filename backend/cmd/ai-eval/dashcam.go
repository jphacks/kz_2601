package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Dashcam (#410) expect key: dashcamAuto (bool: some dashcam has autoStart,
// i.e. 「スタンドに置いたら自動で」 became a condition and not a manual button).
// Cases: internal/ai/eval/dashcam_cases.json (-cases).

// dashcamAutoStarts lists the autoStart of every dashcam in the app.
func dashcamAutoStarts(a *dsl.App) [][]string {
	var out [][]string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompDashcam {
					out = append(out, x.AutoStart)
				}
			}
		}
	}
	return out
}

func checkDashcam(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["dashcamAuto"].(bool)
	if !ok {
		return nil
	}
	got := dashcamAutoStarts(a)
	auto := false
	for _, conds := range got {
		auto = auto || len(conds) > 0
	}
	if auto != want {
		return []string{fmt.Sprintf("dashcamAuto=%v", got)}
	}
	return nil
}
