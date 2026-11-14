package main

import (
	"io"
	"sort"
	"strings"
	"testing"
)

// withLogModes swaps the registry for the test and restores it afterwards.
func withLogModes(t *testing.T, m map[string]logMode) {
	t.Helper()
	old := logModes
	logModes = m
	t.Cleanup(func() { logModes = old })
}

func TestLogModeNames_3270(t *testing.T) {
	noop := logMode{summary: "x", run: func([]string, io.Writer) error { return nil }}

	t.Run("empty registry gives an empty string", func(t *testing.T) {
		withLogModes(t, map[string]logMode{})
		if got := logModeNames(); got != "" {
			t.Errorf("logModeNames() = %q, want empty", got)
		}
	})

	t.Run("names are sorted and comma separated", func(t *testing.T) {
		withLogModes(t, map[string]logMode{})
		for _, n := range []string{"zeta", "alpha", "mid"} {
			registerLogMode(n, noop)
		}
		if got, want := logModeNames(), "alpha, mid, zeta"; got != want {
			t.Errorf("logModeNames() = %q, want %q", got, want)
		}
	})

	t.Run("single mode has no separator", func(t *testing.T) {
		withLogModes(t, map[string]logMode{})
		registerLogMode("only", noop)
		if got := logModeNames(); got != "only" {
			t.Errorf("logModeNames() = %q, want only", got)
		}
	})

	t.Run("registered modes appear in the usage line", func(t *testing.T) {
		withLogModes(t, map[string]logMode{})
		registerLogMode("b-mode", noop)
		registerLogMode("a-mode", noop)
		usage := "ai-session-report: unknown mode \"x\" (log modes: " + logModeNames() + ")"
		if !strings.Contains(usage, "(log modes: a-mode, b-mode)") {
			t.Errorf("usage = %q", usage)
		}
	})
}

func TestLogModeNamesRealRegistry_3270(t *testing.T) {
	// The real registry (filled by each mode's init) is non-empty and sorted.
	got := logModeNames()
	if got == "" {
		t.Fatal("no log modes registered")
	}
	names := strings.Split(got, ", ")
	if !sort.StringsAreSorted(names) {
		t.Errorf("not sorted: %v", names)
	}
	if len(names) != len(logModes) {
		t.Errorf("got %d names, registry has %d", len(names), len(logModes))
	}
	for _, n := range names {
		if _, ok := logModes[n]; !ok {
			t.Errorf("%q is not registered", n)
		}
	}
}
