package store

import (
	"testing"
	"time"
)

// toStopDoc → toStop keeps a Stop (times in UTC); no emulator needed.
func TestStopDocRoundTrip(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	created := time.Date(2026, 11, 6, 23, 0, 0, 123000, jst)
	cases := []struct {
		name string
		in   Stop
	}{
		{"app until lifted", Stop{Level: StopApp, AppID: "app-a", By: StopByOperator, Operator: "ops@example.com",
			Reason: "spam", CreatedAt: created, Force: true}},
		{"rule with until", Stop{Level: StopRule, AppID: "app-a", RuleID: "r1", By: StopByAuto,
			Until: created.Add(24 * time.Hour), CreatedAt: created}},
		{"lifted", Stop{Level: StopRule, AppID: "app-b", RuleID: "r2", By: StopByOperator, Operator: "ops",
			Until: created.Add(time.Hour), CreatedAt: created, LiftedAt: created.Add(30 * time.Minute)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := toStopDoc(c.in)
			for name, tm := range map[string]time.Time{"until": d.Until, "createdAt": d.CreatedAt, "liftedAt": d.LiftedAt} {
				if tm.Location() != time.UTC {
					t.Errorf("%s is stored in %v, want UTC", name, tm.Location())
				}
			}
			got := d.toStop("stop-1")
			want := c.in
			want.ID = "stop-1"
			if !sameStop(got, want) {
				t.Fatalf("round trip\n got %+v\nwant %+v", got, want)
			}
			if c.in.Until.IsZero() != got.Until.IsZero() || c.in.LiftedAt.IsZero() != got.LiftedAt.IsZero() {
				t.Fatalf("zero times changed: %+v", got)
			}
		})
	}
	t.Run("missing createdAt is now", func(t *testing.T) {
		before := Now()
		d := toStopDoc(Stop{Level: StopApp, AppID: "a", By: StopByAuto})
		got := d.toStop("x")
		if got.CreatedAt.Before(before) || got.CreatedAt.After(Now()) {
			t.Fatalf("createdAt = %v, want now (%v)", got.CreatedAt, before)
		}
		if !got.Until.IsZero() || !got.LiftedAt.IsZero() {
			t.Fatalf("until / liftedAt = %v / %v, want zero", got.Until, got.LiftedAt)
		}
	})
}

func sameStop(a, b Stop) bool {
	ta := [3]time.Time{a.Until, a.CreatedAt, a.LiftedAt}
	tb := [3]time.Time{b.Until, b.CreatedAt, b.LiftedAt}
	for i := range ta {
		if !ta[i].Equal(tb[i]) {
			return false
		}
	}
	a.Until, a.CreatedAt, a.LiftedAt = time.Time{}, time.Time{}, time.Time{}
	b.Until, b.CreatedAt, b.LiftedAt = time.Time{}, time.Time{}, time.Time{}
	return a == b
}
