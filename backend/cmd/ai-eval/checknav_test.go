package main

import (
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func loadCheckNavApp(t *testing.T) *dsl.App {
	t.Helper()
	raw, err := os.ReadFile("testdata/checknav_small.json")
	if err != nil {
		t.Fatal(err)
	}
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// Each key checkNav reads gets a met and an unmet row. The app has tabs
// home (list of items, swipeToDelete) and people (stat, table), a sheet
// home -> item_form, a refs field and no unreachable screen.
func TestCheckNav(t *testing.T) {
	app := loadCheckNavApp(t)
	cases := []struct {
		name string
		exp  map[string]any
		mode string
		want string // joined misses; "" = met
	}{
		{"patch met", map[string]any{"patch": true}, "patch", ""},
		{"patch unmet", map[string]any{"patch": true}, "", "patch="},
		{"not patch met", map[string]any{"patch": false}, "", ""},
		{"not patch unmet", map[string]any{"patch": false}, "patch", "patch=patch"},
		{"unreachable met", map[string]any{"unreachable": float64(0)}, "", ""},
		{"unreachable unmet", map[string]any{"unreachable": float64(1)}, "", "unreachable=[]"},
		{"nav met", map[string]any{"nav": []any{"home>item_form"}}, "", ""},
		{"nav unmet", map[string]any{"nav": []any{"home>item_form", "people>item_form"}}, "", "nav!people>item_form"},
		{"tabs met", map[string]any{"tabs": float64(2)}, "", ""},
		{"tabs unmet", map[string]any{"tabs": float64(3)}, "", "tabs=2"},
		{"tabsMin met", map[string]any{"tabsMin": float64(2)}, "", ""},
		{"tabsMin unmet", map[string]any{"tabsMin": float64(3)}, "", "tabs=2"},
		{"tabOrder met", map[string]any{"tabOrder": "home,people"}, "", ""},
		{"tabOrder prefix met", map[string]any{"tabOrder": "home"}, "", ""},
		{"tabOrder unmet", map[string]any{"tabOrder": "people"}, "", "tabOrder=home,people"},
		{"tabOrder too long", map[string]any{"tabOrder": "home,people,item_form"}, "", "tabOrder=home,people"},
		{"tabHas met", map[string]any{"tabHas": "stat"}, "", ""},
		{"tabHas skips the first tab", map[string]any{"tabHas": "list"}, "", "tabHas!list"},
		{"refs met", map[string]any{"refs": true}, "", ""},
		{"refs unmet", map[string]any{"refs": false}, "", "refs=true"},
		{"deletable met", map[string]any{"deletable": []any{"items"}}, "", ""},
		{"deletable unmet", map[string]any{"deletable": []any{"items", "people"}}, "", "deletable!people"},
		{"has met", map[string]any{"has": []any{"list", "stat", "textField"}}, "", ""},
		{"has unmet", map[string]any{"has": []any{"list", "map"}}, "", "has!map"},
		{"lacks met", map[string]any{"lacks": []any{"map"}}, "", ""},
		{"lacks unmet", map[string]any{"lacks": []any{"map", "table"}}, "", "lacks!table"},
		{"other keys ignored", map[string]any{"scope": "public"}, "", ""},
		{"misses sorted", map[string]any{"tabs": float64(3), "has": []any{"map"}, "refs": false}, "", "has!map|refs=true|tabs=2"},
	}
	for _, c := range cases {
		got := strings.Join(checkNav(c.exp, app, c.mode), "|")
		if got != c.want {
			t.Errorf("%s: misses %q, want %q", c.name, got, c.want)
		}
	}
}

// deletable also accepts a deleteRecord button or toolbar item.
func TestCheckNavDeletableByAction(t *testing.T) {
	app := loadCheckNavApp(t)
	for i := range app.Screens {
		if app.Screens[i].ID != "people" {
			continue
		}
		app.Screens[i].Toolbar = append(app.Screens[i].Toolbar, dsl.ToolbarButton{
			Label:  "削除",
			Action: dsl.Action{Type: dsl.ActionDeleteRecord, Entity: "people"},
		})
	}
	if miss := checkNav(map[string]any{"deletable": []any{"people"}}, app, ""); len(miss) != 0 {
		t.Fatal(miss)
	}
}
