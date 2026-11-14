package main

import (
	"testing"
	"time"
)

func TestInboxCreatedAt_3195(t *testing.T) {
	tokyo := time.FixedZone("Asia/Tokyo", 9*3600)
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"zero is dash", time.Time{}, "-"},
		{"utc", time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC), "2026-11-09T12:00:00Z"},
		{"tokyo is normalized to utc", time.Date(2026, 11, 9, 21, 30, 0, 0, tokyo), "2026-11-09T12:30:00Z"},
		{"tokyo crossing midnight", time.Date(2026, 11, 10, 1, 0, 0, 0, tokyo), "2026-11-09T16:00:00Z"},
		{"sub-second is truncated", time.Date(2026, 11, 9, 12, 0, 0, 500_000_000, time.UTC), "2026-11-09T12:00:00Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := createdAt(c.in); got != c.want {
				t.Fatalf("createdAt(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
