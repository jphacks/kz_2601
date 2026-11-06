// Package schedule computes when time-triggered rules run (#342). It has no
// I/O: callers pass the clock and the location in.
//
// The runner binary must `import _ "time/tzdata"` so that LoadLocation works
// in a container image without a system zoneinfo.
package schedule

import (
	"errors"
	"fmt"
	"hash/fnv"
	"time"
)

// Every is how often a time-triggered rule runs.
type Every string

const (
	Daily   Every = "daily"
	Weekly  Every = "weekly"
	Monthly Every = "monthly"
)

const (
	// DefaultZone is used when the time zone is empty or unknown.
	DefaultZone = "Asia/Tokyo"
	// DefaultMaxLate is how late a run may start before it is skipped.
	DefaultMaxLate = 30 * time.Minute
	// DefaultInactiveDays stops an app's scheduled rules after this many days unopened.
	DefaultInactiveDays = 30
	// MaxDay is the last day of the month a monthly rule may name.
	MaxDay = 28
)

// Spec is the time trigger of a rule. It mirrors the DSL's RuleTrigger
// (every / time / weekday / day) but is kept independent of pkg/dsl.
type Spec struct {
	Every   Every
	Time    string // HH:MM, wall clock in the rule's location
	Weekday string // weekly: mon..sun
	Day     int    // monthly: 1..MaxDay
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// Validate reports whether the spec can be scheduled.
func (s Spec) Validate() error {
	if _, _, err := parseHHMM(s.Time); err != nil {
		return err
	}
	switch s.Every {
	case Daily:
		if s.Weekday != "" || s.Day != 0 {
			return errors.New("schedule: daily takes no weekday / day")
		}
	case Weekly:
		if _, ok := weekdays[s.Weekday]; !ok {
			return fmt.Errorf("schedule: weekly needs weekday mon..sun (got %q)", s.Weekday)
		}
		if s.Day != 0 {
			return errors.New("schedule: weekly takes no day")
		}
	case Monthly:
		if s.Day < 1 || s.Day > MaxDay {
			return fmt.Errorf("schedule: monthly needs day 1-%d (got %d)", MaxDay, s.Day)
		}
		if s.Weekday != "" {
			return errors.New("schedule: monthly takes no weekday")
		}
	default:
		return fmt.Errorf("schedule: every must be daily, weekly or monthly (got %q)", s.Every)
	}
	return nil
}

func parseHHMM(v string) (h, m int, err error) {
	if len(v) != 5 || v[2] != ':' {
		return 0, 0, fmt.Errorf("schedule: time %q must be HH:MM", v)
	}
	digit := func(c byte) (int, bool) { return int(c - '0'), c >= '0' && c <= '9' }
	h1, ok1 := digit(v[0])
	h2, ok2 := digit(v[1])
	m1, ok3 := digit(v[3])
	m2, ok4 := digit(v[4])
	h, m = h1*10+h2, m1*10+m2
	if !ok1 || !ok2 || !ok3 || !ok4 || h > 23 || m > 59 {
		return 0, 0, fmt.Errorf("schedule: time %q must be HH:MM", v)
	}
	return h, m, nil
}

// Location loads a time zone by IANA name. An empty or unknown name (and
// "Local", which would depend on the server) falls back to DefaultZone.
func Location(name string) *time.Location {
	if name != "" && name != "Local" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return defaultLocation()
}

func defaultLocation() *time.Location {
	if loc, err := time.LoadLocation(DefaultZone); err == nil {
		return loc
	}
	return time.FixedZone("JST", 9*60*60)
}

// Next returns the first run time strictly after `after`, in UTC. The spec's
// wall clock is read in loc (nil means DefaultZone). It returns the zero time
// if the spec is invalid.
//
// Daylight saving time:
//   - A wall time that does not exist (spring forward, e.g. 02:30 on the day
//     New York jumps from 02:00 to 03:00) runs at the instant the clock jumps
//     (03:00 local), i.e. the first wall time that exists after it. The run is
//     not skipped and not moved to the next day.
//   - A wall time that occurs twice (fall back, e.g. 01:30 in New York) runs
//     once, at the first occurrence (the earlier, daylight-time one). If
//     `after` falls between the two occurrences, Next moves to the next
//     scheduled day.
func Next(spec Spec, after time.Time, loc *time.Location) time.Time {
	if spec.Validate() != nil {
		return time.Time{}
	}
	if loc == nil {
		loc = defaultLocation()
	}
	h, m, _ := parseHHMM(spec.Time)
	a := after.In(loc)

	if spec.Every == Monthly {
		// Start a month early so `after` near a month boundary in UTC vs loc
		// cannot skip a run; time.Date normalizes month overflow.
		y, mo := a.Year(), a.Month()-1
		for i := 0; i < 4; i++ {
			if t := resolve(y, mo+time.Month(i), spec.Day, h, m, loc); t.After(after) {
				return t.UTC()
			}
		}
		return time.Time{}
	}

	y, mo, d := a.Date()
	for i := -1; i <= 8; i++ {
		day := time.Date(y, mo, d+i, 12, 0, 0, 0, time.UTC) // calendar arithmetic only
		if spec.Every == Weekly && day.Weekday() != weekdays[spec.Weekday] {
			continue
		}
		if t := resolve(day.Year(), day.Month(), day.Day(), h, m, loc); t.After(after) {
			return t.UTC()
		}
	}
	return time.Time{}
}

// resolve maps a wall clock in loc to an instant, with the DST rules
// documented on Next. time.Date leaves gaps and overlaps unspecified, so the
// offsets are worked out here.
func resolve(y int, mo time.Month, d, h, m int, loc *time.Location) time.Time {
	wall := time.Date(y, mo, d, h, m, 0, 0, time.UTC).Unix() // the wall clock read as if UTC
	offsetAt := func(sec int64) int64 {
		_, off := time.Unix(sec, 0).In(loc).Zone()
		return int64(off)
	}
	// Real zones change offset at most once in a few days, so the offsets a
	// day before and after cover every candidate.
	const span = 26 * 60 * 60
	offBefore, offAfter := offsetAt(wall-span), offsetAt(wall+span)

	best := int64(0)
	found := false
	for _, off := range []int64{offBefore, offAfter} {
		t := wall - off
		if offsetAt(t) == off && (!found || t < best) {
			best, found = t, true
		}
	}
	if found {
		return time.Unix(best, 0).In(loc)
	}

	// In a gap: find the instant the clock jumps. lo is still before it,
	// hi is after it.
	lo, hi := wall-offAfter, wall-offBefore
	if lo > hi {
		lo, hi = hi, lo
	}
	target := offsetAt(hi)
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if offsetAt(mid) == target {
			hi = mid
		} else {
			lo = mid
		}
	}
	return time.Unix(hi, 0).In(loc)
}

