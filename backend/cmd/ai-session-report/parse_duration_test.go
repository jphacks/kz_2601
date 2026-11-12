package main

import (
	"testing"
	"time"
)

// TestParseDuration015: a JSON number is nanoseconds, a string is a Go
// duration or a number, anything else is 0.
func TestParseDuration015(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want time.Duration
	}{
		{"number ns", float64(1500000000), 1500 * time.Millisecond},
		{"zero number", float64(0), 0},
		{"string duration", "1.2s", 1200 * time.Millisecond},
		{"string ms", "350ms", 350 * time.Millisecond},
		{"string number", "2000", 2000 * time.Nanosecond},
		{"empty string", "", 0},
		{"bad string", "fast", 0},
		{"nil", nil, 0},
		{"bool", true, 0},
		{"int (not from JSON)", 5, 0},
		{"object", map[string]any{"s": 1}, 0},
	}
	for _, tc := range cases {
		if got := parseDuration(tc.in); got != tc.want {
			t.Errorf("%s: parseDuration(%v) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}
