package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestLimitHitsWeekly(t *testing.T) {
	var out bytes.Buffer
	handled, err := runLogMode([]string{"limit-hits", "-logs", "testdata/limit_hits.json", "-period", "2026-11-02..2026-11-15"}, &out)
	if !handled || err != nil {
		t.Fatalf("handled=%t err=%v", handled, err)
	}
	want := `## daily limit (429) by week: 2026-11-02..2026-11-15

| week (Mon, JST) | users | reached | reached days | 429s | chats before (p50 / p90 / max) | creates before | edits before | total before |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 2026-11-02 | 3 | 2 | 3 | 4 | 1 / 2 / 2 | 1 / 1 / 1 | 0 / 2 / 2 | 2 / 5 / 5 |
| 2026-11-09 | 2 | 1 | 1 | 1 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
`
	got := out.String()
	if !strings.HasPrefix(got, want) {
		t.Fatalf("got:\n%s\nwant prefix:\n%s", got, want)
	}
	for _, h := range []string{"u1", "u2", "u5"} {
		if strings.Contains(got, h) {
			t.Fatalf("the table must not carry uid hashes (%s)", h)
		}
	}
}

func TestLimitHitsEmptyWeeks(t *testing.T) {
	var out bytes.Buffer
	// a period with no 429: every week is listed, distributions are "-"
	if _, err := runLogMode([]string{"limit-hits", "-logs", "testdata/limit_hits.json", "-period", "2026-11-18..2026-11-30"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{
		"| 2026-11-16 | 0 | 0 | 0 | 0 | - | - | - | - |",
		"| 2026-11-23 | 0 | 0 | 0 | 0 | - | - | - | - |",
		"| 2026-11-30 | 0 | 0 | 0 | 0 | - | - | - | - |",
	} {
		if !strings.Contains(out.String(), row) {
			t.Errorf("missing %q in:\n%s", row, out.String())
		}
	}
}

func TestLimitHitsArgs(t *testing.T) {
	for name, args := range map[string][]string{
		"no period":  {"-logs", "testdata/limit_hits.json"},
		"bad period": {"-logs", "testdata/limit_hits.json", "-period", "2026-11"},
		"no logs":    {"-period", "2026-11-02..2026-11-15"},
		"extra arg":  {"-logs", "testdata/limit_hits.json", "-period", "2026-11-02..2026-11-15", "x"},
	} {
		if _, err := runLogMode(append([]string{"limit-hits"}, args...), &bytes.Buffer{}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestWeekStart(t *testing.T) {
	for in, want := range map[string]string{
		"2026-11-02T00:00:00+09:00": "2026-11-02", // Monday
		"2026-11-08T23:59:00+09:00": "2026-11-02", // Sunday
		"2026-11-08T15:00:00Z":      "2026-11-09", // Monday 00:00 JST
	} {
		at, err := time.Parse(time.RFC3339, in)
		if err != nil {
			t.Fatal(err)
		}
		if got := weekStart(at).Format(time.DateOnly); got != want {
			t.Errorf("weekStart(%s) = %s, want %s", in, got, want)
		}
	}
	if nearestRank([]int{5, 1, 2}, 0.5) != 2 || nearestRank(nil, 0.9) != 0 {
		t.Error("nearestRank")
	}
}
