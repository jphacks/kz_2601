package dsl

import (
	"strings"
	"testing"
)

func longApp(comps ...Component) App {
	app := aiApp(comps...)
	app.Entities[0].Fields = append(app.Entities[0].Fields, Field{ID: "letter", Name: "手紙", Type: FieldText})
	return app
}

// aiButton length long: generate line and the transforms except classify.
func TestAILengthValidPasses(t *testing.T) {
	app := longApp(
		Component{Type: CompAiButton, Task: "generate", Prompt: "祖母へのお礼の手紙", Length: "long"},
		Component{Type: CompAiButton, ID: "b", Task: "generate", Shape: "line", Prompt: "{{body}} から日記", Output: "letter", Length: "long"},
		Component{Type: CompAiButton, ID: "c", Task: "summarize", Input: "body", Output: "letter", Length: "long"},
		Component{Type: CompAiButton, ID: "d", Task: "extract", Input: "body", Output: "letter", Length: "long"},
		Component{Type: CompAiButton, ID: "e", Task: "summarize", Input: "body", Output: "summary", Length: "short"},
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAILengthSemanticErrors(t *testing.T) {
	app := longApp(
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Length: "huge"},
		Component{Type: CompAiButton, Task: "classify", Output: "mood", Length: "long"},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Shape: "list", Length: "long"},
		Component{Type: CompAiButton, Task: "summarize", Input: "body", Output: "summary", Length: "long"},
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, w := range []string{
		`aiButton length "huge" must be short|long`,
		"aiButton length long is not for task classify",
		"aiButton length long needs shape line",
		`aiButton length long output "summary" must be a text field (got string)`,
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in\n%s", w, joined)
		}
	}
}

// The schema knows length (enum short|long).
func TestAILengthSchema(t *testing.T) {
	doc := func(comp string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
"entities":[],"screens":[{"id":"home","title":"h","components":[` + comp + `]}]}`)
	}
	a, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"手紙","length":"long","area":"A1:F8"}`))
	if err != nil {
		t.Fatalf("valid long: %v", err)
	}
	if a.Screens[0].Components[0].Length != "long" {
		t.Fatalf("decoded %+v", a.Screens[0].Components[0])
	}
	if _, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"x","length":"medium"}`)); err == nil {
		t.Error("length medium should fail the schema")
	}
}
