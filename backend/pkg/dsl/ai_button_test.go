package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aiApp is a one-screen form over a diary entity.
func aiApp(comps ...Component) App {
	return App{
		SchemaVersion: SchemaVersion,
		App:           AppMeta{Name: "日記", VoiceEntry: VoiceEntryNone},
		InitialScreen: "form",
		Entities: []Entity{{ID: "diary", Name: "日記", Fields: []Field{
			{ID: "body", Name: "本文", Type: FieldText},
			{ID: "summary", Name: "要約", Type: FieldString},
			{ID: "mood", Name: "気分", Type: FieldEnum, Options: []string{"良い", "普通", "悪い"}},
			{ID: "score", Name: "点数", Type: FieldInteger},
			{ID: "len", Name: "文字数", Type: FieldInteger, Computed: &Expr{Kind: ExprLiteral, Lit: float64(1)}},
		}}},
		Screens: []Screen{{ID: "form", Title: "書く", Entity: "diary", Components: comps}},
	}
}

func TestAIButtonValidPasses(t *testing.T) {
	app := aiApp(
		Component{Type: CompAiButton, Task: "summarize", Input: "body", Output: "summary", Prompt: "{{body}} を 30 字で"},
		Component{Type: CompAiButton, ID: "b", Task: "classify", Output: "mood"},
		Component{Type: CompAiButton, ID: "c", Task: "classify", Input: "body", Output: "summary", Options: []string{"仕事", "趣味"}},
		Component{Type: CompAiButton, ID: "d", Task: "suggest", Entity: "diary", Limit: 7, Output: "summary", Prompt: "{{input}}"},
		Component{Type: CompAiButton, ID: "e", Task: "classify", Output: "mood", Options: []string{"良い", "悪い"}},
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAIButtonSemanticErrors(t *testing.T) {
	app := aiApp(
		Component{Type: CompAiButton, Task: "translate", Output: "summary"},
		Component{Type: CompAiButton, Task: "summarize", Output: "nope"},
		Component{Type: CompAiButton, Task: "summarize", Output: "len"},
		Component{Type: CompAiButton, Task: "summarize", Output: "score"},
		Component{Type: CompAiButton, Task: "summarize", Output: "mood"},
		Component{Type: CompAiButton, Task: "classify", Output: "summary"},
		Component{Type: CompAiButton, Task: "classify", Output: "score", Options: []string{"a", "b"}},
		Component{Type: CompAiButton, Task: "classify", Output: "mood", Options: []string{"最高"}},
		Component{Type: CompAiButton, Task: "rewrite", Output: "summary", Options: []string{"a", "b"}},
		Component{Type: CompAiButton, Task: "rewrite", Input: "body", Entity: "diary", Output: "summary"},
		Component{Type: CompAiButton, Task: "rewrite", Input: "summary", Output: "summary"},
		Component{Type: CompAiButton, Task: "rewrite", Input: "ghost", Output: "summary"},
		Component{Type: CompAiButton, Task: "suggest", Entity: "ghost", Output: "summary"},
		Component{Type: CompAiButton, Task: "suggest", Limit: 3, Output: "summary"},
		Component{Type: CompAiButton, Task: "suggest", Output: "summary", Prompt: "{{title}} を見て"},
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	want := []string{
		`aiButton task "translate" must be one of`,
		`aiButton output "nope" not in entity "diary"`,
		`aiButton output "len" is computed`,
		`aiButton summarize output "score" must be string or text`,
		`aiButton summarize output "mood" must be string or text`,
		`aiButton classify needs options (2+) or an enum output field`,
		`aiButton classify output "score" must be enum, string or text`,
		`aiButton option "最高" is not an option of enum field "mood"`,
		`aiButton options are only for task classify`,
		`aiButton input and entity are exclusive`,
		`aiButton input and output must differ`,
		`aiButton input "ghost" not in entity "diary"`,
		`aiButton entity "ghost" does not exist`,
		`aiButton limit needs entity`,
		`aiButton prompt placeholder {{title}} is not a field`,
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

func TestAIButtonNeedsForm(t *testing.T) {
	app := aiApp(Component{Type: CompAiButton, Task: "summarize", Output: "summary"})
	app.Screens[0].Entity = ""
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "aiButton requires screen.entity") {
		t.Fatalf("want requires screen.entity, got %v", err)
	}
}

// The schema must reject an unknown task, a missing output and stray keys so
// Claude's structured output cannot emit them.
func TestAIButtonSchemaRejects(t *testing.T) {
	for name, comp := range map[string]string{
		"task enum":  `{"type":"aiButton","task":"translate","output":"s"}`,
		"no output":  `{"type":"aiButton","task":"summarize"}`,
		"no task":    `{"type":"aiButton","output":"s"}`,
		"extra key":  `{"type":"aiButton","task":"summarize","output":"s","model":"gpt"}`,
		"one option": `{"type":"aiButton","task":"classify","output":"s","options":["a"]}`,
	} {
		raw := []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home",
		"entities":[{"id":"e","name":"e","fields":[{"id":"s","name":"s","type":"string"}]}],
		"screens":[{"id":"home","title":"t","entity":"e","components":[` + comp + `]}]}`)
		if _, err := Parse(raw); err == nil {
			t.Errorf("%s: expected schema error", name)
		}
	}
}

// diary_ai.json is the canonical aiButton example (summarize + classify).
func TestDiaryAIExampleUsesAIButton(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "diary_ai.json"))
	if err != nil {
		t.Skip("example not found:", err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	tasks := map[string]bool{}
	for _, s := range app.Screens {
		for _, c := range s.Components {
			if c.Type == CompAiButton {
				tasks[c.Task] = true
			}
		}
	}
	for _, w := range []string{"summarize", "classify"} {
		if !tasks[w] {
			t.Errorf("diary_ai.json has no aiButton with task %q", w)
		}
	}
}
