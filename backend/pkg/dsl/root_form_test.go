package dsl

import (
	"strings"
	"testing"
)

// rootFormDoc: a memo form and a list; %s = app extra (tabs), form save action
// JSON, entity extra.
const rootFormDoc = `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"%s},"initialScreen":"form",
"entities":[{"id":"memo","name":"メモ"%s,"fields":[{"id":"body","name":"本文","type":"text"}]}],
"screens":[
 {"id":"form","title":"メモ","entity":"memo","toolbar":[{"label":"保存","action":%s}],
  "components":[{"type":"textField","field":"body"}]},
 {"id":"list","title":"一覧","toolbar":[{"label":"追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}],
  "components":[{"type":"list","entity":"memo","item":{"title":"body"},"onItemTap":{"type":"navigate","screen":"form"}}]}]}`

const formListTabs = `,"tabs":[{"screen":"form","title":"入力"},{"screen":"list","title":"一覧"}]`

func rootFormWarnings(t *testing.T, tabs, entityExtra, save string) []string {
	t.Helper()
	parts := strings.SplitN(rootFormDoc, "%s", 4)
	raw := parts[0] + tabs + parts[1] + entityExtra + parts[2] + save + parts[3]
	var out []string
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "shown as a tab/home screen") {
			out = append(out, w)
		}
	}
	return out
}

func TestRootFormSaveWithoutScreen(t *testing.T) {
	bare := `{"type":"saveRecord","entity":"memo"}`
	w := rootFormWarnings(t, formListTabs, "", bare)
	if len(w) != 1 || !strings.Contains(w[0], `screen "form" is a form (entity "memo")`) ||
		!strings.Contains(w[0], "toolbar[0]") || !strings.Contains(w[0], `list screen "list"`) {
		t.Fatalf("tab form: %v", w)
	}
	// home (no tabs) is a root too
	if w := rootFormWarnings(t, "", "", bare); len(w) != 1 {
		t.Fatalf("home form: %v", w)
	}
}

func TestRootFormSaveOK(t *testing.T) {
	// a then-screen leaves the form
	if w := rootFormWarnings(t, formListTabs, "", `{"type":"saveRecord","entity":"memo","screen":"list"}`); len(w) > 0 {
		t.Fatalf("then-screen: unexpected %v", w)
	}
	// settings (single) are edited in place
	single := strings.Replace(rootFormDoc, `{"type":"list","entity":"memo","item":{"title":"body"},"onItemTap":{"type":"navigate","screen":"form"}}`,
		`{"type":"text","text":"x"}`, 1)
	parts := strings.SplitN(single, "%s", 4)
	for _, w := range lintApp(t, parts[0]+formListTabs+parts[1]+`,"single":true`+parts[2]+`{"type":"saveRecord","entity":"memo"}`+parts[3]) {
		if strings.Contains(w, "shown as a tab/home screen") {
			t.Fatalf("single: unexpected %s", w)
		}
	}
	// the usual shape: list tab opens the form, save closes it
	tabs := `,"tabs":[{"screen":"list","title":"一覧"},{"screen":"list2","title":"x"}]`
	parts = strings.SplitN(rootFormDoc, "%s", 4)
	raw := parts[0] + tabs + parts[1] + parts[2] + `{"type":"saveRecord","entity":"memo"}` + parts[3]
	raw = strings.Replace(raw, `"initialScreen":"form"`, `"initialScreen":"list"`, 1)
	raw = strings.TrimSuffix(raw, "]}") + `,{"id":"list2","title":"x","components":[{"type":"text","text":"x"}]}]}`
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "shown as a tab/home screen") {
			t.Fatalf("pushed form: unexpected %s", w)
		}
	}
}

// No list of the entity: the hint says to add one.
func TestRootFormSaveNoList(t *testing.T) {
	raw := strings.Replace(rootFormDoc, `{"type":"list","entity":"memo","item":{"title":"body"},"onItemTap":{"type":"navigate","screen":"form"}}`,
		`{"type":"text","text":"x"}`, 1)
	parts := strings.SplitN(raw, "%s", 4)
	raw = parts[0] + formListTabs + parts[1] + parts[2] + `{"type":"saveRecord","entity":"memo"}` + parts[3]
	n := 0
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "shown as a tab/home screen") {
			n++
			if !strings.Contains(w, `add a list screen of "memo"`) {
				t.Fatalf("hint: %s", w)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want 1, got %d", n)
	}
}
