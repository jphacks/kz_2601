package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMode(t *testing.T) {
	for apply, want := range map[bool]string{false: "dry-run", true: "APPLY"} {
		if got := mode(apply); got != want {
			t.Errorf("mode(%v) = %q, want %q", apply, got, want)
		}
	}
}

// A failure still prints the counts so far, then the error code (not its text), and exits 1.
func TestRunErrorKeepsCounts(t *testing.T) {
	for _, apply := range []bool{false, true} {
		f := &fakeStore{err: errors.New("deadline")}
		var out bytes.Buffer
		if code := run(context.Background(), f, apply, &out); code != 1 {
			t.Fatalf("apply=%v: exit %d", apply, code)
		}
		s := out.String()
		if !strings.Contains(s, "matching apps: 2") || !strings.Contains(s, "ERROR: unknown") {
			t.Errorf("apply=%v: output:\n%s", apply, s)
		}
		if strings.Contains(s, "deadline") {
			t.Errorf("apply=%v: raw error text leaked:\n%s", apply, s)
		}
		if f.dryRun[0] == apply {
			t.Errorf("apply=%v: dryRun=%v", apply, f.dryRun[0])
		}
	}
}
