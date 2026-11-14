package schedule

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestNext(t *testing.T) {
	tokyo := mustLoc(t, "Asia/Tokyo")
	ny := mustLoc(t, "America/New_York")
	utc := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	daily := func(hm string) Spec { return Spec{Every: Daily, Time: hm} }
	weekly := func(wd, hm string) Spec { return Spec{Every: Weekly, Weekday: wd, Time: hm} }
	monthly := func(d int, hm string) Spec { return Spec{Every: Monthly, Day: d, Time: hm} }

	tests := []struct {
		name  string
		spec  Spec
		after string
		loc   *time.Location
		want  string
	}{
		// Tokyo, no DST today.
		{"tokyo daily later today", daily("08:00"), "2026-11-05T22:00:00Z", tokyo, "2026-11-05T23:00:00Z"},
		{"tokyo daily passed today", daily("08:00"), "2026-11-05T23:30:00Z", tokyo, "2026-11-06T23:00:00Z"},
		{"exactly at the run time goes to next", daily("08:00"), "2026-11-05T23:00:00Z", tokyo, "2026-11-06T23:00:00Z"},
		{"one ns before the run time", daily("08:00"), "2026-11-05T22:59:59.999999999Z", tokyo, "2026-11-05T23:00:00Z"},
		{"tokyo midnight crosses UTC date", daily("00:00"), "2026-11-05T14:00:00Z", tokyo, "2026-11-05T15:00:00Z"},

		// Tokyo historical DST (1949): 04-03 00:00-00:59 does not exist,
		// 09-11 00:00-00:59 occurs twice (JDT then JST).
		{"tokyo spring gap runs at the jump", daily("00:30"), "1949-04-02T12:00:00Z", tokyo, "1949-04-02T15:00:00Z"},
		{"tokyo fall overlap first occurrence", daily("00:30"), "1949-09-10T12:00:00Z", tokyo, "1949-09-10T14:30:00Z"},
		{"tokyo fall overlap runs once", daily("00:30"), "1949-09-10T14:30:00Z", tokyo, "1949-09-11T15:30:00Z"},

		// New York 2026: 03-08 02:00->03:00, 11-01 02:00->01:00.
		{"ny spring gap runs at the jump", daily("02:30"), "2026-03-08T05:00:00Z", ny, "2026-03-08T07:00:00Z"},
		{"ny spring before gap unaffected", daily("01:30"), "2026-03-08T05:00:00Z", ny, "2026-03-08T06:30:00Z"},
		{"ny spring after gap is EDT", daily("08:00"), "2026-03-08T05:00:00Z", ny, "2026-03-08T12:00:00Z"},
		{"ny day before spring is EST", daily("08:00"), "2026-03-07T05:00:00Z", ny, "2026-03-07T13:00:00Z"},
		{"ny fall overlap first occurrence", daily("01:30"), "2026-11-01T04:00:00Z", ny, "2026-11-01T05:30:00Z"},
		{"ny fall overlap runs once", daily("01:30"), "2026-11-01T05:30:00Z", ny, "2026-11-02T06:30:00Z"},
		{"ny fall between occurrences", daily("01:30"), "2026-11-01T06:00:00Z", ny, "2026-11-02T06:30:00Z"},
		{"ny fall after overlap is EST", daily("08:00"), "2026-11-01T04:00:00Z", ny, "2026-11-01T13:00:00Z"},
		{"ny weekly on spring sunday gap", weekly("sun", "02:30"), "2026-03-01T08:00:00Z", ny, "2026-03-08T07:00:00Z"},

		// Weekly, week boundary. 2026-11-06 is a Friday.
		{"weekly later this week", weekly("sat", "09:00"), "2026-11-06T00:00:00Z", tokyo, "2026-11-07T00:00:00Z"},
		{"weekly same day not passed", weekly("fri", "18:00"), "2026-11-06T00:00:00Z", tokyo, "2026-11-06T09:00:00Z"},
		{"weekly same day passed", weekly("fri", "08:00"), "2026-11-06T00:00:00Z", tokyo, "2026-11-12T23:00:00Z"},
		{"weekly sun to mon boundary", weekly("mon", "00:00"), "2026-11-08T14:59:00Z", tokyo, "2026-11-08T15:00:00Z"},
		{"weekly exactly at run", weekly("mon", "00:00"), "2026-11-08T15:00:00Z", tokyo, "2026-11-15T15:00:00Z"},

		// Monthly, month end and year boundary.
		{"monthly day 1 next month", monthly(1, "00:00"), "2026-11-06T00:00:00Z", tokyo, "2026-11-30T15:00:00Z"},
		{"monthly day 28 this month", monthly(28, "23:59"), "2026-11-06T00:00:00Z", tokyo, "2026-11-28T14:59:00Z"},
		{"monthly day 28 february", monthly(28, "12:00"), "2027-01-28T04:00:00Z", tokyo, "2027-02-28T03:00:00Z"},
		{"monthly day 28 passed in feb", monthly(28, "12:00"), "2027-02-28T03:00:00Z", tokyo, "2027-03-28T03:00:00Z"},
		{"monthly across year", monthly(15, "09:00"), "2026-12-20T00:00:00Z", tokyo, "2027-01-15T00:00:00Z"},
		{"monthly utc month differs from local", monthly(1, "08:00"), "2026-11-30T22:00:00Z", tokyo, "2026-11-30T23:00:00Z"},

		// Year boundary.
		{"daily new year tokyo", daily("00:00"), "2026-12-31T15:00:00Z", tokyo, "2027-01-01T15:00:00Z"},
		{"daily new year eve tokyo", daily("00:00"), "2026-12-31T14:00:00Z", tokyo, "2026-12-31T15:00:00Z"},
		{"weekly across year ny", weekly("fri", "07:00"), "2026-12-30T00:00:00Z", ny, "2027-01-01T12:00:00Z"},

		// nil location means Asia/Tokyo.
		{"nil location is tokyo", daily("08:00"), "2026-11-05T22:00:00Z", nil, "2026-11-05T23:00:00Z"},
		// after given in another zone gives the same answer.
		{"after with non-UTC location", daily("08:00"), "2026-11-05T17:00:00-05:00", tokyo, "2026-11-05T23:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Next(tt.spec, utc(tt.after), tt.loc)
			want := utc(tt.want)
			if !got.Equal(want) || got.Location() != time.UTC {
				t.Errorf("Next = %v (%v), want %v UTC", got, got.Location(), want)
			}
		})
	}
}

