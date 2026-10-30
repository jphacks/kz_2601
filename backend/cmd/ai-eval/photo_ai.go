package main

import (
	"fmt"
	"slices"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Photo reading (#418) expect key: photoAnalyze ([kind, ...]: some photoField
// has an analyze entry of each kind, e.g. ["amount"] for a receipt).
// Cases: internal/ai/eval/photo_ai_cases.json (-cases).

// photoAnalyzeKinds lists the analyze kinds used by photoFields in the app.
func photoAnalyzeKinds(a *dsl.App) []string {
	var out []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type != dsl.CompPhotoField {
					continue
				}
				for _, an := range x.Analyze {
					if !slices.Contains(out, an.Kind) {
						out = append(out, an.Kind)
					}
				}
			}
		}
	}
	return out
}

func checkPhotoAI(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["photoAnalyze"].([]any)
	if !ok {
		return nil
	}
	got := photoAnalyzeKinds(a)
	for _, k := range want {
		if !slices.Contains(got, k.(string)) {
			return []string{fmt.Sprintf("photoAnalyze=%v", got)}
		}
	}
	return nil
}
