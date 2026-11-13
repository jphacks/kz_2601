package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

const (
	beforeGlob2751 = "testdata/compare/before_*.json"
	afterGlob2751  = "testdata/compare/after_*.json"
)

// captureStdout2751 runs f with os.Stdout redirected to a pipe and returns
// what was written (compare prints straight to stdout).
func captureStdout2751(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = orig }()
	f()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestCompareLoadSide2751(t *testing.T) {
	s, err := loadSide("before", beforeGlob2751)
	if err != nil {
		t.Fatal(err)
	}
	if s.name != "before" || len(s.all) != 2 {
		t.Fatalf("side = %q with %d records, want before with 2", s.name, len(s.all))
	}
	if got := strings.Join(s.order, ","); got != "both,only-before" {
		t.Errorf("order = %q, want both,only-before", got)
	}
	if len(s.recs["both"]) != 1 || s.recs["both"][0].ClaudeMs != 1000 {
		t.Errorf("recs[both] = %+v", s.recs["both"])
	}
}

func TestCompareLoadSideErrors2751(t *testing.T) {
	if _, err := loadSide("before", "testdata/compare/nope_*.json"); err == nil || !strings.Contains(err.Error(), "no files match") {
		t.Errorf("no match: err = %v, want 'no files match'", err)
	}
	if _, err := loadSide("before", "testdata/compare/broken_*.json"); err == nil || !strings.Contains(err.Error(), "broken_2751.json") {
		t.Errorf("broken json: err = %v, want it to name the file", err)
	}
	if _, err := loadSide("before", "testdata/compare/[.json"); err == nil {
		t.Error("malformed glob should fail")
	}
}

func TestCompare2751(t *testing.T) {
	var cmpErr error
	out := captureStdout2751(t, func() { cmpErr = compare(beforeGlob2751, afterGlob2751) })
	if cmpErr != nil {
		t.Fatal(cmpErr)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	has := func(want string) {
		t.Helper()
		for _, l := range lines {
			if l == want {
				return
			}
		}
		t.Errorf("no line equals %q in:\n%s", want, out)
	}
	has("| id | kind | runs (before/after) | 一発通過 | Claude ms 中央値 before → after | 出力 tokens 中央値 before → after | first event ms (after) |")
	has("|---|---|---|---|---|---|---|")
	// a case on both sides: halved
	has("| both | create | 1/1 | 1/1 → 1/1 | 1000 → 500 (-50%) | 200 → 100 (-50%) | 120 |")
	// before only (failed run: no medians) and after only (kind from the after side)
	has("| only-before | edit | 1/0 | 0/0 → 0/0 | 0 → 0 (-) | 0 → 0 (-) | - |")
	has("| only-after | edit | 0/1 | 0/0 → 1/1 | 0 → 300 (-) | 0 → 30 (-) | - |")
	// row order: before's cases first, then the ones only in after
	var ids []string
	for _, l := range lines {
		for _, id := range []string{"| both ", "| only-before ", "| only-after "} {
			if strings.HasPrefix(l, id) {
				ids = append(ids, strings.TrimSpace(strings.TrimPrefix(id, "|")))
			}
		}
	}
	if got := strings.Join(ids, ","); got != "both,only-before,only-after" {
		t.Errorf("row order = %q", got)
	}
	has("| 指標 | before | after | 差 |")
	has("| 一発通過 | 1/2 | 2/2 | |")
	has("| expect miss | 0 | 0 | |")
}

func TestCompareErrors2751(t *testing.T) {
	if err := compare("testdata/compare/nope_*.json", afterGlob2751); err == nil || !strings.Contains(err.Error(), "before") {
		t.Errorf("missing before side: err = %v", err)
	}
	if err := compare(beforeGlob2751, "testdata/compare/nope_*.json"); err == nil || !strings.Contains(err.Error(), "after") {
		t.Errorf("missing after side: err = %v", err)
	}
}
