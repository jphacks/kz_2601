package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #346: stop-app -for pauses until a deadline and -force refuses the
// rule-triggering writes; stop-all / resume-all is the environment-wide
// stop; stop-log shows who did what; an operator name is required.
func TestKillSwitchCommands(t *testing.T) {
	t.Setenv("USER", "")
	ctx := context.Background()
	m := store.NewMemory()
	clock := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = func() time.Time { return store.Now() } })
	do := func(args ...string) (string, error) {
		var b strings.Builder
		err := run(ctx, m, args, &b, false)
		return b.String(), err
	}
	get := func(id string) store.Stop {
		got, err := m.GetStops(ctx, "stg", []string{id})
		if err != nil {
			t.Fatal(err)
		}
		return got[id]
	}

	if _, err := do("stop-app", "A1", "-env", "stg", "-reason", "x", "-yes"); err == nil || !strings.Contains(err.Error(), "-operator") {
		t.Fatalf("operator must be required: %v", err)
	}
	if _, err := do("stop-app", "A1", "-env", "stg", "-reason", "x", "-for", "10s", "-operator", "alice", "-yes"); err == nil {
		t.Fatal("-for under a minute accepted")
	}
	out, err := do("stop-app", "A1", "-env", "stg", "-reason", "スパム", "-for", "24h", "-force", "-operator", "alice", "-yes")
	if err != nil || !strings.Contains(out, "for 24h0m0s") || !strings.Contains(out, "refusing") {
		t.Fatalf("stop-app -for -force: %q %v", out, err)
	}
	if s := get(store.AppStopID("A1")); !s.Force || s.Operator != "alice" || !s.Until.Equal(clock.Add(24*time.Hour)) || !s.Active(clock) || s.Active(clock.Add(25*time.Hour)) {
		t.Fatalf("paused stop: %+v", s)
	}
	// without -for: until resumed, not forced
	if _, err := do("stop-app", "A2", "-env", "stg", "-reason", "x", "-operator", "alice", "-yes"); err != nil {
		t.Fatal(err)
	}
	if s := get(store.AppStopID("A2")); s.Force || !s.Until.IsZero() {
		t.Fatalf("plain stop: %+v", s)
	}

	if _, err := do("stop-all", "extra", "-env", "stg", "-reason", "x", "-operator", "alice"); !errors.Is(err, errUsage) {
		t.Fatalf("stop-all takes no id: %v", err)
	}
	if _, err := do("stop-all", "-env", "stg", "-reason", "障害", "-force", "-operator", "alice", "-yes"); err != nil {
		t.Fatal(err)
	}
	if s := get(store.GlobalStopID); s.Level != store.StopGlobal || !s.Force || !s.Active(clock) {
		t.Fatalf("global stop: %+v", s)
	}
	clock = clock.Add(time.Minute)
	if _, err := do("resume-all", "-env", "stg", "-yes"); err == nil {
		t.Fatal("resume without an operator accepted")
	}
	if _, err := do("resume-all", "-env", "stg", "-operator", "bob", "-yes"); err != nil || get(store.GlobalStopID).Active(clock) {
		t.Fatalf("resume-all: %v", err)
	}
	if out, _ := do("stops", "-env", "stg"); !strings.Contains(out, "\tforce\n") {
		t.Fatalf("stops shows force: %q", out)
	}

	out, err = do("stop-log", "-env", "stg")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err != nil || len(lines) != 4 || !strings.Contains(lines[0], "\tlift\tglobal\t") || !strings.Contains(lines[0], "operator=bob") ||
		!strings.Contains(lines[3], "\tstop\tapp\tA1\t") || !strings.Contains(lines[3], "operator=alice\tforce=true") || !strings.HasSuffix(lines[3], "スパム") {
		t.Fatalf("stop-log: %q %v", out, err)
	}
	if out, _ := do("stop-log", "A2", "-env", "stg"); strings.Count(out, "\n") != 1 || !strings.Contains(out, "\tA2\t") {
		t.Fatalf("stop-log A2: %q", out)
	}
}
