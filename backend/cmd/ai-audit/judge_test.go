package main

import (
	"os"
	"testing"
)

func loadFile(t *testing.T, path string) *app {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := load(raw)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestIsType(t *testing.T) {
	cases := []struct {
		c     obj
		types []string
		want  bool
	}{
		{obj{"type": "stat"}, []string{"stat"}, true},
		{obj{"type": "chart"}, []string{"stat", "chart"}, true},
		{obj{"type": "list"}, []string{"stat", "chart"}, false},
		{obj{}, []string{"stat"}, false},
		{obj{"type": "stat"}, nil, false},
	}
	for _, c := range cases {
		if got := isType(c.types...)(c.c); got != c.want {
			t.Errorf("isType(%v)(%v) = %v, want %v", c.types, c.c, got, c.want)
		}
	}
	a := loadFile(t, "testdata/small.json")
	if !a.has(isType("toggle")) {
		t.Error("a container child must be visible to has()")
	}
}

func TestHasComputed(t *testing.T) {
	cases := []struct {
		name string
		dsl  string
		want bool
	}{
		{"empty", `{}`, false},
		{"plain field", `{"entities":[{"id":"e","fields":[{"id":"f","type":"text"}]}]}`, false},
		{"computed field", `{"entities":[{"id":"e","fields":[{"id":"f","computed":{"op":"toNumber"}}]}]}`, true},
		{"stat with value", `{"screens":[{"id":"s","components":[{"type":"stat","value":{"agg":"count"}}]}]}`, true},
		{"stat without value", `{"screens":[{"id":"s","components":[{"type":"stat"}]}]}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := load([]byte(c.dsl))
			if err != nil {
				t.Fatal(err)
			}
			if got := a.hasComputed(); got != c.want {
				t.Fatalf("hasComputed = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAnyFilterOp(t *testing.T) {
	a := loadFile(t, "testdata/small.json")
	cases := []struct {
		op   string
		want bool
	}{
		{"eq", true},        // inside the computed agg
		{"thisMonth", true}, // inside the stat value
		{"thisWeek", false},
	}
	for _, c := range cases {
		if got := anyFilterOp(a.raw, c.op); got != c.want {
			t.Errorf("anyFilterOp(%q) = %v, want %v", c.op, got, c.want)
		}
	}
	if anyFilterOp(nil, "eq") || anyFilterOp("eq", "eq") {
		t.Error("non-object values have no filter")
	}
}

func TestHasComputedAgg(t *testing.T) {
	if !hasComputedAgg(loadFile(t, "testdata/small.json")) {
		t.Error("small.json has a computed agg")
	}
	a, err := load([]byte(`{"entities":[{"id":"e","fields":[{"id":"f","computed":{"op":"toNumber"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if hasComputedAgg(a) {
		t.Error("a computed without agg is not a computed agg")
	}
}

func TestNeedOnSmall(t *testing.T) {
	a := loadFile(t, "testdata/small.json")
	cases := map[string]string{
		"agg":          "○",
		"filter_month": "○",
		"filter_today": "×",
		"toggle":       "△", // form toggle only, no list.item.toggle
		"tabs3":        "×",
		"no_such_need": "?",
	}
	for n, want := range cases {
		if got, why := need(a, n); got != want {
			t.Errorf("need(%s) = %s (%s), want %s", n, got, why, want)
		}
	}
}

func TestEvaluateAndScore(t *testing.T) {
	cases := []auditCase{{ID: "small", Needs: []string{"agg", "filter_month", "toggle", "tabs3", "filter_today"}}}
	res, rf := evaluate(cases, "testdata/run.json")
	if rf.Model != "test-model" || len(rf.Cases) != 3 {
		t.Fatalf("run file = %+v", rf)
	}

	small := res["small"]
	if !small.ok || len(small.marks) != 5 {
		t.Fatalf("small = %+v", small)
	}
	if ok, part, ng := score(small); ok != 2 || part != 1 || ng != 2 {
		t.Fatalf("score(small) = %d/%d/%d, want 2/1/2", ok, part, ng)
	}

	if r := res["gen_failed"]; r.ok || r.err != "timeout" || len(r.marks) != 0 {
		t.Fatalf("gen_failed = %+v", r)
	}
	if r := res["bad_dsl"]; r.ok || r.err == "" {
		t.Fatalf("an undecodable DSL must turn into a failure: %+v", r)
	}

	if ok, part, ng := score(result{}); ok+part+ng != 0 {
		t.Fatal("no marks scores zero")
	}
	// anything that is not ○ or △ counts as ng (× and ?)
	r := result{marks: map[string][2]string{"a": {"○"}, "b": {"△"}, "c": {"×"}, "d": {"?"}}}
	if ok, part, ng := score(r); ok != 1 || part != 1 || ng != 2 {
		t.Fatalf("score = %d/%d/%d, want 1/1/2", ok, part, ng)
	}
}
