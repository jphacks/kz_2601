package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #221: the ai-eval seeds for "add 2 fields" (19 fits, 39 hits the cap) must
// be valid and really have that many fields.
func TestCheckupSeedsAreValid(t *testing.T) {
	for name, want := range map[string]int{"checkup19": 19, "checkup39": 39} {
		raw, err := os.ReadFile(filepath.Join("examples", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		app, err := dsl.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := len(app.EntityByID("checkup").Fields); n != want {
			t.Fatalf("%s fields = %d, want %d", name, n, want)
		}
	}
}

// #221: edit / chat prompts carry the new cap and the no-silent-delete rule.
func TestFieldCapPromptRules(t *testing.T) {
	for name, s := range map[string]string{"system": systemPrompt, "edit": editRules, "chatEdit": chatEditSystemPrompt} {
		if strings.Contains(s, "field 20") || strings.Contains(s, "項目は 20") {
			t.Errorf("%s prompt still says 20 fields", name)
		}
	}
	if !strings.Contains(editRules, "#221") || !strings.Contains(chatEditSystemPrompt, "#221") {
		t.Error("edit / chat edit prompts must carry the #221 no-silent-delete rule")
	}
	app := dsl.Initial()
	app.Entities = []dsl.Entity{{ID: "memo", Name: "メモ", Fields: []dsl.Field{{ID: "t", Name: "本文", Type: dsl.FieldString}}}}
	if s := summarizeApp(&app); !strings.Contains(s, "項目 1/40") {
		t.Errorf("summary must show the field count against the cap:\n%s", s)
	}
}
