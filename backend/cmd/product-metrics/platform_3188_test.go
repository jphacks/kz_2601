package main

import (
	"os"
	"strings"
	"testing"
)

// -by-platform: OS from the line's clientPlatform or the http.request line of
// the same requestId; bad_request is not counted; the default output has no
// OS table.
func TestWeeklyByPlatform_3188(t *testing.T) {
	f, err := os.Open("testdata/logs_by_platform.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	evs, err := parse(f)
	if err != nil {
		t.Fatal(err)
	}
	got := renderByPlatform(evs)
	for _, want := range []string{
		"| 2026-10-19 | linux | 1 | 0.0% | 100.0% | 0.0% |",
		"| 2026-10-19 | macos | 1 | 100.0% | 0.0% | 0.0% |",
		"| 2026-10-19 | other | 1 | 100.0% | 0.0% | 0.0% |",
		"| 2026-10-19 | unknown | 1 | 100.0% | 0.0% | 0.0% |",
		"| 2026-10-19 | windows | 2 | 50.0% | 0.0% | 50.0% |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "| ios |") {
		t.Errorf("bad_request counted:\n%s", got)
	}
	if strings.Contains(render(aggregate(evs)), "OS ごと") {
		t.Errorf("default output has an OS table")
	}
	if got := renderByPlatform(nil); !strings.Contains(got, "行が無い") {
		t.Errorf("empty: %s", got)
	}
}
