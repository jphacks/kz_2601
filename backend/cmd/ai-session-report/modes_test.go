package main

import (
	"io"
	"strings"
	"testing"
)

// withLogModesS7001 swaps the registry for the test and restores it after.
// Not parallel: logModes is a package-level map.
func withLogModesS7001(t *testing.T, names ...string) {
	t.Helper()
	saved := logModes
	logModes = map[string]logMode{}
	t.Cleanup(func() { logModes = saved })
	for _, n := range names {
		registerLogMode(n, logMode{summary: n, run: func([]string, io.Writer) error { return nil }})
	}
}

func TestLogModeNames_S7Backend001(t *testing.T) {
	cases := []struct {
		name  string
		modes []string
		want  string
	}{
		{"none", nil, ""},
		{"one", []string{"limit-hits"}, "limit-hits"},
		{"sorted by name", []string{"retry", "limit-hits", "chat-outcome"}, "chat-outcome, limit-hits, retry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withLogModesS7001(t, tc.modes...)
			if got := logModeNames(); got != tc.want {
				t.Fatalf("logModeNames() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The real registry (init of each mode file) shows up in the usage text.
func TestLogModeNames_S7Backend001_RealRegistry(t *testing.T) {
	got := logModeNames()
	for n := range logModes {
		if !strings.Contains(got, n) {
			t.Errorf("logModeNames() = %q, missing %q", got, n)
		}
	}
	if !strings.Contains(got, "limit-hits") {
		t.Errorf("logModeNames() = %q, want limit-hits listed", got)
	}
}
