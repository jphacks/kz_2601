package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The quiz example (#262) is a valid, lint-clean app whose answer key is
// hidden and whose score is computed, not typed in.
func TestQuizExample_GradesAutomatically(t *testing.T) {
	raw, err := os.ReadFile("examples/quiz.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatalf("examples/quiz.json must validate: %v", err)
	}
	if p := app.Lint(); len(p) > 0 {
		t.Fatalf("examples/quiz.json must be lint-clean: %v", p)
	}
	if !app.EntityByID("questions").FieldByID("correct").Hidden {
		t.Fatal("answer key must be hidden")
	}
	if f := app.EntityByID("answers").FieldByID("score"); f == nil || f.Computed == nil {
		t.Fatal("score must be computed")
	}
	// no number input for the score on any form
	for _, s := range app.Screens {
		for _, c := range s.Components {
			for _, ch := range append([]dsl.Component{c}, c.Children...) {
				if s.Entity == "answers" && ch.Field == "score" {
					t.Fatalf("screen %s binds score to an input", s.ID)
				}
			}
		}
	}
}

func TestQuizRules_InBothPrompts(t *testing.T) {
	for _, s := range []string{"クイズ・テストの自動採点", "hidden:true", "点数を手入力させない"} {
		if !strings.Contains(systemPrompt, s) {
			t.Errorf("systemPrompt lacks %q", s)
		}
	}
	// the logic pass sees only the expr catalog: the recipe must be there
	for _, s := range []string{"クイズの自動採点", `{\"ref\":\"question.correct\"}`, "hidden"} {
		if !strings.Contains(logicSystemPrompt, strings.ReplaceAll(s, `\"`, `"`)) {
			t.Errorf("logicSystemPrompt lacks %q", s)
		}
	}
}
