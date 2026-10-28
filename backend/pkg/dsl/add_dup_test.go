package dsl

import (
	"reflect"
	"testing"
)

// #416: the toolbar ＋ and a button opening the same form on one screen.
func TestLintDuplicateAddToolbar(t *testing.T) {
	addBtn := `{"type":"button","label":"記録を追加","icon":"plus","action":{"type":"navigate","screen":"form","presentation":"sheet"}},`
	wantLint(t, lintWith(t, "", addBtn), `screen "home" toolbar[0]: "追加" opens form "form" like a button on the same screen`)

	// a button inside a container counts too
	inSection := `{"type":"section","title":"s","children":[` + addBtn[:len(addBtn)-1] + `]},`
	wantLint(t, lintWith(t, "", inSection), `screen "home" toolbar[0]`)

	// only the toolbar (or only the button) is the rule, not a warning
	if got := lintWith(t, "", ""); len(got) > 0 {
		t.Fatalf("toolbar only: %v", got)
	}
}

func TestDuplicateAddToolbarScope(t *testing.T) {
	nav := func(screen, prefill string) *Action {
		return &Action{Type: ActionNavigate, Screen: screen, PrefillRef: prefill}
	}
	tb := func(a *Action) ToolbarButton { return ToolbarButton{Label: "x", Action: *a} }
	app := &App{Screens: []Screen{
		{ID: "home",
			Toolbar: []ToolbarButton{
				tb(nav("form", "")),                // 0: dup of the button
				tb(nav("settings", "")),            // 1: not a form
				tb(nav("form", "parent")),          // 2: other prefillRef
				tb(&Action{Type: ActionOpenVoice}), // 3: not navigate
				tb(nav("other_form", "")),          // 4: no button opens it
			},
			Components: []Component{
				{Type: CompButton, Action: nav("form", "")},
				{Type: CompButton, Action: nav("settings", "")},
				{Type: CompList, OnItemTap: nav("other_form", "")}, // editing a row is not "add"
			}},
		{ID: "settings"},
		{ID: "form", Entity: "log"},
		{ID: "other_form", Entity: "log"},
	}}
	if got := app.DuplicateAddToolbar(&app.Screens[0]); !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("got %v, want [0]", got)
	}
	if got := app.DuplicateAddToolbar(&app.Screens[1]); got != nil {
		t.Fatalf("screen without buttons: %v", got)
	}
}
