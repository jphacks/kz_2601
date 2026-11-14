package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Saved runs only: no API calls (#371).

func gateRecs() ([]Record, map[string]bool) {
	recs := []Record{
		{ID: "a", Group: "g1", OK: true, Checked: true},
		{ID: "b", Group: "g1", OK: true, Checked: true, ExpectMisses: []string{"has!a|b"}},
		{ID: "c", Group: "g2", OK: true, Checked: true},
		{ID: "d", Group: "g2", OK: false, Checked: true, Error: "boom\nline2"},
		{ID: "e", Group: "g2", OK: true, Checked: true},
		{ID: "x", Group: "g3", OK: true},
	}
	has := map[string]bool{}
	for _, r := range recs {
		has[r.ID] = r.Checked
	}
	return recs, has // g1 1/2, g2 2/3, total 3/5
}

func TestLoadGate(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "th.json")
	if err := os.WriteFile(file, []byte(`{"g1": 0.5, "total": 0.6}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := loadGate(0.8, file)
	if err != nil || g["g1"] != 0.5 || g["total"] != 0.8 {
		t.Errorf("stricter total wins: %v %v", g, err)
	}
	if g, err = loadGate(0.4, file); err != nil || g["total"] != 0.6 {
		t.Errorf("file total stricter: %v %v", g, err)
	}
	if g, err = loadGate(0, ""); err != nil || len(g) != 0 {
		t.Errorf("no flags = no gate: %v %v", g, err)
	}
	if _, err = loadGate(90, ""); err == nil || !strings.Contains(err.Error(), "between 0 and 1") {
		t.Errorf("90 is a percent, want an error: %v", err)
	}
	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte(`[0.9]`), 0o644)
	if _, err = loadGate(0, bad); err == nil {
		t.Error("an array is not a threshold file")
	}
	if _, err = loadGate(0, filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing file must fail")
	}
}

func TestGateCheck(t *testing.T) {
	recs, has := gateRecs()
	rows := passByGroup(recs, has)
	tests := []struct {
		name string
		g    gate
		want []string // substrings of the error; nil = pass
	}{
		{"no gate", gate{}, nil},
		{"total exactly at min", gate{"total": 0.6}, nil},
		{"total below", gate{"total": 0.61}, []string{"total: 60% (3 / 5) < min 61%"}},
		{"per group", gate{"g1": 0.5, "g2": 0.7}, []string{"g2: 67% (2 / 3) < min 70%"}},
		{"unknown group fails", gate{"typo": 0.1}, []string{"typo: no checked case"}},
		{"group without expect fails", gate{"g3": 0}, []string{"g3: no checked case"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := gateError(tt.g.check(rows))
			if tt.want == nil {
				if err != nil {
					t.Fatalf("want pass, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("want an error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

func TestEmitMarkdown(t *testing.T) {
	recs, has := gateRecs()
	var out bytes.Buffer
	plainCalled := false
	o := evalOutput{Format: "md", Gate: gate{"g1": 0.5, "total": 0.9, "typo": 0.5}}
	err := o.emit(&out, recs, has, "raw/*.json", func() { plainCalled = true })
	if err == nil {
		t.Fatal("total 60% < 90% must fail")
	}
	if plainCalled {
		t.Error("md to stdout replaces the plain table")
	}
	md := out.String()
	for _, want := range []string{
		"**判定: FAIL**",
		"対象: `raw/*.json`",
		"| group | pass / checked | pass% | min | result | no expect |",
		"| g1 | 1 / 2 | 50% | 50% | PASS | 0 |",
		"| g2 | 2 / 3 | 67% | - | - | 0 |",
		"| g3 | 0 / 0 | - | - | - | 1 |",
		"| total | 3 / 5 | 60% | 90% | FAIL | 1 |",
		"| typo | 0 / 0 | - | 50% | FAIL | 0 |",
		"通らなかったケース (2)",
		`| b | g1 | has!a\|b |`,
		"| d | g2 | error: boom line2 |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "| x |") {
		t.Error("a case without expect is not a failure")
	}
}

func TestEmitMarkdownToFile(t *testing.T) {
	recs, has := gateRecs()
	file := filepath.Join(t.TempDir(), "report.md")
	var out bytes.Buffer
	plainCalled := false
	o := evalOutput{Format: "md", Out: file, Gate: gate{"total": 0.5}}
	if err := o.emit(&out, recs, has, "", func() { plainCalled = true }); err != nil {
		t.Fatal(err)
	}
	if !plainCalled || !strings.Contains(out.String(), "wrote "+file) {
		t.Errorf("plain table stays on stdout: %v %q", plainCalled, out.String())
	}
	md, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(md), "**判定: PASS**") || strings.Contains(string(md), "対象:") {
		t.Errorf("report: %v\n%s", err, md)
	}
}

// Without the new flags the output path and the exit stay as before.
func TestEmitDefaultIsPlain(t *testing.T) {
	recs, has := gateRecs()
	var out bytes.Buffer
	plainCalled := false
	if err := (evalOutput{}).emit(&out, recs, has, "", func() { plainCalled = true }); err != nil {
		t.Fatal(err)
	}
	if !plainCalled || out.Len() != 0 || (evalOutput{}).active() {
		t.Errorf("plain %v, extra output %q", plainCalled, out.String())
	}
	if validateReportFormat("") != nil || validateReportFormat("md") != nil || validateReportFormat("html") == nil {
		t.Error("report formats: '' and md only")
	}
}

// scoreRuns reads saved runs and fails below the gate after printing.
func TestScoreRunsGate(t *testing.T) {
	dir := t.TempDir()
	recs, _ := gateRecs()
	raw, _ := json.Marshal(Run{Cases: recs})
	if err := os.WriteFile(filepath.Join(dir, "2026-11-12-a.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	glob := filepath.Join(dir, "*.json")
	report := filepath.Join(dir, "out.md")
	if err := scoreRuns(glob, evalOutput{Format: "md", Out: report, Gate: gate{"total": 0.6}}); err != nil {
		t.Errorf("60%% >= 60%%: %v", err)
	}
	if err := scoreRuns(glob, evalOutput{Gate: gate{"total": 0.7}}); err == nil {
		t.Error("60% < 70% must fail")
	}
	if _, err := os.Stat(report); err != nil {
		t.Error(err)
	}
}
