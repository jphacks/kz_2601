package dsl

import (
	"strings"
	"testing"
)

func outputsApp(comps ...Component) App {
	app := aiApp(comps...)
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "title", Name: "題名", Type: FieldString},
		Field{ID: "start", Name: "日時", Type: FieldDatetime},
		Field{ID: "day", Name: "日", Type: FieldDate},
		Field{ID: "rate", Name: "率", Type: FieldNumber},
		Field{ID: "done", Name: "済", Type: FieldBoolean},
		Field{ID: "pic", Name: "写真", Type: FieldPhoto},
	)
	return app
}

func fieldsButton(id string, items []string, outputs map[string]string) Component {
	return Component{Type: CompAiButton, ID: id, Task: "generate", Prompt: "{{body}} から抜き出す", Shape: "fields", Items: items, Outputs: outputs}
}

func TestAIOutputsValidPasses(t *testing.T) {
	app := outputsApp(
		fieldsButton("", []string{"題名", "日時", "場所"}, map[string]string{"題名": "title", "日時": "start", "場所": "summary"}),
		fieldsButton("b", []string{"点数", "講評", "気分", "日", "率"}, map[string]string{"点数": "score", "講評": "body", "気分": "mood", "日": "day", "率": "rate"}),
		fieldsButton("c", []string{"題名", "メモ"}, map[string]string{"題名": "title"}), // some items only show
	)
	app.Screens[0].Components[1].Prompt = "答えを採点"
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAIOutputsSemanticErrors(t *testing.T) {
	app := outputsApp(
		Component{Type: CompAiButton, Task: "generate", Prompt: "x", Shape: "list", Outputs: map[string]string{"a": "title"}},
		Component{Type: CompAiButton, ID: "s", Task: "summarize", Input: "body", Output: "summary", Outputs: map[string]string{"a": "title"}},
		Component{Type: CompAiButton, ID: "o", Task: "generate", Prompt: "x", Shape: "fields", Items: []string{"a"}, Output: "summary", Outputs: map[string]string{"a": "title"}},
		fieldsButton("e", []string{"a"}, map[string]string{}),
		fieldsButton("k", []string{"a"}, map[string]string{"zz": "title"}),
		fieldsButton("n", []string{"a"}, map[string]string{"a": "nope"}),
		fieldsButton("c", []string{"a"}, map[string]string{"a": "len"}),
		fieldsButton("b", []string{"a"}, map[string]string{"a": "done"}),
		fieldsButton("p", []string{"a"}, map[string]string{"a": "pic"}),
		fieldsButton("d", []string{"a", "b"}, map[string]string{"a": "title", "b": "title"}),
		Component{Type: CompAiButton, ID: "i", Task: "generate", Prompt: "x", Shape: "fields", Items: []string{"a"}, Input: "body", Outputs: map[string]string{"a": "body"}},
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, w := range []string{
		"outputs is only for task generate with shape fields",
		"outputs and output are exclusive",
		"outputs needs 1..10 items",
		`outputs key "zz" is not one of items`,
		`field "nope" not in entity "diary"`,
		`field "len" is computed`,
		`field "done" must be string, text, integer, number, date, datetime or enum (got boolean)`,
		`(got photo)`,
		`outputs "a" and "b" both fill field "title"`,
		`input and outputs field "body" must differ`,
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in\n%s", w, joined)
		}
	}
	if n := strings.Count(joined, "only for task generate with shape fields"); n != 2 {
		t.Errorf("list + summarize should both be rejected once, got %d:\n%s", n, joined)
	}
}

func TestAIOutputsHiddenAndAdminFields(t *testing.T) {
	app := outputsApp(
		fieldsButton("h", []string{"a"}, map[string]string{"a": "key"}),
		fieldsButton("m", []string{"a"}, map[string]string{"a": "memo"}),
	)
	app.Entities[0].Fields = append(app.Entities[0].Fields,
		Field{ID: "key", Name: "正解", Type: FieldString, Hidden: true},
		Field{ID: "memo", Name: "管理メモ", Type: FieldString, EditableBy: RoleAdmins},
	)
	err := app.Validate()
	if err == nil {
		t.Fatal("hidden / editableBy admins targets should be rejected")
	}
	for _, w := range []string{`field "key" is hidden`, `field "memo" is editableBy admins`} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("missing %q in\n%s", w, err)
		}
	}
}

func TestAIOutputsNeedForm(t *testing.T) {
	app := outputsApp(fieldsButton("", []string{"a"}, map[string]string{"a": "title"}))
	app.Screens[0].Entity = ""
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "outputs needs screen.entity") {
		t.Fatalf("got %v", err)
	}
}

func TestAIOutputsSchema(t *testing.T) {
	doc := func(comp string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"form",
"entities":[{"id":"ev","name":"予定","fields":[{"id":"title","name":"題名","type":"string"},{"id":"start","name":"日時","type":"datetime"}]}],
"screens":[{"id":"form","title":"h","entity":"ev","components":[` + comp + `]}]}`)
	}
	a, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"抜き出す","shape":"fields","items":["題名","日時"],"outputs":{"題名":"title","日時":"start"},"area":"A1:F6"}`))
	if err != nil {
		t.Fatalf("valid outputs: %v", err)
	}
	if o := a.Screens[0].Components[0].Outputs; o["題名"] != "title" || o["日時"] != "start" {
		t.Fatalf("decoded %+v", a.Screens[0].Components[0])
	}
	if _, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"x","shape":"fields","items":["題名"],"outputs":{"題名":1}}`)); err == nil {
		t.Error("non-string field id should fail the schema")
	}
	if _, err := Parse(doc(`{"type":"aiButton","task":"generate","prompt":"x","shape":"fields","items":["題名"],"outputs":{}}`)); err == nil {
		t.Error("empty outputs should fail the schema")
	}
}
