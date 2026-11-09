package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// savedRecordDoc: entity "days" (title, on) with the form "day_form", listed
// on home by homeComp, plus a toolbar "new" button on home. formExtra is
// appended to the form's components.
func savedRecordDoc(homeComp, formExtra string) string {
	return `{"schemaVersion":1,"app":{"name":"t","voiceEntry":"floating"},"initialScreen":"home",
"entities":[
 {"id":"days","name":"候補日","fields":[{"id":"title","name":"名前","type":"string"},{"id":"on","name":"日付","type":"date"}]},
 {"id":"votes","name":"回答","fields":[{"id":"day","name":"候補日","type":"ref","refEntity":"days"}]}],
"screens":[
 {"id":"home","title":"t","toolbar":[{"label":"追加","icon":"plus","action":{"type":"navigate","screen":"day_form"}}],"components":[` + homeComp + `,
   {"type":"list","area":"A7:F10","entity":"votes","item":{"title":"day"},"onItemTap":{"type":"navigate","screen":"vote_form"}}]},
 {"id":"day_form","title":"f","entity":"days","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"days"}}],"components":[
   {"type":"textField","area":"A1:F2","field":"title"},{"type":"datePicker","area":"A3:F4","field":"on"}` + formExtra + `]},
 {"id":"vote_form","title":"v","entity":"votes","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"votes"}}],"components":[
   {"type":"picker","area":"A1:F2","field":"day"}]}]}`
}

const (
	daysTable    = `{"type":"table","area":"A1:F6","entity":"days","columns":[{"field":"title","title":"名前"},{"field":"on","title":"日付"}]}`
	daysListTap  = `{"type":"list","area":"A1:F6","entity":"days","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"day_form"}}`
	daysListBare = `{"type":"list","area":"A1:F6","entity":"days","item":{"title":"title"}}`
	daysCalendar = `{"type":"calendar","area":"A1:F6","entity":"days","dateField":"on","onDateTap":{"type":"navigate","screen":"day_form"}}`
	answerButton = `,{"type":"button","area":"A5:F6","label":"この日に回答","action":{"type":"navigate","screen":"vote_form","prefillRef":"day"}}`
	deleteButton = `,{"type":"button","area":"A5:F6","label":"削除","action":{"type":"deleteRecord","entity":"days"}}`
)

func savedRecordWarnings(t *testing.T, raw string) []string {
	t.Helper()
	var out []string
	for _, w := range lintApp(t, raw) {
		if strings.Contains(w, "open a saved") {
			out = append(out, w)
		}
	}
	return out
}

// schedule_poll (#755) / adaptive_study (#761): shown only in a table and
// opened only by the toolbar "new" button.
func TestLintSavedRecordTableOnly(t *testing.T) {
	for name, extra := range map[string]string{"plain": "", "prefillRef": answerButton, "delete": deleteButton} {
		w := savedRecordWarnings(t, savedRecordDoc(daysTable, extra))
		if len(w) != 1 || !strings.Contains(w[0], `entity "days" has a form screen "day_form"`) || !strings.Contains(w[0], "onItemTap") {
			t.Errorf("%s: want 1 warning for days, got %v", name, w)
		}
	}
}

func TestLintSavedRecordOpenable(t *testing.T) {
	for name, raw := range map[string]string{
		"list onItemTap": savedRecordDoc(daysListTap, answerButton),
		"calendar tap":   savedRecordDoc(daysCalendar, deleteButton),
		"table + list":   savedRecordDoc(strings.Replace(daysTable, "A1:F6", "A1:F3", 1)+","+strings.Replace(daysListTap, "A1:F6", "A4:F6", 1), ""),
		// a list without onItemTap is a deliberate append-only log (bmi)
		"append-only list": savedRecordDoc(daysListBare, ""),
	} {
		if w := savedRecordWarnings(t, raw); len(w) != 0 {
			t.Errorf("%s: want none, got %v", name, w)
		}
	}
}

// A form control that only works on a saved record means editing is meant,
// even when the rows are a list without onItemTap.
func TestLintSavedRecordListWithoutTapButEditOnlyForm(t *testing.T) {
	for name, extra := range map[string]string{"prefillRef": answerButton, "delete": deleteButton} {
		if w := savedRecordWarnings(t, savedRecordDoc(daysListBare, extra)); len(w) != 1 {
			t.Errorf("%s: want 1 warning, got %v", name, w)
		}
	}
}

// An entity without a form is not this lint's business (entityReach reports it).
func TestLintSavedRecordNoForm(t *testing.T) {
	raw := strings.Replace(savedRecordDoc(daysTable, ""), `"entity":"days","toolbar"`, `"toolbar"`, 1)
	raw = strings.Replace(raw, `{"type":"textField","area":"A1:F2","field":"title"},{"type":"datePicker","area":"A3:F4","field":"on"}`, `{"type":"text","area":"A1:F2","text":"x"}`, 1)
	if w := savedRecordWarnings(t, raw); len(w) != 0 {
		t.Errorf("want none, got %v", w)
	}
}

// Every example (and the few-shot quiz) keeps its saved records openable.
func TestLintSavedRecordExamplesClean(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(docsDir, "examples", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	few, _ := filepath.Glob("../../internal/ai/examples/*.json")
	for _, f := range append(files, few...) {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range savedRecordWarnings(t, string(raw)) {
			t.Errorf("%s: %s", filepath.Base(f), w)
		}
	}
}
