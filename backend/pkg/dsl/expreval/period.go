package expreval

import "time"

// Period is the span a date / datetime value (and its optional end) covers,
// [Start, End): a date "YYYY-MM-DD" is its whole day in loc (an end date is
// included), a datetime is [start, end) or the instant itself without an
// end. ok is false for an empty / malformed start, a malformed end or an end
// before the start. Used by the server rules' overlap lookup.
func Period(start, end any, loc *time.Location) (from, to time.Time, ok bool) {
	s, ok := parseDateJSON(start, loc)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	sDay := isDay(start)
	e, eDay := s, sDay
	if str, _ := end.(string); end != nil && str != "" {
		if e, ok = parseDateJSON(end, loc); !ok {
			return time.Time{}, time.Time{}, false
		}
		eDay = isDay(end)
	}
	if e.Before(s) {
		return time.Time{}, time.Time{}, false
	}
	switch {
	case eDay:
		e = startOfDay(addDays(startOfDay(e, loc), 1, loc), loc)
	case !e.After(s):
		e = s.Add(time.Nanosecond) // an instant
	}
	return s, e, true
}

func isDay(v any) bool {
	s, _ := v.(string)
	return dayRe.MatchString(s)
}

// Overlaps reports whether [aFrom, aTo) and [bFrom, bTo) share an instant.
func Overlaps(aFrom, aTo, bFrom, bTo time.Time) bool {
	return aFrom.Before(bTo) && bFrom.Before(aTo)
}
