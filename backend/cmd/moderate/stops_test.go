package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #346: the stop commands are dry runs unless -yes, need -env and -reason,
// refuse the production database without -allow-prod-db, and lift only
// what is active.
func TestRunStops(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()
	clock := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = func() time.Time { return store.Now() } })
	do := func(prod bool, args ...string) (string, error) {
		var b strings.Builder
		err := run(ctx, m, args, &b, prod)
		return b.String(), err
	}
	active := func(id string) bool {
		got, err := m.GetStops(ctx, "prod", []string{id})
		if err != nil {
			t.Fatal(err)
		}
		return got[id].Active(clock)
	}

	if _, err := do(false, "stop-app", "A1", "-reason", "x"); !errors.Is(err, errUsage) {
		t.Fatalf("-env must be required: %v", err)
	}
	if _, err := do(false, "stop-app", "A1", "-env", "prod"); err == nil {
		t.Fatal("-reason must be required")
	}
	if _, err := do(false, "stop-app", "A/1", "-env", "prod", "-reason", "x"); !errors.Is(err, errUsage) {
		t.Fatalf("bad appId: %v", err)
	}
	// production database: refused without -allow-prod-db, even a dry run
	if _, err := do(true, "stop-app", "A1", "-env", "prod", "-reason", "x"); err == nil || !strings.Contains(err.Error(), "-allow-prod-db") {
		t.Fatalf("prod db: %v", err)
	}
	// dry run
	if out, err := do(false, "stop-app", "A1", "-env", "prod", "-reason", "スパム"); err != nil || !strings.HasPrefix(out, "dry run") {
		t.Fatalf("dry run: %q %v", out, err)
	}
	if active(store.AppStopID("A1")) {
		t.Fatal("dry run wrote")
	}
	if out, err := do(true, "stop-app", "A1", "-env", "prod", "-reason", "スパム", "-yes", "-allow-prod-db"); err != nil || !strings.HasPrefix(out, "done") {
		t.Fatalf("stop-app: %q %v", out, err)
	}
	got, _ := m.GetStops(ctx, "prod", []string{store.AppStopID("A1")})
	if s := got[store.AppStopID("A1")]; !s.Active(clock) || s.By != store.StopByOperator || s.Reason != "スパム" || !s.CreatedAt.Equal(clock) {
		t.Fatalf("stop: %+v", s)
	}
	if _, err := do(false, "stop-rule", "A1", "r1", "-env", "prod", "-reason", "x", "-yes"); err != nil || !active(store.RuleStopID("A1", "r1")) {
		t.Fatalf("stop-rule: %v", err)
	}
	if _, err := do(false, "pause-notify", "A1", "-env", "prod", "-reason", "x", "-for", "1000h", "-yes"); err == nil {
		t.Fatal("-for over 30 days accepted")
	}
	if _, err := do(false, "pause-notify", "A1", "-env", "prod", "-reason", "x", "-for", "2h", "-yes"); err != nil || !active(store.NotifyStopID("A1")) {
		t.Fatalf("pause-notify: %v", err)
	}
	if out, err := do(false, "stops", "-env", "prod"); err != nil || strings.Count(out, "\tactive\t") != 3 {
		t.Fatalf("stops: %q %v", out, err)
	}
	// another env is separate
	if out, _ := do(false, "stops", "-env", "stg"); out != "" {
		t.Fatalf("stg stops: %q", out)
	}

	clock = clock.Add(time.Minute)
	if out, err := do(false, "resume-app", "A1", "-env", "prod"); err != nil || !strings.HasPrefix(out, "dry run") || !active(store.AppStopID("A1")) {
		t.Fatalf("resume dry run: %q %v", out, err)
	}
	for _, args := range [][]string{{"resume-app", "A1"}, {"resume-rule", "A1", "r1"}, {"resume-notify", "A1"}} {
		if _, err := do(false, append(args, "-env", "prod", "-yes")...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if active(store.AppStopID("A1")) || active(store.RuleStopID("A1", "r1")) || active(store.NotifyStopID("A1")) {
		t.Fatal("still active after resume")
	}
	if _, err := do(false, "resume-app", "A1", "-env", "prod", "-yes"); err == nil {
		t.Fatal("resuming a lifted stop must fail")
	}
	if out, _ := do(false, "stops", "-env", "prod"); strings.Count(out, "\tended\t") != 3 {
		t.Fatalf("lifted stops are kept as the record: %q", out)
	}
}
