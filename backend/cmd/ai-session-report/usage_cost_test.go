package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUsageCostBeforeAfter(t *testing.T) {
	var out bytes.Buffer
	handled, err := runLogMode([]string{"usage-cost", "-logs", "testdata/usage_cost.json",
		"-period", "2026-11-01..2026-11-07", "-period", "2026-11-08..2026-11-14"}, &out)
	if !handled || err != nil {
		t.Fatalf("handled=%t err=%v", handled, err)
	}
	want := `## ai.usage by operation: 2026-11-01..2026-11-07 → 2026-11-08..2026-11-14

| operation | calls | cache read | cache write | cost / call (median, USD) | cost total (USD) |
| --- | ---: | ---: | ---: | ---: | ---: |
| chat_first | 1 → 0 | 1/1 (100%) → - | 0/1 (0%) → - | 0.0100 → - | 0.0100 → - |
| generate | 3 → 3 | 0/3 (0%) → 2/3 (67%) | 1/3 (33%) → 1/3 (33%) | 0.0500 → 0.0300 | 0.1000 → 0.1000 |
| patch | 1 → 0 | 0/1 (0%) → - | 0/1 (0%) → - | 0.0200 → - | 0.0200 → - |
| (all) | 5 → 3 | 1/5 (20%) → 2/3 (67%) | 1/5 (20%) → 1/3 (33%) | 0.0300 → 0.0300 | 0.1300 → 0.1000 |
`
	if got := out.String(); !strings.HasPrefix(got, want) {
		t.Fatalf("got:\n%s\nwant prefix:\n%s", got, want)
	}
}

func TestUsageCostOnePeriod(t *testing.T) {
	var out bytes.Buffer
	if _, err := runLogMode([]string{"usage-cost", "-logs", "testdata/usage_cost.json", "-period", "2026-11-08..2026-11-30"}, &out); err != nil {
		t.Fatal(err)
	}
	// the 11-20 line is in; ai.generate / http.request lines are not ai.usage calls
	if !strings.Contains(out.String(), "| generate | 4 | 2/4 (50%) | 1/4 (25%) | 0.0400 | 9.1000 |") {
		t.Fatalf("got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "u1") || strings.Contains(out.String(), "uid") {
		t.Fatal("the table must not carry users")
	}
}

func TestUsageCostArgs(t *testing.T) {
	for name, args := range map[string][]string{
		"no period":     {"-logs", "testdata/usage_cost.json"},
		"three periods": {"-logs", "testdata/usage_cost.json", "-period", "2026-11-01..2026-11-01", "-period", "2026-11-02..2026-11-02", "-period", "2026-11-03..2026-11-03"},
		"bad period":    {"-logs", "testdata/usage_cost.json", "-period", "2026-11-08"},
		"reversed":      {"-logs", "testdata/usage_cost.json", "-period", "2026-11-08..2026-11-01"},
		"no logs":       {"-period", "2026-11-01..2026-11-07"},
		"missing file":  {"-logs", "testdata/nope.json", "-period", "2026-11-01..2026-11-07"},
		"extra arg":     {"-logs", "testdata/usage_cost.json", "-period", "2026-11-01..2026-11-07", "x"},
	} {
		if _, err := runLogMode(append([]string{"usage-cost"}, args...), &bytes.Buffer{}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if handled, _ := runLogMode([]string{"-uids", "a"}, &bytes.Buffer{}); handled {
		t.Error("flags are not a mode")
	}
}

func TestUsageStatMedian(t *testing.T) {
	s := &usageStat{Costs: []float64{3, 1, 2}}
	if m, ok := s.median(); !ok || m != 2 {
		t.Errorf("odd median %v", m)
	}
	if _, ok := (&usageStat{}).median(); ok {
		t.Error("no priced line has no median")
	}
}
