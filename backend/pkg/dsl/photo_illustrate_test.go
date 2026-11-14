package dsl

import (
	"strings"
	"testing"
)

func illustrateDoc(fields, comp string) string {
	return `{"schemaVersion":1,"app":{"name":"アイコン","voiceEntry":"none"},"initialScreen":"home",
"entities":[{"id":"item","name":"項目","fields":[` + fields + `,{"id":"pic","name":"絵","type":"photo"}]}],
"screens":[
 {"id":"home","title":"一覧","components":[
  {"type":"list","entity":"item","item":{"title":"pic","image":"pic"},"onItemTap":{"type":"navigate","screen":"form"}},
  {"type":"button","label":"追加","action":{"type":"navigate","screen":"form"}}]},
 {"id":"form","title":"項目","entity":"item","components":[` + comp + `],
  "toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"item"}}]}]}`
}

func TestPhotoIllustrateValid(t *testing.T) {
	doc := illustrateDoc(`{"id":"name","name":"名前","type":"string","required":true}`,
		`{"type":"textField","field":"name"},{"type":"photoField","field":"pic","source":"library","illustrate":true}`)
	app, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !app.Screens[1].Components[1].Illustrate {
		t.Fatal("illustrate not decoded")
	}
	for _, p := range app.Lint() {
		if strings.Contains(p, "illustrate") {
			t.Errorf("unexpected lint: %s", p)
		}
	}
}

func TestPhotoIllustrateOnlyOnPhotoField(t *testing.T) {
	doc := illustrateDoc(`{"id":"name","name":"名前","type":"string"}`,
		`{"type":"textField","field":"name","illustrate":true}`)
	_, err := Parse([]byte(doc))
	if err == nil || !strings.Contains(err.Error(), "illustrate") {
		t.Fatalf("err = %v", err)
	}
	// Validate alone (no schema pass) also refuses it.
	var probs []string
	checkPhotoIllustrate("c", &Component{Type: CompTextField, Illustrate: true}, func(f string, a ...any) { probs = append(probs, f) })
	checkPhotoIllustrate("c", &Component{Type: CompPhotoField, Illustrate: true}, func(f string, a ...any) { probs = append(probs, f) })
	if len(probs) != 1 {
		t.Fatalf("problems = %v", probs)
	}
}

func TestPhotoIllustrateLintNeedsText(t *testing.T) {
	doc := illustrateDoc(`{"id":"n","name":"数","type":"integer"}`,
		`{"type":"numberField","field":"n"},{"type":"photoField","field":"pic","illustrate":true}`)
	app, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range app.Lint() {
		if strings.Contains(p, "photoField illustrate") && strings.Contains(p, `"item"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("lint = %v", app.Lint())
	}
}
