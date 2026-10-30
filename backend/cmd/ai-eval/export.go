package main

import (
	"fmt"
	"slices"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Export (#421) expect key: export (csv | pdf | both: a button or toolbar
// runs an export action of that format). Cases: internal/ai/eval/export_cases.json.

// exportFormats lists the format of every export action on buttons / toolbars.
func exportFormats(a *dsl.App) []string {
	var out []string
	add := func(act *dsl.Action) {
		if act != nil && act.Type == dsl.ActionExport {
			out = append(out, act.Format)
		}
	}
	for _, s := range a.Screens {
		for i := range s.Toolbar {
			add(&s.Toolbar[i].Action)
		}
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				add(x.Action)
			}
		}
	}
	return out
}

func checkExport(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["export"].(string)
	if !ok {
		return nil
	}
	got := exportFormats(a)
	need := []string{want}
	if want == "both" {
		need = []string{"csv", "pdf"}
	}
	for _, f := range need {
		if !slices.Contains(got, f) {
			return []string{fmt.Sprintf("export=%v (want %s)", got, want)}
		}
	}
	return nil
}
