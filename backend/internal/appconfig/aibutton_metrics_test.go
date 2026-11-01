package appconfig

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func aiButtonBody(day string) map[string]any {
	return map[string]any{"days": []map[string]any{{
		"reportId": "0F8E2C1A-1111-4222-8333-944455556666", "day": day, "availability": "available",
		"tasks": []map[string]any{
			{"task": "summarize", "attempts": 5, "failures": map[string]int{"context_too_long": 2, "invalid_response": 1, "guardrail": 0}},
			{"task": "classify", "attempts": 3, "failures": map[string]int{}},
		},
	}}}
}

func TestAIButtonMetricsLogsCountsOnly(t *testing.T) {
	e := newEnv(t)
	logs := captureSlog(t)
	const rawUID = "uid-RAW-metrics-446"
	today := time.Now().UTC().Format(time.DateOnly)
	r := e.do(rawUID, http.MethodPost, "/v1/metrics/ai-button", aiButtonBody(today), map[string]string{"X-Client-Build": "42"})
	if r.code != http.StatusNoContent {
		t.Fatalf("status %d: %s", r.code, r.raw)
	}
	if strings.Contains(logs.String(), rawUID) {
		t.Fatal("raw uid must not be logged")
	}
	byEvent := map[string][]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %s", line)
		}
		if ev, _ := m["event"].(string); ev != "" {
			byEvent[ev] = append(byEvent[ev], m)
		}
	}
	if n := len(byEvent[AIButtonAvailabilityEvent]); n != 1 {
		t.Fatalf("availability lines = %d", n)
	}
	a := byEvent[AIButtonAvailabilityEvent][0]
	if a["availability"] != "available" || a["day"] != today || a["client_build"] != "42" || a["uid_hash"] != nil {
		t.Fatalf("availability line: %v", a)
	}
	if n := len(byEvent[AIButtonTaskEvent]); n != 2 {
		t.Fatalf("task lines = %d", n)
	}
	for _, l := range byEvent[AIButtonTaskEvent] {
		if l["task"] == "summarize" && (l["attempts"] != float64(5) || l["failures"] != float64(3)) {
			t.Fatalf("summarize line: %v", l)
		}
	}
	if n := len(byEvent[AIButtonFailureEvent]); n != 2 { // zero counts are skipped
		t.Fatalf("failure lines = %d", n)
	}
}

func TestAIButtonMetricsRejects(t *testing.T) {
	e := newEnv(t)
	today := time.Now().UTC().Format(time.DateOnly)
	mutate := func(f func(d map[string]any)) map[string]any {
		b := aiButtonBody(today)
		f(b["days"].([]map[string]any)[0])
		return b
	}
	cases := map[string]any{
		"empty":           map[string]any{"days": []any{}},
		"free text field": mutate(func(d map[string]any) { d["prompt"] = "私の日記" }),
		"unknown task":    mutate(func(d map[string]any) { d["tasks"] = []map[string]any{{"task": "日記を要約", "attempts": 1}} }),
		"unknown reason": mutate(func(d map[string]any) {
			d["tasks"] = []map[string]any{{"task": "rewrite", "attempts": 1, "failures": map[string]int{"秘密": 1}}}
		}),
		"unknown avail": mutate(func(d map[string]any) { d["availability"] = "maybe" }),
		"old day":       mutate(func(d map[string]any) { d["day"] = "2020-01-01" }),
		"bad report id": mutate(func(d map[string]any) { d["reportId"] = "x" }),
		"negative":      mutate(func(d map[string]any) { d["tasks"] = []map[string]any{{"task": "rewrite", "attempts": -1}} }),
		"repeated task": mutate(func(d map[string]any) { d["tasks"] = []map[string]any{{"task": "rewrite"}, {"task": "rewrite"}} }),
		"too many failures": mutate(func(d map[string]any) {
			d["tasks"] = []map[string]any{{"task": "rewrite", "failures": map[string]int{"refusal": 10001}}}
		}),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if r := e.do("u1", http.MethodPost, "/v1/metrics/ai-button", body, nil); r.code != http.StatusBadRequest {
				t.Fatalf("status %d: %s", r.code, r.raw)
			}
		})
	}
	if r := e.do("", http.MethodPost, "/v1/metrics/ai-button", aiButtonBody(today), nil); r.code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status %d", r.code)
	}
}
