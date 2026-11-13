package loop

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// usageDown is a Control whose switches read fine but whose usage does not.
type usageDown struct{ *MemoryControl }

func (usageDown) Usage(context.Context, string) (int, error) { return 0, errors.New("deadline") }

func TestAllowLLMFailsClosed(t *testing.T) {
	ctx := context.Background()
	on := envOf(map[string]string{"LOOP_TRIAGE": "on"})
	// triage itself is stopped: the cap is not even read
	if err := (&Gate{Control: usageDown{NewMemoryControl()}, Getenv: envOf(nil)}).AllowLLM(ctx); !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "LOOP_TRIAGE") {
		t.Fatalf("stopped triage: %v", err)
	}
	// unreadable usage stops the call rather than spending blind
	if err := (&Gate{Control: usageDown{NewMemoryControl()}, Getenv: on}).AllowLLM(ctx); !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "usage unreadable") {
		t.Fatalf("unreadable usage: %v", err)
	}
}

func TestAllowNeedsExactlyOn(t *testing.T) {
	ctx := context.Background()
	for _, v := range []string{"", "1", "true", "ON", " on"} {
		g := &Gate{Control: NewMemoryControl(), Getenv: envOf(map[string]string{"LOOP_DRAFT": v})}
		if err := g.Allow(ctx, StageDraft); !errors.Is(err, ErrStopped) {
			t.Errorf("LOOP_DRAFT=%q runs", v)
		}
	}
	// LOOP_PAUSED wins over an "on" stage
	g := &Gate{Control: NewMemoryControl(), Getenv: envOf(map[string]string{"LOOP_DRAFT": "on", "LOOP_PAUSED": "1"})}
	if err := g.Allow(ctx, StageDraft); !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "LOOP_PAUSED") {
		t.Fatalf("paused: %v", err)
	}
}

func TestTokenCapIgnoresBadValues(t *testing.T) {
	for v, want := range map[string]int{"": DefaultMonthlyTokenCap, "abc": DefaultMonthlyTokenCap, "0": DefaultMonthlyTokenCap,
		"-5": DefaultMonthlyTokenCap, "1.5": DefaultMonthlyTokenCap, "42": 42} {
		g := &Gate{Getenv: envOf(map[string]string{"LOOP_MONTHLY_TOKEN_CAP": v})}
		if got := g.TokenCap(); got != want {
			t.Errorf("cap %q = %d want %d", v, got, want)
		}
	}
}

func TestStoppedRecordsReason(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryControl()
	now := time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)
	g := &Gate{Control: c, Getenv: envOf(nil), Now: func() time.Time { return now }}
	stop := g.Allow(ctx, StagePublish)
	if err := g.Stopped(ctx, StagePublish, stop); err != stop {
		t.Fatalf("Stopped returned %v, want the original error", err)
	}
	evs, _ := c.ListAudit(ctx, AuditQuery{})
	if len(evs) != 1 {
		t.Fatalf("%+v", evs)
	}
	e := evs[0]
	// the reason is the code without the "loop stage stopped: " prefix
	if e.Stage != StagePublish || e.Action != "stopped" || e.Reason != "LOOP_PUBLISH is not on" || !e.At.Equal(now) || e.Actor != ActorLoop {
		t.Fatalf("%+v", e)
	}
	if !strings.HasPrefix(e.ID, "aud_20261112T030000_") {
		t.Fatalf("id %q", e.ID)
	}

	// the audit write fails: both errors come back
	c.Fail = errors.New("down")
	err := g.Stopped(ctx, StagePublish, stop)
	if !errors.Is(err, ErrStopped) || !errors.Is(err, c.Fail) {
		t.Fatalf("joined: %v", err)
	}
}

func TestRecordKeepsGivenFields(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryControl()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	g := &Gate{Control: c, Getenv: envOf(nil), Now: func() time.Time { t.Fatal("clock read although At was given"); return at }}
	if err := g.Record(ctx, Event{ID: "aud_fixed", At: at, Actor: "ko-tarou", Stage: StageDraft, Action: "approved"}); err != nil {
		t.Fatal(err)
	}
	evs, _ := c.ListAudit(ctx, AuditQuery{})
	if len(evs) != 1 || evs[0].ID != "aud_fixed" || evs[0].Actor != "ko-tarou" || !evs[0].ExpiresAt.Equal(at.Add(Retention)) {
		t.Fatalf("%+v", evs)
	}
}

func TestMemoryControlListAudit(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryControl()
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	// appended out of order
	for i, h := range []int{3, 1, 2, 5, 4} {
		e := Event{ID: string(rune('a' + i)), At: base.Add(time.Duration(h) * time.Hour), DraftID: "d1", ItemID: "i1"}
		if h%2 == 0 {
			e.DraftID, e.ItemID = "d2", "i2"
		}
		if err := c.AppendAudit(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	hours := func(evs []Event) []int {
		out := []int{}
		for _, e := range evs {
			out = append(out, int(e.At.Sub(base).Hours()))
		}
		return out
	}
	check := func(q AuditQuery, want ...int) {
		t.Helper()
		evs, err := c.ListAudit(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if got := hours(evs); !slices.Equal(got, want) {
			t.Fatalf("%+v: %v want %v", q, got, want)
		}
	}
	check(AuditQuery{}, 1, 2, 3, 4, 5) // oldest first
	check(AuditQuery{DraftID: "d1"}, 1, 3, 5)
	check(AuditQuery{ItemID: "i2"}, 2, 4)
	check(AuditQuery{DraftID: "d1", ItemID: "i2"})
	check(AuditQuery{Since: base.Add(3 * time.Hour)}, 3, 4, 5) // Since is inclusive
	check(AuditQuery{Limit: 2}, 4, 5)                          // the limit keeps the newest

	c.Fail = errors.New("down")
	if _, err := c.ListAudit(ctx, AuditQuery{}); err == nil {
		t.Fatal("ListAudit ignored Fail")
	}
	if err := c.SetSwitch(ctx, StageDraft, false, "x", base); err == nil {
		t.Fatal("SetSwitch ignored Fail")
	}
	if _, err := c.AddUsage(ctx, "2026-11", 1); err == nil {
		t.Fatal("AddUsage ignored Fail")
	}
	if _, err := c.Usage(ctx, "2026-11"); err == nil {
		t.Fatal("Usage ignored Fail")
	}
}

func TestMemoryControlSwitchesAreCopies(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryControl()
	_ = c.SetSwitch(ctx, StageTriage, false, "x", time.Now())
	sw, _ := c.Switches(ctx)
	sw[StageTriage] = true
	sw[SwitchAll] = true
	again, _ := c.Switches(ctx)
	if again[StageTriage] || len(again) != 1 {
		t.Fatalf("caller changed the switches: %v", again)
	}
}
