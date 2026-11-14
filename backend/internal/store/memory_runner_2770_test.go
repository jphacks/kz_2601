package store

import (
	"errors"
	"maps"
	"testing"
	"time"
)

func finishOutboxFixture(t *testing.T) (*Memory, string, OutboxEvent) {
	t.Helper()
	base := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	m := NewMemory()
	m.now = func() time.Time { return base }
	env := "finish2770"
	rs := PublicRecords("APP2770", "post")
	emit := func(_ *Record, next Record) []OutboxEvent {
		return []OutboxEvent{{Env: env, Kind: OutboxRecordCreated, AppID: "APP2770", Scope: "public",
			Entity: next.Entity, RecordID: next.ID, Version: 1, RuleIDs: []string{"r1"}, ActorUID: "u1"}}
	}
	if _, created, err := m.CreateRecordEmit(t.Context(), rs, Record{ID: "rec1", Data: map[string]any{"n": "a"}, Author: "u1"}, emit); err != nil || !created {
		t.Fatalf("CreateRecordEmit: created=%v err=%v", created, err)
	}
	claimed, err := m.ClaimOutbox(t.Context(), env, base.Add(time.Second), time.Minute, 0)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("ClaimOutbox: %+v %v", claimed, err)
	}
	if claimed[0].Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", claimed[0].Attempts)
	}
	return m, env, claimed[0]
}

func storedOutbox(t *testing.T, m *Memory, env, eventID string) OutboxEvent {
	t.Helper()
	evs, err := m.OutboxEvents(t.Context(), env, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.EventID == eventID {
			return ev
		}
	}
	t.Fatalf("event %q not in %+v", eventID, evs)
	return OutboxEvent{}
}

func TestMemoryFinishOutboxStoresTheResultAndDropsAvailableAt_2770(t *testing.T) {
	m, env, ev := finishOutboxFixture(t)
	finishedAt := time.Date(2026, 3, 4, 5, 10, 0, 0, time.UTC)
	res := OutboxResult{Status: OutboxDone, Outcome: "ok", RuleOutcomes: map[string]string{"r1": "ok"}, FinishedAt: finishedAt}
	if err := m.FinishOutbox(t.Context(), env, ev.EventID, ev.Attempts, res); err != nil {
		t.Fatal(err)
	}
	got := storedOutbox(t, m, env, ev.EventID)
	if !got.AvailableAt.IsZero() {
		t.Fatalf("AvailableAt = %v, want zero", got.AvailableAt)
	}
	if got.Status != OutboxDone || got.Outcome != "ok" || got.LastError != "" || !got.FinishedAt.Equal(finishedAt) {
		t.Fatalf("stored = %+v", got)
	}
	if !maps.Equal(got.RuleOutcomes, map[string]string{"r1": "ok"}) {
		t.Fatalf("RuleOutcomes = %v", got.RuleOutcomes)
	}
	if got.Attempts != ev.Attempts {
		t.Fatalf("Attempts = %d, want %d", got.Attempts, ev.Attempts)
	}
}

func TestMemoryFinishOutboxKeepsTheGaveUpErrorCode_2770(t *testing.T) {
	m, env, ev := finishOutboxFixture(t)
	res := OutboxResult{Status: OutboxGaveUp, Outcome: "error", LastError: "http_500", FinishedAt: time.Date(2026, 3, 4, 6, 0, 0, 0, time.UTC)}
	if err := m.FinishOutbox(t.Context(), env, ev.EventID, ev.Attempts, res); err != nil {
		t.Fatal(err)
	}
	got := storedOutbox(t, m, env, ev.EventID)
	if got.Status != OutboxGaveUp || got.Outcome != "error" || got.LastError != "http_500" || got.RuleOutcomes != nil {
		t.Fatalf("stored = %+v", got)
	}
}

func TestMemoryFinishOutboxCopiesRuleOutcomes_2770(t *testing.T) {
	m, env, ev := finishOutboxFixture(t)
	outcomes := map[string]string{"r1": "ok"}
	if err := m.FinishOutbox(t.Context(), env, ev.EventID, ev.Attempts, OutboxResult{Status: OutboxDone, Outcome: "ok", RuleOutcomes: outcomes}); err != nil {
		t.Fatal(err)
	}
	outcomes["r1"] = "changed"
	outcomes["r2"] = "added"
	if got := storedOutbox(t, m, env, ev.EventID); !maps.Equal(got.RuleOutcomes, map[string]string{"r1": "ok"}) {
		t.Fatalf("RuleOutcomes = %v, want the values at the call", got.RuleOutcomes)
	}
}

func TestMemoryFinishOutboxRejectsAnotherAttempt_2770(t *testing.T) {
	m, env, ev := finishOutboxFixture(t)
	if err := m.FinishOutbox(t.Context(), env, ev.EventID, ev.Attempts+1, OutboxResult{Status: OutboxDone, Outcome: "ok"}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("err = %v, want ErrLeaseLost", err)
	}
	got := storedOutbox(t, m, env, ev.EventID)
	if got.Status != OutboxPending || got.AvailableAt.IsZero() || got.Outcome != "" {
		t.Fatalf("stored = %+v, want it untouched", got)
	}
}

func TestMemoryFinishOutboxRejectsAnEventThatIsNoLongerLeased_2770(t *testing.T) {
	m, env, ev := finishOutboxFixture(t)
	first := OutboxResult{Status: OutboxDone, Outcome: "ok", FinishedAt: time.Date(2026, 3, 4, 5, 10, 0, 0, time.UTC)}
	if err := m.FinishOutbox(t.Context(), env, ev.EventID, ev.Attempts, first); err != nil {
		t.Fatal(err)
	}
	if err := m.FinishOutbox(t.Context(), env, ev.EventID, ev.Attempts, OutboxResult{Status: OutboxGaveUp, Outcome: "error", LastError: "late"}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("err = %v, want ErrLeaseLost", err)
	}
	got := storedOutbox(t, m, env, ev.EventID)
	if got.Status != OutboxDone || got.Outcome != "ok" || got.LastError != "" || !got.FinishedAt.Equal(first.FinishedAt) {
		t.Fatalf("stored = %+v, want the first result", got)
	}
}

func TestMemoryFinishOutboxOnAnUnknownEvent_2770(t *testing.T) {
	m, env, ev := finishOutboxFixture(t)
	for _, tc := range []struct{ name, env, eventID string }{
		{"unknown event id", env, "evt_nope"},
		{"unknown env", "other2770", ev.EventID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.FinishOutbox(t.Context(), tc.env, tc.eventID, ev.Attempts, OutboxResult{Status: OutboxDone}); !errors.Is(err, ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
		})
	}
}