// Jitter spreads runs scheduled for the same minute. It returns a duration
// in [0, max) derived from key (e.g. the appId); the same key always gets the
// same value. With max of at least a second the result is whole seconds.
func Jitter(key string, max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	f := fnv.New64a()
	_, _ = f.Write([]byte(key))
	d := time.Duration(f.Sum64() % uint64(max))
	if max >= time.Second {
		d = d.Truncate(time.Second)
	}
	return d
}

// IsLate reports whether a run due at `due` is too late to start at `now`.
// Exactly maxLate behind still runs. maxLate <= 0 means DefaultMaxLate.
func IsLate(due, now time.Time, maxLate time.Duration) bool {
	if maxLate <= 0 {
		maxLate = DefaultMaxLate
	}
	return now.Sub(due) > maxLate
}

// Inactive reports whether an app last opened at lastOpened has gone unused
// for `days` days (days <= 0 means DefaultInactiveDays), so its scheduled
// rules stop. Exactly `days` days counts as inactive. A zero lastOpened is
// inactive; callers pass the app's creation time if it was never opened.
func Inactive(lastOpened, now time.Time, days int) bool {
	if days <= 0 {
		days = DefaultInactiveDays
	}
	if lastOpened.IsZero() {
		return true
	}
	return now.Sub(lastOpened) >= time.Duration(days)*24*time.Hour
}
