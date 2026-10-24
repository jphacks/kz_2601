package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// #172: the medicine streak case scores ○ on the canonical example (no API).
func TestMedicineStreakCase(t *testing.T) {
	raw, err := os.ReadFile("../../internal/ai/eval/audit_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []auditCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	var c *auditCase
	for i := range cases {
		if cases[i].ID == "a08b_medicine_streak" {
			c = &cases[i]
		}
	}
	if c == nil {
		t.Fatal("audit case a08b_medicine_streak missing")
	}

	ex, err := os.ReadFile("../../../docs/dsl/examples/medicine.json")
	if err != nil {
		t.Fatal(err)
	}
	a, err := load(ex)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range c.Needs {
		if m, why := need(a, n); m != "○" {
			t.Errorf("need %s = %s (%s)", n, m, why)
		}
	}
	if l := lint(a); len(l) > 0 {
		t.Errorf("lint: %v", l)
	}

	// a stat "直近7日の回数" that sums a doneInDays computed is not a missing period filter
	withLast7 := strings.Replace(string(ex), `"label": "最長の連続(日)"`, `"label": "直近7日の回数"`, 1)
	withLast7 = strings.Replace(withLast7, `"agg": "max", "entity": "medicine", "field": "streak"`, `"agg": "sum", "entity": "medicine", "field": "last7"`, 1)
	a, err = load([]byte(withLast7))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lint(a) {
		if strings.Contains(l, "直近7日") {
			t.Errorf("lint flagged doneInDays stat: %s", l)
		}
	}

	// without streak the need is not met
	noStreak := strings.ReplaceAll(string(ex), `"op": "streak"`, `"op": "toNumber"`)
	a, _ = load([]byte(noStreak))
	if m, _ := need(a, "streak"); m == "○" {
		t.Error("need streak should not be ○ without the streak op")
	}
}
