package expreval

import (
	"testing"
	"time"
)

func TestPeriod(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name       string
		start, end any
		day        bool
		from, to   string // "" = not a period
	}{
		{"date range includes the end day", "2026-11-10", "2026-11-12", true, "2026-11-10T00:00:00+09:00", "2026-11-13T00:00:00+09:00"},
		{"one date", "2026-11-10", nil, true, "2026-11-10T00:00:00+09:00", "2026-11-11T00:00:00+09:00"},
		{"empty end is the start", "2026-11-10", "", true, "2026-11-10T00:00:00+09:00", "2026-11-11T00:00:00+09:00"},
		{"datetime range", "2026-11-10T10:00:00+09:00", "2026-11-10T11:00:00+09:00", false, "2026-11-10T10:00:00+09:00", "2026-11-10T11:00:00+09:00"},
		{"date field holding RFC 3339 is its day in loc", "2026-11-10T23:30:00Z", "2026-11-12T00:00:00+09:00", true, "2026-11-11T00:00:00+09:00", "2026-11-13T00:00:00+09:00"},
		{"datetime field holding a day is its first instant", "2026-11-10", nil, false, "2026-11-10T00:00:00+09:00", "2026-11-10T00:00:00.000000001+09:00"},
		{"end before start", "2026-11-10", "2026-11-09", true, "", ""},
		{"same-day datetimes in a date field", "2026-11-10T18:00:00+09:00", "2026-11-10T09:00:00+09:00", true, "2026-11-10T00:00:00+09:00", "2026-11-11T00:00:00+09:00"},
		{"empty start", "", "2026-11-09", true, "", ""},
		{"malformed end", "2026-11-10", "soon", true, "", ""},
		{"not a string", float64(3), nil, true, "", ""},
	}
	for _, c := range cases {
		from, to, ok := Period(c.start, c.end, c.day, jst)
		if c.from == "" {
			if ok {
				t.Errorf("%s: got a period %v..%v", c.name, from, to)
			}
			continue
		}
		if !ok || !from.Equal(at(c.from)) || !to.Equal(at(c.to)) {
			t.Errorf("%s: %v..%v ok=%v", c.name, from, to, ok)
		}
	}
	// a datetime without an end is the instant: it overlaps a range that holds it
	from, to, _ := Period("2026-11-10T10:00:00+09:00", nil, false, jst)
	rf, rt, _ := Period("2026-11-10T10:00:00+09:00", "2026-11-10T11:00:00+09:00", false, jst)
	if !Overlaps(from, to, rf, rt) {
		t.Error("instant at the start of a range")
	}
	// back-to-back periods do not overlap
	nf, nt, _ := Period("2026-11-10T11:00:00+09:00", "2026-11-10T12:00:00+09:00", false, jst)
	if Overlaps(rf, rt, nf, nt) {
		t.Error("back-to-back datetimes overlap")
	}
	// consecutive date ranges do not overlap, the shared end day does
	af, at2, _ := Period("2026-11-10", "2026-11-12", true, jst)
	bf, bt, _ := Period("2026-11-13", "2026-11-14", true, jst)
	cf, ct, _ := Period("2026-11-12", "2026-11-14", true, jst)
	if Overlaps(af, at2, bf, bt) || !Overlaps(af, at2, cf, ct) {
		t.Error("date ranges")
	}
}

// An end that is set but not a date string is malformed: not a period.
func TestPeriodNonStringEnd(t *testing.T) {
	for _, end := range []any{float64(3), true, map[string]any{}} {
		if from, to, ok := Period("2026-11-10", end, true, time.UTC); ok {
			t.Errorf("end %v: got a period %v..%v", end, from, to)
		}
	}
}
