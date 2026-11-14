package main

import (
	"bytes"
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

// #3392: the local check from PR #3104 as a test — a run file that is in the repo
// goes through evaluate and report, and the tables come out. No API, no network.
func TestAuditRawFixture(t *testing.T) {
	raw, err := os.ReadFile("../../internal/ai/eval/audit_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []auditCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}

	runPath := "../../../docs/backend/ai-eval/raw/2026-10-24-claude-sonnet-5-5-low-audit3-before.json"
	if _, err := os.Stat(runPath); err != nil {
		t.Fatal(err)
	}
	res, rf := evaluate(cases, runPath)

	var buf bytes.Buffer
	report(&buf, cases, res, rf, nil)
	out := buf.String()

	for _, want := range []string{
		"## 機械判定(" + rf.Model,
		"| id | 生成 | needs ○/△/× | 欠落した needs | lint 件数 |",
		"## needs 別の充足(○/△/×)",
		"## lint 種別の件数",
		"## ケース別の詳細",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "比較 run") {
		t.Errorf("compare column without -compare:\n%s", out)
	}

	rows := 0
	for _, c := range cases {
		if strings.Contains(out, "| "+c.ID+" | ") {
			rows++
		}
	}
	if rows == 0 {
		t.Errorf("no case row in the table:\n%s", out)
	}
}
