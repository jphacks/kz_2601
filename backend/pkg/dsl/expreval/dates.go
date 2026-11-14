package expreval

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	// ISO8601DateFormatter's default (.withInternetDateTime): seconds and an
	// offset are required. FieldCoding.date also reads fractional seconds by
	// removing them, parsing the rest and adding them back.
	isoRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[+-]\d{2}:?\d{2})$`)
	// DateFormatter "yyyy-MM-dd" (en_US_POSIX): month and day may have one digit.
	dayRe = regexp.MustCompile(`^(\d{1,4})-(\d{1,2})-(\d{1,2})$`)
)

// parseDate is FieldCoding.date(from:): RFC 3339 with an offset (fractional
// seconds allowed), then "YYYY-MM-DD" at the start of that day in loc
// (dayStart). Anything else (no offset, impossible dates such as 2026-02-30) is
// not a date.
func parseDate(s string, loc *time.Location) (time.Time, bool) {
	if m := isoRe.FindStringSubmatch(s); m != nil {
		y, mo, d := atoi(m[1]), atoi(m[2]), atoi(m[3])
		h, mi, sec := atoi(m[4]), atoi(m[5]), atoi(m[6])
		if !validDay(y, mo, d) || h > 23 || mi > 59 || sec > 59 {
			return time.Time{}, false
		}
		zone := time.UTC
		if m[8] != "Z" {
			off := strings.ReplaceAll(m[8][1:], ":", "")
			oh, om := atoi(off[:2]), atoi(off[2:])
			secs := oh*3600 + om*60
			if m[8][0] == '-' {
				secs = -secs
			}
			zone = time.FixedZone("", secs)
		}
		return time.Date(y, time.Month(mo), d, h, mi, sec, fracNanos(m[7]), zone), true
	}
	if m := dayRe.FindStringSubmatch(s); m != nil {
		y, mo, d := atoi(m[1]), atoi(m[2]), atoi(m[3])
		if y == 0 || !validDay(y, mo, d) {
			return time.Time{}, false
		}
		return dayStart(y, time.Month(mo), d, loc)
	}
	return time.Time{}, false
}

// dayStart is Calendar.startOfDay: the first instant of y-m-d in loc. On a day
// whose 0:00 falls in a DST gap (America/Sao_Paulo 2018-11-04) that is the
// transition (1:00). time.Date normalizes a time in a gap to either side of the
// transition, so move to the transition from there. A day that does not exist
// at all (Pacific/Apia 2011-12-30) is not a date.
func dayStart(y int, m time.Month, d int, loc *time.Location) (time.Time, bool) {
	t := time.Date(y, m, d, 0, 0, 0, 0, loc)
	if ty, tm, td := t.Date(); ty != y || tm != m || td != d {
		_, end := t.ZoneBounds() // landed on the previous day: the gap starts at end
		t = end
	} else if t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0 {
		start, _ := t.ZoneBounds() // landed past the gap: it ends at start
		t = start
	}
	if t.IsZero() {
		return time.Time{}, false
	}
	t = t.In(loc)
	ty, tm, td := t.Date()
	return t, ty == y && tm == m && td == d
}

// fracNanos turns ".123" into nanoseconds (digits past the 9th are dropped).
func fracNanos(s string) int {
	if s == "" {
		return 0
	}
	digits := (s[1:] + "000000000")[:9]
	return atoi(digits)
}

func parseDateJSON(v any, loc *time.Location) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	return parseDate(s, loc)
}

func validDay(y, m, d int) bool {
	if m < 1 || m > 12 || d < 1 {
		return false
	}
	return d <= time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func dayKey(t time.Time, loc *time.Location) string { return t.In(loc).Format("2006-01-02") }

func startOfDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	s, _ := dayStart(y, m, d, loc) // t's own day always exists
	return s
}

// addDays is Calendar.date(byAdding: .day): same wall-clock time n days later.
// A time in a DST gap moves forward by the gap (Sao Paulo 2018-11-03 0:30 + 1
// day = 2018-11-04 1:30), as Foundation does.
func addDays(t time.Time, n int, loc *time.Location) time.Time {
	l := t.In(loc)
	return wallClock(l.Year(), l.Month(), l.Day()+n, l, loc)
}

// wallClock is y-m-d at clock's wall-clock time in loc. A time in a DST gap
// moves forward by the gap, as Foundation's calendar math does.
func wallClock(y int, m time.Month, d int, clock time.Time, loc *time.Location) time.Time {
	r := time.Date(y, m, d, clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), loc)
	want := time.Date(y, m, d, clock.Hour(), clock.Minute(), clock.Second(), clock.Nanosecond(), time.UTC)
	got := time.Date(r.Year(), r.Month(), r.Day(), r.Hour(), r.Minute(), r.Second(), r.Nanosecond(), time.UTC)
	if got.Before(want) {
		// time.Date read the gap time with the offset after the gap: use the one before it
		_, off := r.Zone()
		return want.Add(-time.Duration(off) * time.Second).In(loc)
	}
	return r
}

// addMonths is Calendar.date(byAdding: .month): same wall-clock time n months
// later, the day clamped to the last day of that month (Jan 31 + 1 = Feb 28).
// A time in a DST gap moves forward like addDays (Sao Paulo 2018-10-04 0:00 + 1
// month = 2018-11-04 1:00, not the previous day).
func addMonths(t time.Time, n int, loc *time.Location) time.Time {
	l := t.In(loc)
	first := time.Date(l.Year(), l.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	y, m := first.Year(), first.Month()
	day := min(l.Day(), time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day())
	return wallClock(y, m, day, l, loc)
}

// nextOccurrence is RepeatDate.next (#266): the first monthly / yearly repeat of
// anchor's day on or after today (anchor's own day when that is today or later).
func nextOccurrence(anchor time.Time, unit string, now time.Time, loc *time.Location) (time.Time, bool) {
	step := 0
	switch unit {
	case "monthly":
		step = 1
	case "yearly":
		step = 12
	default:
		return time.Time{}, false
	}
	a, t := startOfDay(anchor, loc), startOfDay(now, loc)
	if !a.Before(t) {
		return a, true
	}
	k := max(((t.Year()-a.Year())*12+int(t.Month())-int(a.Month()))/step, 0)
	for range 4 {
		if d := startOfDay(addMonths(a, step*k, loc), loc); !d.Before(t) {
			return d, true
		}
		k++
	}
	return time.Time{}, false
}

// daysBetween counts calendar days from a's day to b's day in loc (DST days count as one).
func daysBetween(a, b time.Time, loc *time.Location) int {
	civil := func(t time.Time) time.Time {
		y, m, d := t.In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	return int(civil(b).Sub(civil(a)).Hours() / 24)
}

func sameDay(a, b time.Time, loc *time.Location) bool { return dayKey(a, loc) == dayKey(b, loc) }

func sameMonth(a, b time.Time, loc *time.Location) bool {
	x, y := a.In(loc), b.In(loc)
	return x.Year() == y.Year() && x.Month() == y.Month()
}

func sameYear(a, b time.Time, loc *time.Location) bool { return a.In(loc).Year() == b.In(loc).Year() }

// week is Monday 0:00 up to the next Monday 0:00 (Aggregator.weekCalendar).
func week(now time.Time, loc *time.Location) (time.Time, time.Time) {
	sod := startOfDay(now, loc)
	back := (int(sod.Weekday()) + 6) % 7
	start := addDays(sod, -back, loc)
	return start, addDays(start, 7, loc)
}

// timeOfDay is ExprEvaluator.timeOfDay: "HH:mm" / "H:mm" / "HH:mm:ss"
// (00:00 to 23:59:59) to seconds after 0:00.
func timeOfDay(s string) (float64, bool) {
	s = strings.TrimFunc(s, func(r rune) bool { return r == '\t' || unicode.Is(unicode.Zs, r) })
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	for _, p := range parts {
		if p == "" || len(p) > 2 {
			return 0, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return 0, false
			}
		}
	}
	h, m := atoi(parts[0]), atoi(parts[1])
	sec := 0
	if len(parts) == 3 {
		sec = atoi(parts[2])
	}
	if h > 23 || m > 59 || sec > 59 {
		return 0, false
	}
	return float64(h*3600 + m*60 + sec), true
}

// clockString is ExprEvaluator.clockString: seconds after 0:00 wrapped into
// one day, as "HH:mm" ("HH:mm:ss" when the seconds are not 0).
func clockString(secs float64) string {
	t := int(math.Mod(math.Mod(secs, 86400)+86400, 86400))
	if t%60 != 0 {
		return fmt.Sprintf("%02d:%02d:%02d", t/3600, t%3600/60, t%60)
	}
	return fmt.Sprintf("%02d:%02d", t/3600, t%3600/60)
}

func clockSeconds(t time.Time, loc *time.Location) float64 {
	l := t.In(loc)
	return float64(l.Hour()*3600 + l.Minute()*60 + l.Second())
}
