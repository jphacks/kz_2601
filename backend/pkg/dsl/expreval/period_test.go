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
		from, to   string // "" = not a period
	}{
		{"date range includes the end day", "2026-11-10", "2026-11-12", "2026-11-10T00:00:00+09:00", "2026-11-13T00:00:00+09:00"},
		{"one date", "2026-11-10", nil, "2026-11-10T00:00:00+09:00", "2026-11-11T00:00:00+09:00"},
		{"empty end is the start", "2026-11-10", "", "2026-11-10T00:00:00+09:00", "2026-11-11T00:00:00+09:00"},
		{"datetime range", "2026-11-10T10:00:00+09:00", "2026-11-10T11:00:00+09:00", "2026-11-10T10:00:00+09:00", "2026-11-10T11:00:00+09:00"},
		{"end before start", "2026-11-10", "2026-11-09", "", ""},
		{"empty start", "", "2026-11-09", "", ""},
		{"malformed end", "2026-11-10", "soon", "", ""},
		{"not a string", float64(3), nil, "", ""},
	}
	for _, c := range cases {
		from, to, ok := Period(c.start, c.end, jst)
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
	from, to, _ := Period("2026-11-10T10:00:00+09:00", nil, jst)
	rf, rt, _ := Period("2026-11-10T10:00:00+09:00", "2026-11-10T11:00:00+09:00", jst)
	if !Overlaps(from, to, rf, rt) {
		t.Error("instant at the start of a range")
	}
	// back-to-back periods do not overlap
	nf, nt, _ := Period("2026-11-10T11:00:00+09:00", "2026-11-10T12:00:00+09:00", jst)
	if Overlaps(rf, rt, nf, nt) {
		t.Error("back-to-back datetimes overlap")
	}
	// consecutive date ranges do not overlap, the shared end day does
	af, at2, _ := Period("2026-11-10", "2026-11-12", jst)
	bf, bt, _ := Period("2026-11-13", "2026-11-14", jst)
	cf, ct, _ := Period("2026-11-12", "2026-11-14", jst)
	if Overlaps(af, at2, bf, bt) || !Overlaps(af, at2, cf, ct) {
		t.Error("date ranges")
	}
}
