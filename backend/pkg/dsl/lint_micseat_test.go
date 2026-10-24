package dsl

import (
	"strings"
	"testing"
)

func micSeatDoc(homeComps string) string {
	return `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",
"entities":[{"id":"log","name":"記録","fields":[{"id":"title","name":"内容","type":"string"}]}],
"screens":[
 {"id":"home","title":"t","components":[
   {"type":"list","area":"A1:F10","entity":"log","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form"}}` + homeComps + `]},
 {"id":"form","title":"f","entity":"log","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"log"}}],"components":[
   {"type":"textField","area":"A1:F2","field":"title"},
   {"type":"button","area":"A11:F12","label":"削除","action":{"type":"deleteRecord","entity":"log"}}]}]}`
}

func micSeatWarnings(t *testing.T, homeComps string) []string {
	t.Helper()
	var out []string
	for _, w := range lintApp(t, micSeatDoc(homeComps)) {
		if strings.Contains(w, "mic button") {
			out = append(out, w)
		}
	}
	return out
}

// #140: parts anchored at the bottom-left push the mic FAB onto list rows.
func TestLintMicSeat(t *testing.T) {
	add := func(area string) string {
		return `,{"type":"button","area":"` + area + `","label":"追加","action":{"type":"navigate","screen":"form"}}`
	}
	for _, area := range []string{"A11:F12", "A11:C12", "B12:C12"} {
		if w := micSeatWarnings(t, add(area)); len(w) != 1 {
			t.Errorf("%s: want 1 mic-seat warning, got %v", area, w)
		}
	}
	// bottom-right button, and the form's full-width delete button, are fine
	if w := micSeatWarnings(t, add("D11:F12")); len(w) != 0 {
		t.Errorf("D11:F12: want none, got %v", w)
	}
}

// A tall list reaching the bottom is the background the FAB floats over, not a seat conflict.
func TestLintMicSeatIgnoresTallParts(t *testing.T) {
	raw := strings.Replace(micSeatDoc(""), `"area":"A1:F10"`, `"area":"A1:F12"`, 1)
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "mic button") {
			t.Fatalf("unexpected: %s", w)
		}
	}
}
