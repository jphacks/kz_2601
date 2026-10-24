package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every example (gallery included) keeps the mic seat empty: Claude copies them,
// and a scrolled-far-down part (A20:C21 on a 12-row-tall check) must not count.
func TestLintMicSeatExamplesClean(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(docsDir, "examples", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		app, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(f), err)
		}
		for _, w := range app.Lint() {
			if strings.Contains(w, "mic button") {
				t.Errorf("%s: %s", filepath.Base(f), w)
			}
		}
	}
}

// On a screen taller than 12 rows only its last 2 rows are the seat.
func TestLintMicSeatScrollingScreen(t *testing.T) {
	btn := func(area string) string {
		return `,{"type":"button","area":"` + area + `","label":"b","action":{"type":"navigate","screen":"form"}}`
	}
	if w := micSeatWarnings(t, btn("A11:C12")+btn("D20:F21")); len(w) != 0 {
		t.Errorf("A11:C12 is mid-screen when the screen ends at row 21: %v", w)
	}
	if w := micSeatWarnings(t, btn("A20:C21")); len(w) != 1 {
		t.Errorf("A20:C21 at the end of the screen: want 1, got %v", w)
	}
}

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
