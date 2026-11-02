package expreval

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	// ISO8601DateFormatter's default (.withInternetDateTime): seconds and an
	// offset are required, fractional seconds are not accepted.
	isoRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(Z|[+-]\d{2}:?\d{2})$`)
	// DateFormatter "yyyy-MM-dd" (en_US_POSIX): month and day may have one digit.
	dayRe = regexp.MustCompile(`^(\d{1,4})-(\d{1,2})-(\d{1,2})$`)
)

// parseDate is FieldCoding.date(from:): RFC 3339 with an offset, then
// "YYYY-MM-DD" at 0:00 in loc. Anything else (fractional seconds, no offset,
// impossible dates such as 2026-02-30) is not a date.
func parseDate(s string, loc *time.Location) (time.Time, bool) {
	if m := isoRe.FindStringSubmatch(s); m != nil {
		y, mo, d := atoi(m[1]), atoi(m[2]), atoi(m[3])
		h, mi, sec := atoi(m[4]), atoi(m[5]), atoi(m[6])
		if !validDay(y, mo, d) || h > 23 || mi > 59 || sec > 59 {
			return time.Time{}, false
		}
		zone := time.UTC
		if m[7] != "Z" {
			off := strings.ReplaceAll(m[7][1:], ":", "")
			oh, om := atoi(off[:2]), atoi(off[2:])
			secs := oh*3600 + om*60
			if m[7][0] == '-' {
				secs = -secs
			}
			zone = time.FixedZone("", secs)
		}
		return time.Date(y, time.Month(mo), d, h, mi, sec, 0, zone), true
	}
	if m := dayRe.FindStringSubmatch(s); m != nil {
		y, mo, d := atoi(m[1]), atoi(m[2]), atoi(m[3])
		if y == 0 || !validDay(y, mo, d) {
			return time.Time{}, false
		}
		return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, loc), true
	}
	return time.Time{}, false
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
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// addDays is Calendar.date(byAdding: .day): same wall-clock time n days later.
func addDays(t time.Time, n int, loc *time.Location) time.Time {
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day()+n, l.Hour(), l.Minute(), l.Second(), l.Nanosecond(), loc)
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

// timeOfDay is ExprEvaluator.timeOfDay: "HH:mm" / "H:mm" / "HH:mm:ss" (up to
// "24:00") to seconds after 0:00.
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
	if h > 24 || m > 59 || sec > 59 || (h == 24 && (m != 0 || sec != 0)) {
		return 0, false
	}
	return float64(h*3600 + m*60 + sec), true
}

func clockSeconds(t time.Time, loc *time.Location) float64 {
	l := t.In(loc)
	return float64(l.Hour()*3600 + l.Minute()*60 + l.Second())
}
