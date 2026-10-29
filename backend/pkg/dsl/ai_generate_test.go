package dsl

import (
	"strings"
	"testing"
)

// #417: aiButton task generate works on any screen; output is optional.
func TestAIGenerateValidPasses(t *testing.T) {
	app := aiApp(
		Component{Type: CompAiButton, Task: "generate", Prompt: "励ましの一言"},
		Component{Type: CompAiButton, ID: "b", Task: "generate", Shape: "list", Count: 5, Prompt: "罰ゲームの案"},
		Component{Type: CompAiButton, ID: "c", Task: "generate", Shape: "fields", Items: []string{"主菜", "副菜", "汁物"},
			Entity: "diary", Limit: 20, Prompt: "{{input}} から今日の献立"},
		Component{Type: CompAiButton, ID: "d", Task: "generate", Output: "summary", Input: "body", Prompt: "{{body}} への返事"},
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
	// no screen.entity: shown under the button only
	app = aiApp(Component{Type: CompAiButton, Task: "generate", Shape: "list", Entity: "diary", Prompt: "お題"})
	app.Screens[0].Entity = ""
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAIGenerateSemanticErrors(t *testing.T) {
	long := strings.Repeat("あ", MaxAIInstructionRunes+1)
	app := aiApp(
		Component{Type: CompAiButton, Task: "generate"},
		Component{Type: CompAiButton, Task: "generate", Prompt: long},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Shape: "table"},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Shape: "list", Count: 11},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Count: 3},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Shape: "fields"},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Shape: "fields", Items: []string{"主菜", "主菜"}},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Items: []string{"a"}},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Entity: "diary", Limit: 21},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Output: "score"},
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Options: []string{"a", "b"}},
		Component{Type: CompAiButton, Task: "summarize", Output: "summary", Shape: "list"},
		Component{Type: CompAiButton, Task: "summarize"},
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	want := []string{
		"aiButton generate needs prompt",
		"aiButton generate prompt must be at most 500 characters (got 501)",
		`aiButton shape "table" must be line|list|fields`,
		"aiButton count must be 1..10 and only with shape list", // count 11
		"aiButton count must be 1..10 and only with shape list", // count on line
		"aiButton shape fields needs 1..10 items",
		`aiButton items must be unique non-empty names of at most 20 characters ("主菜")`,
		"aiButton items are only for shape fields",
		"aiButton generate limit must be at most 20",
		`aiButton generate output "score" must be string or text`,
		"aiButton options are only for task classify",
		"aiButton shape / count / items are only for task generate",
		"aiButton summarize needs output",
	}
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("problems missing %q:\n%s", w, joined)
		}
	}
	if len(ve.Problems) != len(want) {
		t.Errorf("expected %d problems, got %d:\n%s", len(want), len(ve.Problems), joined)
	}
}

func TestAIGenerateWithoutFormRejectsFormKeys(t *testing.T) {
	app := aiApp(
		Component{Type: CompAiButton, Task: "generate", Prompt: "{{body}} に一言", Output: "summary"},
	)
	app.Screens[0].Entity = ""
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "output / input need screen.entity") ||
		!strings.Contains(err.Error(), "placeholder {{body}} needs screen.entity") {
		t.Fatalf("got %v", err)
	}
}

// The schema bounds count / items so structured output cannot exceed them.
func TestAIGenerateSchemaRejects(t *testing.T) {
	for name, comp := range map[string]string{
		"shape enum": `{"type":"aiButton","task":"generate","prompt":"x","shape":"table"}`,
		"count 0":    `{"type":"aiButton","task":"generate","prompt":"x","shape":"list","count":0}`,
		"count 11":   `{"type":"aiButton","task":"generate","prompt":"x","shape":"list","count":11}`,
		"items 11":   `{"type":"aiButton","task":"generate","prompt":"x","shape":"fields","items":["1","2","3","4","5","6","7","8","9","10","11"]}`,
	} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home",
		"entities":[{"id":"e","name":"e","fields":[{"id":"s","name":"s","type":"string"}]}],
		"screens":[{"id":"home","title":"t","components":[` + comp + `]}]}`)
		if err := ValidateSchema(raw); err == nil {
			t.Errorf("%s: expected schema error", name)
		}
	}
	ok := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home",
		"entities":[{"id":"e","name":"e","fields":[{"id":"s","name":"s","type":"string"}]}],
		"screens":[{"id":"home","title":"t","components":[{"type":"aiButton","task":"generate","prompt":"罰ゲームの案","shape":"list","count":5}]}]}`)
	if _, err := Parse(ok); err != nil {
		t.Fatalf("generate without output: %v", err)
	}
}
