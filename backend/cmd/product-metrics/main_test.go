package main

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/weekly.golden.md")

// testdata/logs.json: 3 users over 3 weeks; the numbers in the golden file
// are worked out by hand in docs/metrics/product-metrics.md (fixture).
func TestWeeklyGolden(t *testing.T) {
	f, err := os.Open("testdata/logs.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	evs, err := parse(f)
	if err != nil {
		t.Fatal(err)
	}
	got := render(aggregate(evs))
	if *update {
		if err := os.WriteFile("testdata/weekly.golden.md", []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/weekly.golden.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("output differs from golden (go test ./cmd/product-metrics -update):\n%s", got)
	}
}

// Bare slog JSON lines (local `make dev`) parse the same; non-JSON and
// anonymous lines are skipped.
func TestParseJSONLines(t *testing.T) {
	in := `2026/11/02 listening on :8080
{"time":"2026-11-02T00:00:00Z","level":"INFO","msg":"session.start","event":"session.start","uid_hash":"u1"}
{"time":"2026-11-02T00:01:00Z","level":"INFO","msg":"ai.generate","event":"ai.generate","mode":"create","outcome":"validated","uid_hash":"u1","app_hash":"a1"}
{"time":"2026-11-02T00:02:00Z","level":"INFO","msg":"ai.generate","event":"ai.generate","mode":"create","outcome":"validated"}
{"time":"2026-11-02T00:03:00Z","level":"INFO","msg":"appconfig: request","path":"/v1/apps"}
`
	evs, err := parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[1].Event != "ai.generate" || evs[1].App != "a1" || evs[1].UID != "u1" {
		t.Fatalf("events: %+v", evs)
	}
	r := aggregate(evs)
	if n := get(r.New, week(evs[0].At)); n.Users != 1 || n.Generated != 1 || n.Minutes[0] != 1 {
		t.Fatalf("new: %+v", n)
	}
}

// Weeks and days are cut in JST: Sunday 23:59 JST is the previous week,
// Monday 00:00 JST (Sunday 15:00 UTC) the next.
func TestWeekAndDayAreJST(t *testing.T) {
	sun := time.Date(2026, 11, 1, 14, 59, 0, 0, time.UTC) // Sun 23:59 JST
	mon := time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)  // Mon 00:00 JST
	if got := week(sun).Format(time.DateOnly); got != "2026-10-26" {
		t.Errorf("week(sun) = %s", got)
	}
	if got := week(mon).Format(time.DateOnly); got != "2026-11-02" {
		t.Errorf("week(mon) = %s", got)
	}
	if day(mon).Sub(day(sun)) != 24*time.Hour {
		t.Errorf("day boundary not at JST midnight")
	}
}

// D1 / D7 count only once that day has data; a later day does not count.
func TestRetentionDue(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	evs := []event{
		{At: at("2026-10-05T01:00:00Z"), Event: "ai.generate", UID: "u", Mode: "create", Outcome: "validated"},
		{At: at("2026-10-07T01:00:00Z"), Event: "app.opened", UID: "u"}, // day 2: neither D1 nor D7
		{At: at("2026-10-08T01:00:00Z"), Event: "session.start", UID: "other"},
	}
	x := get(aggregate(evs).Ret, week(evs[0].At))
	if x.Users != 1 || x.D1Due != 1 || x.D1 != 0 || x.D7Due != 0 {
		t.Fatalf("retention: %+v", x)
	}
}

func TestEmptyInput(t *testing.T) {
	if got := render(aggregate(nil)); !strings.Contains(got, "ありません") {
		t.Fatalf("empty: %s", got)
	}
}
