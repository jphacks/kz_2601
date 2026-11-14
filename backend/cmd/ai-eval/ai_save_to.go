package main

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// aiSaveTo (bool): some generate aiButton saves its list items as records
// (saveTo; dsl/ai_save_to.go). Cases: internal/ai/eval/ai_save_to_cases.json.
func checkAISaveTo(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["aiSaveTo"].(bool)
	if !ok {
		return nil
	}
	got := false
	for _, b := range generateButtons(a) {
		got = got || (b.SaveTo != nil && b.Shape == "list")
	}
	switch {
	case got && !want:
		return []string{"aiSaveTo=false but a generate list button has saveTo"}
	case !got && want:
		return []string{"aiSaveTo=true but no generate list button has saveTo"}
	}
	return nil
}
