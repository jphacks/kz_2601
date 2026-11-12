package main

import (
	"strings"
	"testing"
	"time"
)

func TestRetryBreakdown(t *testing.T) {
	keep := map[string]bool{"aa": true}
	// edit: invalid_output then 2 validation (bad_ref 3 problems, missing_field 1)
	// create: 1 validation (bad_ref 1); one first-try success; a line before
	// validation_codes existed; a bogus code is dropped; one line outside the window
	lines := `{"time":"2026-11-05T10:00:00Z","event":"ai.generate","mode":"edit","outcome":"regenerated","retry_reasons":"invalid_output,validation,validation","validation_codes":"bad_ref:3,missing_field:1","uid_hash":"aa"}
{"time":"2026-11-05T10:01:00Z","event":"ai.generate","mode":"create","outcome":"regenerated","retry_reasons":"validation","validation_codes":"bad_ref:1,<b>:9,x:y","uid_hash":"aa"}
{"time":"2026-11-05T10:02:00Z","event":"ai.generate","mode":"edit","outcome":"validated","uid_hash":"aa"}
{"time":"2026-11-05T10:03:00Z","event":"ai.generate","mode":"edit","outcome":"regenerated","retry_reasons":"lint","uid_hash":"aa"}
{"time":"2026-11-09T10:00:00Z","event":"ai.generate","mode":"edit","outcome":"regenerated","retry_reasons":"validation","validation_codes":"bad_ref:5","uid_hash":"aa"}`
	evs, err := parseLogs(strings.NewReader(lines), keep)
	if err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	r := retryBreakdowns(evs, since, since.Add(48*time.Hour))
	if r.Lines != 4 || r.Retried != 3 || r.ByMode["edit"] != 2 || r.ByMode["create"] != 1 {
		t.Fatalf("lines: %+v", r)
	}
	if r.Reasons["validation"] != 3 || r.ReasonLines["validation"] != 2 || r.Reasons["invalid_output"] != 1 || r.Reasons["lint"] != 1 {
		t.Fatalf("reasons: %+v", r)
	}
	if len(r.Codes) != 2 || r.Codes["bad_ref"] != 4 || r.CodeLines["bad_ref"] != 2 || r.Codes["missing_field"] != 1 {
		t.Fatalf("codes: %+v", r)
	}
	var b strings.Builder
	renderRetryBreakdown(&b, r)
	s := b.String()
	for _, want := range []string{"| validation | 3 | 2 |", "| bad_ref | 4 | 2 |", "| missing_field | 1 | 1 |", "3 / 4(75%・edit 2・create 1)"} {
		if !strings.Contains(s, want) {
			t.Errorf("render lacks %q:\n%s", want, s)
		}
	}
	if strings.Index(s, "| bad_ref") > strings.Index(s, "| missing_field") {
		t.Errorf("codes not ordered by count:\n%s", s)
	}
}

func TestRetryBreakdown_Empty(t *testing.T) {
	var b strings.Builder
	renderRetryBreakdown(&b, retryBreakdowns(nil, time.Time{}, time.Now()))
	if !strings.Contains(b.String(), "retry_reasons の付いた行が無い") {
		t.Fatalf("render:\n%s", b.String())
	}
}
