package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const casesDir = "../../internal/ai/eval"

func TestCaseFilesValid(t *testing.T) {
	for _, f := range []string{"cases.json", "demo_cases.json", "m0disc_cases.json", "audit_cases.json", "audit_en_cases.json", "groupby_cases.json", "parent_child_cases.json", "ai_generate_cases.json"} {
		if err := checkCasesFile(filepath.Join(casesDir, f), false); err != nil {
			t.Error(err)
		}
	}
	for _, f := range []string{"chat_cases.json", "m0disc_chat_cases.json"} {
		if err := checkCasesFile(filepath.Join(casesDir, f), true); err != nil {
			t.Error(err)
		}
	}
}

// m0discPending: cases left without expect (none since the M0 integration of
// #378 / #379 / #384 / #398 / #405). Add an id here only with a reason.
var m0discPending = map[string]string{}

// m0disc is the M0 regression set (#281): every case carries an expect.
func TestM0discEveryCaseHasExpect(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(casesDir, "m0disc_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if len(c.Expect) == 0 && m0discPending[c.ID] == "" {
			t.Errorf("%s: no expect", c.ID)
		}
	}
}

// D-10 (#258): every m0disc chat step checks the reply for DSL words.
func TestM0discChatChecksDSLTerms(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(casesDir, "m0disc_chat_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []ChatCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		for i, st := range c.Steps {
			for _, w := range []string{"public", "private", "disclosure"} {
				if !slices.Contains(st.TextNot, w) {
					t.Errorf("%s step %d: textNot lacks %s", c.ID, i+1, w)
				}
			}
		}
	}
}

func TestValidateExpect(t *testing.T) {
	ok := map[string]any{"photo": true, "scope": "public", "notification": "08:00", "has": []any{"mapView"}, "tabs": 2.0}
	if bad := validateExpect(ok); len(bad) > 0 {
		t.Fatalf("valid expect rejected: %v", bad)
	}
	for name, exp := range map[string]map[string]any{
		"unknown key":      {"phot": true},
		"bool as string":   {"photo": "true"},
		"bad notification": {"notification": "8時"},
		"bad scope":        {"scope": "friends"},
		"empty list":       {"has": []any{}},
		"list of numbers":  {"lacks": []any{1.0}},
		"keep alone":       {"keep": []any{"a"}},
		"optHas alone":     {"optHas": []any{"a"}},
	} {
		if bad := validateExpect(exp); len(bad) == 0 {
			t.Errorf("%s: accepted %v", name, exp)
		}
	}
}

func TestValidateCases(t *testing.T) {
	cases := []Case{
		{ID: "a", Kind: "create", Transcript: "x"},
		{ID: "a", Kind: "create", Transcript: "x"},
		{ID: "b", Kind: "edit", Base: "later", Transcript: "x"},
		{ID: "c", Kind: "edit", Base: "example:kakeibo", Transcript: "x"},
		{ID: "d", Kind: "patch", Transcript: ""},
	}
	got := strings.Join(validateCases(cases), "\n")
	for _, want := range []string{"[a] duplicate id", "[b] base later", "[d] kind must be", "[d] empty transcript"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "[c]") {
		t.Errorf("example base rejected:\n%s", got)
	}
}

func TestCheckExpectShared(t *testing.T) {
	for _, tc := range []struct {
		scopes []string
		want   bool
	}{
		{[]string{"post:public"}, true},
		{[]string{"memo:private", "chore:shared"}, true},
		{[]string{"memo:private"}, false},
		{nil, false},
	} {
		m := &Metrics{Scopes: tc.scopes}
		if miss := checkExpect(map[string]any{"shared": tc.want}, m, ""); len(miss) > 0 {
			t.Errorf("%v want shared=%v: %v", tc.scopes, tc.want, miss)
		}
		if miss := checkExpect(map[string]any{"shared": !tc.want}, m, ""); len(miss) == 0 {
			t.Errorf("%v: shared=%v passed", tc.scopes, !tc.want)
		}
	}
}

func TestPassByGroup(t *testing.T) {
	recs := []Record{
		{ID: "a", Group: "g1", OK: true},
		{ID: "b", Group: "g1", OK: true, ExpectMisses: []string{"photo=false"}},
		{ID: "c", Group: "g2", OK: false},
		{ID: "d", Group: "g2", OK: true},
	}
	got := passByGroup(recs, map[string]bool{"a": true, "b": true, "c": true})
	want := []groupPass{
		{Group: "g1", Checked: 2, Pass: 1},
		{Group: "g2", Checked: 1, Pass: 0, Unchecked: 1},
		{Group: "total", Checked: 3, Pass: 1, Unchecked: 1},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}
