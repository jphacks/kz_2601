package loop

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestGateDefaultsToStopped(t *testing.T) {
	ctx := context.Background()
	g := &Gate{Control: NewMemoryControl(), Getenv: envOf(nil)}
	for _, s := range Stages {
		if err := g.Allow(ctx, s); !errors.Is(err, ErrStopped) {
			t.Fatalf("%s runs with no env: %v", s, err)
		}
	}
}

func TestGateSwitches(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryControl()
	env := map[string]string{"LOOP_TRIAGE": "on", "LOOP_DRAFT": "on"}
	g := &Gate{Control: c, Getenv: envOf(env)}
	if err := g.Allow(ctx, StageTriage); err != nil {
		t.Fatal(err)
	}
	// a Firestore switch stops one stage without a deploy
	_ = c.SetSwitch(ctx, StageTriage, false, "ko-tarou", time.Now())
	if err := g.Allow(ctx, StageTriage); !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "switches.triage") {
		t.Fatalf("stage switch ignored: %v", err)
	}
	if err := g.Allow(ctx, StageDraft); err != nil {
		t.Fatalf("other stage stopped: %v", err)
	}
	// and all of them
	_ = c.SetSwitch(ctx, SwitchAll, false, "ko-tarou", time.Now())
	if err := g.Allow(ctx, StageDraft); !errors.Is(err, ErrStopped) {
		t.Fatalf("all switch ignored: %v", err)
	}
	_ = c.SetSwitch(ctx, SwitchAll, true, "ko-tarou", time.Now())
	_ = c.SetSwitch(ctx, StageTriage, true, "ko-tarou", time.Now())
	if err := g.Allow(ctx, StageTriage); err != nil {
		t.Fatal(err)
	}
	env["LOOP_PAUSED"] = "1"
	if err := g.Allow(ctx, StageTriage); !errors.Is(err, ErrStopped) {
		t.Fatal("LOOP_PAUSED ignored")
	}
	delete(env, "LOOP_PAUSED")
	c.Fail = errors.New("unavailable")
	if err := g.Allow(ctx, StageTriage); !errors.Is(err, ErrStopped) {
		t.Fatal("unreadable switches did not stop")
	}
}

func TestGateTokenCap(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryControl()
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	g := &Gate{Control: c, Getenv: envOf(map[string]string{"LOOP_TRIAGE": "on", "LOOP_MONTHLY_TOKEN_CAP": "1000"}), Now: func() time.Time { return now }}
	if err := g.AllowLLM(ctx); err != nil {
		t.Fatal(err)
	}
	_, _ = c.AddUsage(ctx, Month(now), 1000)
	if err := g.AllowLLM(ctx); !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "token cap") {
		t.Fatalf("cap ignored: %v", err)
	}
	// next month starts fresh
	now = now.AddDate(0, 1, 0)
	if err := g.AllowLLM(ctx); err != nil {
		t.Fatal(err)
	}
	if (&Gate{Getenv: envOf(nil)}).TokenCap() != DefaultMonthlyTokenCap {
		t.Fatal("default cap")
	}
}

func TestRecord(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryControl()
	g := &Gate{Control: c, Getenv: envOf(nil)}
	if err := g.Record(ctx, Event{Stage: StageIntake, Action: "ingested", ItemID: "lin_1", FeedbackID: "fb_1"}); err != nil {
		t.Fatal(err)
	}
	evs, _ := c.ListAudit(ctx, AuditQuery{ItemID: "lin_1"})
	if len(evs) != 1 || evs[0].Actor != ActorLoop || evs[0].ID == "" || evs[0].ExpiresAt.Sub(evs[0].At) != Retention {
		t.Fatalf("%+v", evs)
	}
	c.Fail = errors.New("down")
	if err := g.Record(ctx, Event{Stage: StageIntake, Action: "x"}); err == nil {
		t.Fatal("failed audit write swallowed")
	}
}

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestoreLoopControl(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	cl, err := firestore.NewClient(ctx, "demo-kz2601")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cl.Close() }()
	f := NewFirestore(cl)
	sfx := time.Now().Format("150405.000000")
	g := &Gate{Control: f, Getenv: envOf(map[string]string{"LOOP_NOTIFY": "on"})}
	if err := g.Record(ctx, Event{Stage: StageNotify, Action: "noticed", DraftID: "drf_" + sfx}); err != nil {
		t.Fatal(err)
	}
	evs, err := f.ListAudit(ctx, AuditQuery{DraftID: "drf_" + sfx})
	if err != nil || len(evs) != 1 || evs[0].Action != "noticed" {
		t.Fatalf("%+v %v", evs, err)
	}
	month := "t" + sfx
	if n, err := f.AddUsage(ctx, month, 5); err != nil || n != 5 {
		t.Fatalf("%d %v", n, err)
	}
	if n, err := f.AddUsage(ctx, month, 7); err != nil || n != 12 {
		t.Fatalf("%d %v", n, err)
	}
	if err := f.SetSwitch(ctx, StageNotify, false, "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := g.Allow(ctx, StageNotify); !errors.Is(err, ErrStopped) {
		t.Fatalf("firestore switch ignored: %v", err)
	}
	if err := f.SetSwitch(ctx, StageNotify, true, "test", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := g.Allow(ctx, StageNotify); err != nil {
		t.Fatal(err)
	}
}
