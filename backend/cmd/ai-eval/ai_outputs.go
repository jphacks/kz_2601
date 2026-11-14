package main

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// aiOutputs (bool): some generate fields aiButton fills form fields per item
// (outputs; dsl/ai_outputs.go). Cases: internal/ai/eval/ai_outputs_cases.json.
func checkAIOutputs(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["aiOutputs"].(bool)
	if !ok {
		return nil
	}
	got := false
	for _, b := range generateButtons(a) {
		got = got || (len(b.Outputs) > 0 && b.Shape == "fields")
	}
	switch {
	case got && !want:
		return []string{"aiOutputs=false but a generate fields button has outputs"}
	case !got && want:
		return []string{"aiOutputs=true but no generate fields button has outputs"}
	}
	return nil
}
