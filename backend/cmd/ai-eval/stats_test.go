package main

import "testing"

func TestPercentile(t *testing.T) {
	cases := []struct {
		name string
		v    []int64
		p    float64
		want int64
	}{
		{"empty", nil, 0.5, 0},
		{"one", []int64{7}, 0.9, 7},
		{"p0", []int64{30, 10, 20}, 0, 10},
		{"p50 odd", []int64{30, 10, 20}, 0.5, 20},
		{"p50 even rounds up", []int64{40, 10, 30, 20}, 0.5, 30},
		{"p90", []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}, 0.9, 90},
		{"p100", []int64{30, 10, 20}, 1, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := percentile(c.v, c.p); got != c.want {
				t.Errorf("percentile(%v, %v) = %d, want %d", c.v, c.p, got, c.want)
			}
		})
	}
}

func TestPercentileKeepsInput(t *testing.T) {
	v := []int64{3, 1, 2}
	percentile(v, 0.5)
	if v[0] != 3 || v[1] != 1 || v[2] != 2 {
		t.Errorf("input reordered: %v", v)
	}
}

func TestMedianOf(t *testing.T) {
	ms := func(r Record) int64 { return r.ClaudeMs }
	cases := []struct {
		name string
		recs []Record
		want int64
	}{
		{"empty", nil, 0},
		{"only failed", []Record{{OK: false, ClaudeMs: 5}}, 0},
		{"odd", []Record{{OK: true, ClaudeMs: 300}, {OK: true, ClaudeMs: 100}, {OK: true, ClaudeMs: 200}}, 200},
		{"even", []Record{{OK: true, ClaudeMs: 100}, {OK: true, ClaudeMs: 200}, {OK: true, ClaudeMs: 300}, {OK: true, ClaudeMs: 400}}, 300},
		{"skips failed", []Record{{OK: true, ClaudeMs: 100}, {OK: false, ClaudeMs: 9999}, {OK: true, ClaudeMs: 200}, {OK: true, ClaudeMs: 300}}, 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := medianOf(c.recs, ms); got != c.want {
				t.Errorf("medianOf = %d, want %d", got, c.want)
			}
		})
	}
}

func TestOkCount(t *testing.T) {
	recs := []Record{
		{OK: true},                     // first pass
		{OK: true, Regenerations: 1},   // regenerated
		{OK: true, AreaFallback: true}, // area fallback
		{OK: false},                    // failed
		{OK: false, Regenerations: 2},  // failed after retries
		{OK: true},                     // first pass
	}
	ok, first := okCount(recs)
	if ok != 4 || first != 2 {
		t.Errorf("okCount = (%d, %d), want (4, 2)", ok, first)
	}
	if ok, first := okCount(nil); ok != 0 || first != 0 {
		t.Errorf("okCount(nil) = (%d, %d), want (0, 0)", ok, first)
	}
}

func TestPct(t *testing.T) {
	cases := []struct {
		a, b int64
		want string
	}{
		{0, 100, "-"},
		{0, 0, "-"},
		{100, 150, "+50%"},
		{200, 100, "-50%"},
		{100, 100, "+0%"},
		{3, 4, "+33%"},
	}
	for _, c := range cases {
		if got := pct(c.a, c.b); got != c.want {
			t.Errorf("pct(%d, %d) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}
