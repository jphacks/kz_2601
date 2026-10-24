package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #164: the demo pass produced a 割り勘 with no way to add an expense (the
// list only edits). A toolbar button to the form is added; the summary names it.
func TestAddMissingAddButtons(t *testing.T) {
	raw, err := os.ReadFile("testdata/warikan_no_add_button.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	fixed := addMissingAddButtons(raw, app, LocaleJA)
	got, err := dsl.Parse(fixed)
	if err != nil {
		t.Fatalf("fixed DSL must validate: %v", err)
	}
	home := &got.Screens[0]
	if home.ID != "home" || len(home.Toolbar) != 2 || home.Toolbar[1].Action.Screen != "expense_form" || home.Toolbar[1].Label != "支出を追加" {
		t.Fatalf("toolbar: %+v", home)
	}
	if s := summarize(got); !strings.Contains(s, "右上の「メンバー追加」からメンバーを、右上の「支出を追加」から支出を追加できます。") {
		t.Errorf("summary: %q", s)
	}
	// already complete: unchanged
	ok, _ := dsl.Parse([]byte(fewShotWarikan))
	if out := addMissingAddButtons([]byte(fewShotWarikan), ok, LocaleJA); string(out) != fewShotWarikan {
		t.Error("an app with add buttons must stay as is")
	}
	en, _ := dsl.Parse([]byte(fewShotExpenseEN))
	if out := addMissingAddButtons([]byte(fewShotExpenseEN), en, LocaleEN); string(out) != fewShotExpenseEN {
		t.Error("expense_en must stay as is")
	}
}
