package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Malformed DSL items must be reported, never panic lint/need/evaluate.
func TestMalformedRuns(t *testing.T) {
	cases := []struct {
		name string
		dsl  string
		want []string // substrings expected in lint output
	}{
		{"tabs as strings", `{"initialScreen":"home","app":{"tabs":["home","list"]},"screens":[{"id":"home"}]}`,
			[]string{"malformed: app.tabs[0] がオブジェクトでない(string)", "malformed: app.tabs[1]"}},
		{"tabs null element", `{"app":{"tabs":[null]}}`, []string{"app.tabs[0] がオブジェクトでない(null)"}},
		{"tabs numbers", `{"app":{"tabs":[1,2.5]}}`, []string{"app.tabs[0] がオブジェクトでない(number)", "app.tabs[1]"}},
		{"tabs not an array", `{"app":{"tabs":"home"}}`, nil},
		{"tabs null", `{"app":{"tabs":null}}`, nil},
		{"app not an object", `{"app":[1,2]}`, nil},
		{"tabs mixed", `{"initialScreen":"a","app":{"tabs":[{"screen":"b"},7]},"screens":[{"id":"a"},{"id":"b"}]}`,
			[]string{"app.tabs[1] がオブジェクトでない(number)"}},
		{"entities non-object", `{"entities":["x",null,3]}`,
			[]string{"dsl.entities[0] がオブジェクトでない(string)", "dsl.entities[1] がオブジェクトでない(null)", "dsl.entities[2] がオブジェクトでない(number)"}},
		{"fields non-object", `{"entities":[{"id":"e","fields":[true,[1]]}]}`,
			[]string{"entity e.fields[0] がオブジェクトでない(bool)", "entity e.fields[1] がオブジェクトでない(array)"}},
		{"screens non-object", `{"screens":["home",null]}`,
			[]string{"dsl.screens[0] がオブジェクトでない(string)", "dsl.screens[1] がオブジェクトでない(null)"}},
		{"components non-object", `{"initialScreen":"s","screens":[{"id":"s","components":["text",null,{"type":"stack","children":[1,"x",{"type":"toggle","field":"f"}]}]}]}`,
			[]string{"screen s.components[0]", "screen s.components[1]", "screen s stack.children[0] がオブジェクトでない(number)", "screen s stack.children[1] がオブジェクトでない(string)"}},
		{"toolbar non-object", `{"initialScreen":"s","screens":[{"id":"s","toolbar":["add",null]}]}`,
			[]string{"screen s.toolbar[0]", "screen s.toolbar[1]"}},
		{"nested action not object", `{"initialScreen":"s","screens":[{"id":"s","components":[{"type":"button","action":"go"},{"type":"list","onItemTap":[1]}]}]}`, nil},
		{"filter / item non-object", `{"screens":[{"id":"s","components":[{"type":"stat","label":"今月","filter":"x","value":[null]},{"type":"list","item":3,"sort":"a"}]}]}`, nil},
		{"null dsl", `null`, []string{"malformed: dsl がオブジェクトでない(null)"}},
		{"children of untyped container", `{"screens":[{"id":"s","components":[{"type":"text"},{"children":[null]}]}]}`,
			[]string{"screen s.components[1].children[0] がオブジェクトでない(null)"}},
		{"empty object", `{}`, nil},
	}
	needs := needNames(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := load([]byte(tc.dsl))
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(lint(a), "\n")
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("lint missing %q in:\n%s", w, got)
				}
			}
			if len(tc.want) == 0 && strings.Contains(got, "malformed:") {
				t.Errorf("unexpected malformed report:\n%s", got)
			}
			for _, n := range needs {
				need(a, n)
			}
		})
	}
}

func TestMalformedTabsNotCounted(t *testing.T) {
	a, err := load([]byte(`{"app":{"tabs":["a","b",{"screen":"c"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := need(a, "tabs3"); m == "○" {
		t.Error("string tabs must not count toward tabs3")
	}
}

// evaluate runs end to end over a run file whose cases include malformed DSLs.
func TestEvaluateMalformedRun(t *testing.T) {
	run := `{"model":"m","cases":[
		{"id":"c1","ok":true,"dsl":{"app":{"tabs":["home",null,3]}}},
		{"id":"c2","ok":true,"dsl":[1,2]},
		{"id":"c3","ok":true},
		{"id":"c4","ok":true,"dsl":{"screens":[{"id":"s","components":[{"type":"stack","children":[null]}]}]}},
		{"id":"c5","ok":true,"dsl":null}]}`
	p := filepath.Join(t.TempDir(), "run.json")
	if err := os.WriteFile(p, []byte(run), 0o600); err != nil {
		t.Fatal(err)
	}
	res, _ := evaluate([]auditCase{{ID: "c1", Needs: []string{"tabs3", "agg"}}}, p)
	if r := res["c1"]; !r.ok || len(r.lint) == 0 || !strings.HasPrefix(r.lint[0], "malformed:") {
		t.Errorf("c1: %+v", r)
	}
	if m := res["c1"].marks["tabs3"]; m[0] != "×" {
		t.Errorf("c1 tabs3: %v", m)
	}
	for _, id := range []string{"c2", "c3"} {
		if r := res[id]; r.ok || r.err == "" {
			t.Errorf("%s should fail to load: %+v", id, r)
		}
	}
	if r := res["c5"]; !r.ok || !strings.Contains(strings.Join(r.lint, "\n"), "malformed: dsl がオブジェクトでない(null)") {
		t.Errorf("c5 (dsl null) should be reported, not loaded as an empty app: %+v", r)
	}
	if r := res["c4"]; !strings.Contains(strings.Join(r.lint, "\n"), "children[0] がオブジェクトでない(null)") {
		t.Errorf("c4: %v", r.lint)
	}
}
