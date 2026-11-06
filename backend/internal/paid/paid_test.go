package paid

import (
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func app(entities []dsl.Entity, rules int, comps ...dsl.ComponentType) *dsl.App {
	a := &dsl.App{Entities: entities}
	for range rules {
		a.ServerRules = append(a.ServerRules, dsl.ServerRule{ID: "r"})
	}
	var cs []dsl.Component
	for _, c := range comps {
		cs = append(cs, dsl.Component{Type: c})
	}
	a.Screens = []dsl.Screen{{ID: "home", Components: []dsl.Component{{Type: "vstack", Children: cs}}}}
	return a
}

func ids(fs []Feature) string { return strings.Join(IDs(fs), ",") }

func TestDefaultList(t *testing.T) {
	var got []string
	for _, d := range Default.Defs() {
		if d.Title.JA == "" || d.Title.EN == "" || d.Reason.JA == "" || d.Reason.EN == "" {
			t.Errorf("%s: title / reason in ja and en", d.ID)
		}
		if d.SaveGated() && (d.WithoutInstruction.JA == "" || d.WithoutInstruction.EN == "") {
			t.Errorf("%s: a save-gated feature needs withoutInstruction", d.ID)
		}
		got = append(got, d.ID+":"+d.Gate)
	}
	if strings.Join(got, ",") != "sharedData:save,serverRules:save,onlineAI:use" {
		t.Fatalf("list: %v", got)
	}
}

func TestInAppAndAdded(t *testing.T) {
	private := app([]dsl.Entity{{ID: "e"}}, 0, dsl.CompAiButton)
	shared := app([]dsl.Entity{{ID: "e", Scope: dsl.ScopeShared}}, 0)
	rules := app([]dsl.Entity{{ID: "e"}}, 1)
	both := app([]dsl.Entity{{ID: "e", Scope: dsl.ScopePublic}}, 2)

	if got := ids(Default.InApp(private, "ja")); got != "" {
		t.Fatalf("aiButton is gated at use, not at save: %q", got)
	}
	if got := ids(Default.InApp(both, "ja")); got != "sharedData,serverRules" {
		t.Fatalf("both: %q", got)
	}
	if got := ids(Default.Added(shared, nil, "ja")); got != "sharedData" {
		t.Fatalf("new shared app: %q", got)
	}
	if got := ids(Default.Added(both, shared, "ja")); got != "serverRules" {
		t.Fatalf("rules added to a shared app: %q", got)
	}
	if got := ids(Default.Added(both, both, "en")); got != "" {
		t.Fatalf("an app keeps what it had: %q", got)
	}
	if got := ids(Default.Added(private, rules, "ja")); got != "" {
		t.Fatalf("removing is free: %q", got)
	}
	if d, ok := Default.ByID(OnlineAI); !ok || !d.Uses(private) || d.SaveGated() {
		t.Fatalf("onlineAI: %+v", d)
	}
	if f := Default.Added(rules, nil, "en"); f[0].Title != "Automatic server actions" {
		t.Fatalf("en: %+v", f)
	}
}

func TestParse(t *testing.T) {
	if _, err := Parse([]byte(`{"features":[{"id":"a","gate":"later"}]}`)); err == nil {
		t.Fatal("unknown gate")
	}
	if _, err := Parse([]byte(`{"features":[{"id":"a"},{"id":"a"}]}`)); err == nil {
		t.Fatal("duplicate id")
	}
	c, err := Parse([]byte(`{"features":[{"id":"a","enabled":false,"dslServerRules":true},{"id":"b","enabled":true}]}`))
	if err != nil || len(c.Defs()) != 1 || c.Defs()[0].ID != "b" || !c.Defs()[0].SaveGated() {
		t.Fatalf("%+v %v", c, err)
	}
}
