package expreval

import "time"

// Period is the span a date / datetime value (and its optional end) covers,
// [Start, End). day is the field's type, not the form of the stored value:
// a date period is whole days in loc (the end day included, a value kept in
// RFC 3339 counts as its day in loc), a datetime period is [start, end) or
// the instant itself without an end. A nil / "" end is the start; ok is
// false for an empty / malformed start, a malformed end (also one that is
// not a string) or an end before the start. Used by the server rules'
// overlap lookup.
func Period(start, end any, day bool, loc *time.Location) (from, to time.Time, ok bool) {
	s, ok := parseDateJSON(start, loc)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	e := s
	if str, isStr := end.(string); end != nil && (!isStr || str != "") {
		if e, ok = parseDateJSON(end, loc); !ok {
			return time.Time{}, time.Time{}, false
		}
	}
	if day {
		s, e = startOfDay(s, loc), startOfDay(e, loc)
	}
	if e.Before(s) {
		return time.Time{}, time.Time{}, false
	}
	switch {
	case day:
		e = startOfDay(addDays(e, 1, loc), loc)
	case !e.After(s):
		e = s.Add(time.Nanosecond) // an instant
	}
	return s, e, true
}

// Overlaps reports whether [aFrom, aTo) and [bFrom, bTo) share an instant.
func Overlaps(aFrom, aTo, bFrom, bTo time.Time) bool {
	return aFrom.Before(bTo) && bFrom.Before(aTo)
}
