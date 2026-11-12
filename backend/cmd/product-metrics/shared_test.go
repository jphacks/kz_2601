package main

import (
	"os"
	"strings"
	"testing"
)

// testdata/shared.json: 2 weeks; a1 has u1 and u2, a2 has u3. Lines without
// a uid_hash, with an unknown kind, or of another event are not counted.
// Numbers worked out by hand below.
func TestSharedReport(t *testing.T) {
	f, err := os.Open("testdata/shared.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	us, err := parseShared(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(us) != 6 {
		t.Fatalf("lines %d: %+v", len(us), us)
	}
	out := renderShared(aggregateShared(us))
	for _, want := range []string{
		// week 1: read by u1 10, u2 30, u3 4 -> p50 10 (nearest rank), p90 30
		"| 2026-11-02 | 3 | read | 3 | 44 | 10 | 30 | 30 |",
		"| 2026-11-02 | 3 | write | 1 | 1 | 1 | 1 | 1 |",
		"| 2026-11-02 | 3 | notify | 1 | 1 | 1 | 1 | 1 |",
		"| 2026-11-09 | 1 | read | 1 | 5 | 5 | 5 | 5 |",
		"| 2026-11-09 | 1 | write | 0 | 0 | — | — | — |",
		// by size: a1 (2 people), a2 (1 person); week 2 only a1 with u1
		"| 2026-11-02 | 1 | 1 | 4 | 0 | 0 |",
		"| 2026-11-02 | 2-5 | 1 | 40 | 1 | 1 |",
		"| 2026-11-09 | 1 | 1 | 5 | 0 | 0 |",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "99") {
		t.Fatalf("skipped lines counted:\n%s", out)
	}
	// the product report ignores shared.usage
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	evs, err := parse(f)
	if err != nil || len(evs) != 1 || evs[0].Event != "ai.generate" {
		t.Fatalf("product events %+v %v", evs, err)
	}
}

func TestSharedReportEmpty(t *testing.T) {
	if out := renderShared(aggregateShared(nil)); !strings.Contains(out, "SHARED_USAGE_LOG=on") {
		t.Fatal(out)
	}
}
