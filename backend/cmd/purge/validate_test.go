package main

import (
	"testing"
	"time"
)

func TestParseFlagsRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"older-than below the floor", []string{"-older-than", "23h"}},
		{"older-than zero", []string{"-older-than", "0s"}},
		{"older-than negative", []string{"-older-than", "-1h"}},
		{"feedback-older-than below the floor", []string{"-feedback-older-than", "1h"}},
		{"feedback-older-than negative", []string{"-feedback-older-than", "-720h"}},
		{"limit zero", []string{"-limit", "0"}},
		{"limit negative", []string{"-limit", "-1"}},
		{"feedback-limit zero", []string{"-feedback-limit", "0"}},
		{"feedback-limit negative", []string{"-feedback-limit", "-500"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseFlags(newFlagSet3280(), tc.args); err == nil {
				t.Errorf("%q: no error", tc.args)
			}
		})
	}
}

func TestParseFlagsAcceptsTheFloor(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"older-than exactly the floor", []string{"-older-than", "24h"}},
		{"feedback-older-than exactly the floor", []string{"-feedback-older-than", "24h"}},
		{"limit one", []string{"-limit", "1"}},
		{"feedback-limit one", []string{"-feedback-limit", "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseFlags(newFlagSet3280(), tc.args); err != nil {
				t.Errorf("%q: %v", tc.args, err)
			}
		})
	}
}

func TestMinRetentionIsTheFloor(t *testing.T) {
	if minRetention != 24*time.Hour {
		t.Fatalf("minRetention = %s", minRetention)
	}
}
