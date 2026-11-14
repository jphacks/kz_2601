package dsl

import (
	"strings"
	"testing"
)

func photoInputApp(comps ...Component) App {
	app := longApp(comps...)
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "rx", Name: "処方箋", Type: FieldPhoto})
	return app
}

// aiButton inputPhoto: a photo field of screen.entity, any task.
func TestAIInputPhotoValidPasses(t *testing.T) {
	app := photoInputApp(
		Component{Type: CompAiButton, Task: "extract", InputPhoto: "rx", Output: "letter", Prompt: "薬の名前と飲み方"},
		Component{Type: CompAiButton, ID: "b", Task: "summarize", Input: "body", InputPhoto: "rx", Output: "summary"},
		Component{Type: CompAiButton, ID: "c", Task: "generate", Prompt: "写真の料理の感想", InputPhoto: "rx", Output: "letter", Length: "long"},
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAIInputPhotoSemanticErrors(t *testing.T) {
	app := photoInputApp(
		Component{Type: CompAiButton, Task: "extract", InputPhoto: "nope", Output: "letter"},
		Component{Type: CompAiButton, Task: "extract", InputPhoto: "body", Output: "letter"},
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{`inputPhoto "nope" not in entity`, `inputPhoto "body" must be a photo field`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

// Without screen.entity (a generate button on a home screen) there is no draft photo.
func TestAIInputPhotoNeedsForm(t *testing.T) {
	var problems []string
	add := func(f string, a ...any) { problems = append(problems, f) }
	checkAIInputPhoto("home", &Component{Type: CompAiButton, Task: "generate", InputPhoto: "rx"}, nil, add)
	if len(problems) != 1 || !strings.Contains(problems[0], "needs screen.entity") {
		t.Fatalf("%v", problems)
	}
}
