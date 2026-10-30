package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// aiButton generate (#417) expect keys: aiGenerate (string: the shape of some
// generate button, or any) and aiGenerateEntity (bool: one of them reads
// records). Cases: internal/ai/eval/ai_generate_cases.json (-cases).

// generateButtons lists the generate aiButtons of the app.
func generateButtons(a *dsl.App) []dsl.Component {
	var out []dsl.Component
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type == dsl.CompAiButton && x.Task == "generate" {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

func checkAIGenerate(exp map[string]any, a *dsl.App) []string {
	var miss []string
	btns := generateButtons(a)
	if want, ok := exp["aiGenerate"].(string); ok {
		found := false
		for _, b := range btns {
			shape := b.Shape
			if shape == "" {
				shape = "line"
			}
			found = found || want == "any" || shape == want
		}
		if !found {
			shapes := make([]string, 0, len(btns))
			for _, b := range btns {
				shapes = append(shapes, b.Shape)
			}
			miss = append(miss, fmt.Sprintf("aiGenerate=%s got %v", want, shapes))
		}
	}
	if want, ok := exp["aiGenerateEntity"].(bool); ok {
		got := false
		for _, b := range btns {
			got = got || b.Entity != ""
		}
		if got != want {
			miss = append(miss, fmt.Sprintf("aiGenerateEntity=%v", got))
		}
	}
	return miss
}
