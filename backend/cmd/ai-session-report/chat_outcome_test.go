package main

import (
	"strings"
	"testing"
	"time"
)

func TestChatOutcomes(t *testing.T) {
	keep := map[string]bool{"aa": true}
	// app x1: question -> question -> ready, then a new conversation (turn 1 again): answer
	// app x2: proposal (1 turn); one failed turn; a line outside the window
	lines := `{"time":"2026-11-05T10:00:00Z","event":"ai.chat","outcome":"replied","reply_kind":"question","turn":1,"uid_hash":"aa","app_hash":"x1","duration_ms":900}
{"time":"2026-11-05T10:01:00Z","event":"ai.chat","outcome":"replied","reply_kind":"question","turn":2,"uid_hash":"aa","app_hash":"x1"}
{"time":"2026-11-05T10:01:30Z","event":"ai.chat","outcome":"replied","reply_kind":"proposal","turn":1,"uid_hash":"aa","app_hash":"x2"}
{"time":"2026-11-05T10:02:00Z","event":"ai.chat","outcome":"replied","reply_kind":"ready","turn":3,"clarify_ended_in_build":true,"uid_hash":"aa","app_hash":"x1"}
{"time":"2026-11-05T10:03:00Z","event":"ai.chat","outcome":"failed","error_code":"generation_failed","uid_hash":"aa","app_hash":"x1"}
{"time":"2026-11-05T11:00:00Z","event":"ai.chat","outcome":"replied","reply_kind":"answer","turn":1,"uid_hash":"aa","app_hash":"x1"}
{"time":"2026-11-05T11:00:05Z","event":"ai.chat","outcome":"replied","reply_kind":"mystery","turn":2,"uid_hash":"aa","app_hash":"x1"}
{"time":"2026-11-09T10:00:00Z","event":"ai.chat","outcome":"replied","reply_kind":"ready","turn":1,"uid_hash":"aa","app_hash":"x3"}`
	evs, err := parseLogs(strings.NewReader(lines), keep)
	if err != nil {
		t.Fatal(err)
	}
	if evs[0].Duration != 900*time.Millisecond {
		t.Errorf("duration_ms: %v", evs[0].Duration)
	}
	since := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	o := chatOutcomes(evs, since, since.Add(48*time.Hour), 30*time.Minute)
	if o.Turns != 6 || o.Failed != 1 || o.ClarifyBuilt != 1 {
		t.Fatalf("counts: %+v", o)
	}
	for k, want := range map[string]int{"question": 2, "ready": 1, "answer": 1, "proposal": 1, "other": 1} {
		if o.ReplyKinds[k] != want {
			t.Errorf("reply %s = %d, want %d", k, o.ReplyKinds[k], want)
		}
	}
	if o.Conversations != 3 || o.EndedReady != 1 {
		t.Fatalf("conversations: %+v", o)
	}
	for k, want := range map[string]int{"1": 1, "2": 1, "3": 1} {
		if o.TurnsPerConv[k] != want {
			t.Errorf("turns %s = %d, want %d (%v)", k, o.TurnsPerConv[k], want, o.TurnsPerConv)
		}
	}
	var b strings.Builder
	renderChatOutcome(&b, o)
	if s := b.String(); !strings.Contains(s, "| question | 2 | 33% |") || !strings.Contains(s, "| 3 | 1 |") {
		t.Fatalf("render:\n%s", s)
	}
}

func TestTurnBucket(t *testing.T) {
	for n, want := range map[int]string{1: "1", 4: "4", 5: "5+", 12: "5+"} {
		if got := turnBucket(n); got != want {
			t.Errorf("turnBucket(%d) = %q, want %q", n, got, want)
		}
	}
}
