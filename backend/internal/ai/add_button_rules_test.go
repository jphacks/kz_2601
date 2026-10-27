package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #416: a real output (お小遣い帳) had the toolbar ＋ and the bottom-right
// 「記録を追加」 both opening income_form. The toolbar copy goes, the button stays.
func TestDropDuplicateAddToolbar(t *testing.T) {
	raw, err := os.ReadFile("examples/okozukai.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !hasLint(app.Lint(), "like a button on the same screen") {
		t.Fatal("fixture must start with the duplicate (lint)")
	}
	fixed := dropDuplicateAddToolbar(raw, app)
	got, err := dsl.Parse(fixed)
	if err != nil {
		t.Fatalf("fixed DSL must validate: %v", err)
	}
	if hasLint(got.Lint(), "like a button on the same screen") {
		t.Errorf("duplicate left: %v", got.Lint())
	}
	home := screenOf(got, "home")
	for _, tb := range home.Toolbar {
		if tb.Action.Screen == "income_form" {
			t.Errorf("toolbar still opens income_form: %+v", home.Toolbar)
		}
	}
	buttons := 0
	for _, c := range home.Components {
		if c.Type == dsl.CompButton && c.Action != nil && c.Action.Screen == "income_form" {
			buttons++
		}
	}
	if buttons != 1 {
		t.Errorf("the add button must stay: %d", buttons)
	}

	// few-shots no longer show both
	for name, src := range map[string]string{"kakeibo": fewShotKakeibo, "expense_en": fewShotExpenseEN, "warikan": fewShotWarikan} {
		a, err := dsl.Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out := dropDuplicateAddToolbar([]byte(src), a); string(out) != src {
			t.Errorf("%s must stay as is", name)
		}
	}
}

func TestAddButtonRulesReachPrompt(t *testing.T) {
	if !strings.Contains(systemPrompt, "## 追加ボタンは 1 画面に 1 つ(#416)") {
		t.Error("systemPrompt lacks the add button rules")
	}
}

func hasLint(warns []string, sub string) bool {
	for _, w := range warns {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func screenOf(a *dsl.App, id string) *dsl.Screen {
	for i := range a.Screens {
		if a.Screens[i].ID == id {
			return &a.Screens[i]
		}
	}
	return nil
}