func TestNextInvalid(t *testing.T) {
	after := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	for _, s := range []Spec{
		{},
		{Every: "hourly", Time: "08:00"},
		{Every: Daily, Time: "8:00"},
		{Every: Daily, Time: "24:00"},
		{Every: Daily, Time: "08:60"},
		{Every: Daily, Time: "0a:00"},
		{Every: Daily, Time: "08:00", Day: 1},
		{Every: Weekly, Time: "08:00"},
		{Every: Weekly, Time: "08:00", Weekday: "monday"},
		{Every: Weekly, Time: "08:00", Weekday: "mon", Day: 1},
		{Every: Monthly, Time: "08:00"},
		{Every: Monthly, Time: "08:00", Day: 29},
		{Every: Monthly, Time: "08:00", Day: 1, Weekday: "mon"},
	} {
		if s.Validate() == nil {
			t.Errorf("Validate(%+v) = nil, want error", s)
		}
		if got := Next(s, after, nil); !got.IsZero() {
			t.Errorf("Next(%+v) = %v, want zero", s, got)
		}
	}
}

func TestLocation(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", DefaultZone},
		{"Not/AZone", DefaultZone},
		{"Local", DefaultZone},
		{"America/New_York", "America/New_York"},
		{"UTC", "UTC"},
	} {
		if got := Location(tt.in).String(); got != tt.want {
			t.Errorf("Location(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestJitter(t *testing.T) {
	const max = 5 * time.Minute
	seen := map[time.Duration]bool{}
	for _, key := range []string{"app-1", "app-2", "app-3", "app-4", "app-5", "", "日本語のID"} {
		a, b := Jitter(key, max), Jitter(key, max)
		if a != b {
			t.Errorf("Jitter(%q) not stable: %v vs %v", key, a, b)
		}
		if a < 0 || a >= max {
			t.Errorf("Jitter(%q) = %v, want [0, %v)", key, a, max)
		}
		if a%time.Second != 0 {
			t.Errorf("Jitter(%q) = %v, want whole seconds", key, a)
		}
		seen[a] = true
	}
	if len(seen) < 3 {
		t.Errorf("Jitter spreads too little: %v", seen)
	}
	for _, m := range []time.Duration{0, -time.Minute} {
		if got := Jitter("app-1", m); got != 0 {
			t.Errorf("Jitter(max=%v) = %v, want 0", m, got)
		}
	}
	if got := Jitter("app-1", time.Nanosecond); got != 0 {
		t.Errorf("Jitter(max=1ns) = %v, want 0", got)
	}
}

func TestIsLate(t *testing.T) {
	due := time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name    string
		now     time.Time
		maxLate time.Duration
		want    bool
	}{
		{"before due", due.Add(-time.Minute), 0, false},
		{"at due", due, 0, false},
		{"exactly default max", due.Add(DefaultMaxLate), 0, false},
		{"just over default max", due.Add(DefaultMaxLate + time.Nanosecond), 0, true},
		{"negative max uses default", due.Add(DefaultMaxLate), -1, false},
		{"custom max exactly", due.Add(5 * time.Minute), 5 * time.Minute, false},
		{"custom max over", due.Add(5*time.Minute + time.Second), 5 * time.Minute, true},
	} {
		if got := IsLate(due, tt.now, tt.maxLate); got != tt.want {
			t.Errorf("%s: IsLate = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestInactive(t *testing.T) {
	opened := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for _, tt := range []struct {
		name       string
		lastOpened time.Time
		now        time.Time
		days       int
		want       bool
	}{
		{"just opened", opened, opened, 0, false},
		{"one ns short of default", opened, opened.Add(DefaultInactiveDays*day - time.Nanosecond), 0, false},
		{"exactly default days", opened, opened.Add(DefaultInactiveDays * day), 0, true},
		{"over default days", opened, opened.Add(40 * day), 0, true},
		{"negative days uses default", opened, opened.Add(29 * day), -1, false},
		{"custom days exactly", opened, opened.Add(7 * day), 7, true},
		{"custom days short", opened, opened.Add(7*day - time.Second), 7, false},
		{"never opened", time.Time{}, opened, 0, true},
	} {
		if got := Inactive(tt.lastOpened, tt.now, tt.days); got != tt.want {
			t.Errorf("%s: Inactive = %v, want %v", tt.name, got, tt.want)
		}
	}
}
