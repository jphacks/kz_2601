package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// developParts returns the component types and photoField.analyze kinds of
// the DSL schema on this branch.
func developParts(t *testing.T) (types, kinds map[string]bool) {
	t.Helper()
	var s struct {
		Defs map[string]struct {
			AnyOf []struct {
				Ref string `json:"$ref"`
			} `json:"anyOf"`
			Properties map[string]struct {
				Const any   `json:"const"`
				Enum  []any `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(dsl.SchemaJSON(), &s); err != nil {
		t.Fatal(err)
	}
	types, kinds = map[string]bool{}, map[string]bool{}
	for _, v := range s.Defs["Container"].Properties["type"].Enum {
		types[fmt.Sprint(v)] = true
	}
	for _, r := range s.Defs["Leaf"].AnyOf {
		types[fmt.Sprint(s.Defs[strings.TrimPrefix(r.Ref, "#/$defs/")].Properties["type"].Const)] = true
	}
	for _, v := range s.Defs["PhotoAnalyze"].Properties["kind"].Enum {
		kinds[fmt.Sprint(v)] = true
	}
	if len(types) < 40 || len(kinds) == 0 {
		t.Fatalf("schema read wrong: %d types, %d kinds", len(types), len(kinds))
	}
	return types, kinds
}

func loadCases(t *testing.T, name string) []Case {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(casesDir, name))
	if err != nil {
		t.Fatal(err)
	}
	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

func strs(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		out = append(out, x.(string))
	}
	return out
}

// usesNewPart reports whether a case expects a part or an expect key that
// this branch does not have yet.
func usesNewPart(c Case, types, kinds map[string]bool) bool {
	for _, ty := range strs(c.Expect["has"]) {
		if !types[ty] {
			return true
		}
	}
	for _, k := range strs(c.Expect["photoAnalyze"]) {
		if !kinds[k] {
			return true
		}
	}
	for k := range c.PendingExpect {
		if _, known := expectKeys[k]; !known {
			return true
		}
	}
	return false
}

var (
	coverageRepo = regexp.MustCompile(`^jphacks/([A-Za-z]+_){1,2}2[2-5]\d\d `)
	teamID       = regexp.MustCompile(`[A-Za-z]+_\d{4}`)
	prInGroup    = regexp.MustCompile(`^cov-pr(\d+)$`)
)

// TestCoverageCases: the past JPHACKS works (jphacks-coverage.md「測り方」)
// stay 30〜40, every case is a create with an expect and the work's repository
// in the note (not in the request), the develop file uses only parts on this
// branch, and every pending case names its PR and really needs it.
func TestCoverageCases(t *testing.T) {
	types, kinds := developParts(t)
	dev := loadCases(t, "jphacks_coverage_cases.json")
	pending := loadCases(t, "jphacks_coverage_pending_cases.json")
	if n := len(dev) + len(pending); n < 30 || n > 40 {
		t.Errorf("%d works, want 30〜40", n)
	}
	seen := map[string]bool{}
	for _, c := range append(append([]Case(nil), dev...), pending...) {
		repo := coverageRepo.FindString(c.Note)
		if c.Kind != "create" || len(c.Expect) == 0 || repo == "" {
			t.Errorf("%s: want a create with an expect and note jphacks/<repo> first (note %q)", c.ID, c.Note)
		}
		if seen[repo] {
			t.Errorf("%s: %s is in two cases", c.ID, repo)
		}
		seen[repo] = true
		if teamID.MatchString(c.Transcript) {
			t.Errorf("%s: the request names a team", c.ID)
		}
	}
	for _, c := range dev {
		if c.Group != "cov-full" && c.Group != "cov-reduced" {
			t.Errorf("%s: group %q, want cov-full (◎) or cov-reduced (○)", c.ID, c.Group)
		}
		if len(c.PendingExpect) > 0 || usesNewPart(c, types, kinds) {
			t.Errorf("%s: uses a part that is not on develop; move it to the pending file", c.ID)
		}
	}
	for _, c := range pending {
		m := prInGroup.FindStringSubmatch(c.Group)
		if m == nil || !strings.Contains(c.Note, "PR #"+m[1]) {
			t.Errorf("%s: group %q and note %q must name the same PR (cov-pr<N> / PR #<N>)", c.ID, c.Group, c.Note)
		}
		if !usesNewPart(c, types, kinds) {
			t.Errorf("%s: needs no new part; move it to jphacks_coverage_cases.json", c.ID)
		}
	}
}

func TestMergePendingExpect(t *testing.T) {
	c := Case{Expect: map[string]any{"photo": true}, PendingExpect: map[string]any{"chart": true, "notYetKey": true}}
	waiting := mergePendingExpect(&c)
	if len(waiting) != 1 || waiting[0] != "notYetKey" {
		t.Errorf("waiting = %v", waiting)
	}
	if c.Expect["chart"] != true || c.Expect["notYetKey"] != nil {
		t.Errorf("expect = %v", c.Expect)
	}
	bad := validatePendingExpect(map[string]any{"photo": true}, map[string]any{"photo": true, "chart": "x", "notYetKey": 1.0})
	if len(bad) != 2 {
		t.Errorf("bad = %v, want the duplicate and chart's type", bad)
	}
}

// TestScoreRuns: the last record of an id wins and only checked cases count.
func TestScoreRuns(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, recs ...Record) {
		raw, _ := json.Marshal(Run{Cases: recs})
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-11-07-a.json",
		Record{ID: "a", Group: "cov-full", OK: true, Checked: true, ExpectMisses: []string{"has!mapView"}},
		Record{ID: "b", Group: "cov-full", OK: true, Checked: true},
		Record{ID: "x", Group: "other", OK: true})
	write("2026-11-08-a.json", Record{ID: "a", Group: "cov-full", OK: true, Checked: true})
	recs, err := latestRecords(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	hasExpect := map[string]bool{}
	for _, r := range recs {
		hasExpect[r.ID] = r.Checked
	}
	rows := passByGroup(recs, hasExpect)
	total := rows[len(rows)-1]
	if len(recs) != 3 || total.Pass != 2 || total.Checked != 2 || total.Unchecked != 1 {
		t.Errorf("recs %d, total %+v", len(recs), total)
	}
	if err := scoreRuns(filepath.Join(dir, "*.json"), evalOutput{}); err != nil {
		t.Fatal(err)
	}
}
