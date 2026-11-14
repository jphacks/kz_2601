package expreval

// Where Go and Swift/Foundation differ, this package copies the iOS behavior
// (checked by docs/dsl/conformance/expr/*.json):
//
//   - Number text (concat / toString / len): integers below 1e15 print without
//     a fraction; other values use Swift's Double description: shortest digits,
//     decimal for 1e-4 <= |x| <= 2^53 with a ".0" when integral
//     ("1000000000000000.0"), exponential otherwise ("1e+16", "1e-05"),
//     "inf" / "-inf" / "nan". strconv's 'g' format differs, so swiftDescription
//     builds it from 'f' / 'e'.
//   - String → number is Swift's Double(String) (strtod on the whole string):
//     no surrounding spaces, "0x10" without a "p" exponent is 16, "1e400" is
//     +inf, "inf" / "infinity" / "nan" in any case; underscores are rejected
//     (strconv would accept some). Commas are removed first ("1,200" = 1200)
//     in ops, agg sums and comparisons.
//   - Date strings are FieldCoding.date(from:): RFC 3339 with seconds and an
//     offset ("Z", "+09:00" or "+0900"), optionally with fractional seconds
//     (".123"); or "YYYY-MM-DD" (month/day may be one digit) at the start of
//     that day in the device time zone (Calendar.startOfDay: 1:00 on a day
//     whose 0:00 is skipped by DST, such as America/Sao_Paulo 2018-11-04,
//     never the previous day). Impossible dates and times (2026-02-30, T24:00,
//     2027-02-29) and days skipped entirely (Pacific/Apia 2011-12-30) are not
//     dates on either side.
//   - Calendar math uses Env.Location like Calendar.current: today = the start
//     of the local day, daysBetween counts local calendar days (a 23/25-hour
//     DST day counts as one), addDays keeps the wall-clock time (a time in a
//     DST gap moves forward by the gap, as Foundation does), hoursBetween reads the
//     local clock of datetimes (so a DST night shows wall-clock hours),
//     thisWeek is Monday 0:00 to the next Monday 0:00. addMinutes adds elapsed
//     minutes to a date / datetime (a "YYYY-MM-DD" is its local start of day;
//     across DST the wall clock moves by the offset change) and wraps a
//     "HH:mm" time within the day, answering "HH:mm".
//   - Datetime computed values merged into records (agg over a computed field)
//     are written as RFC 3339 in UTC, like ISO8601DateFormatter's default.
//   - len counts grapheme clusters like Swift's String.count, approximated
//     without Unicode tables (see graphemeCount): combining marks, ZWJ
//     sequences, variation selectors, skin tones, flags, CR LF and Hangul jamo.
//   - String ordering (gt/lt on two non-numeric strings) compares UTF-16 code
//     units like NSString.compare without options. NSString also treats
//     canonically equivalent strings (precomposed vs combining) as equal;
//     this package does not.
//   - Rounding is .toNearestOrAwayFromZero (math.Round); round/floor/ceil scale
//     by 10^n with math.Pow10 for integral n (exact like Foundation.pow here).
//   - pow is rounded to the nearest float64 like Foundation.pow (math.Pow is
//     not, and its last bit differs by GOARCH; see pow.go).
//   - The evaluation limit is depth 48 (nesting plus computed chains), as on
//     iOS; computed cycles evaluate to nil.
//
// Record.CreatedAt and Record.AuthorName use "" for "absent" (Swift uses nil).
// A filter value Expr that evaluates to nil makes the filters match no record.
// Filter gt/gte/lt/lte compare numbers (numeric strings too), else dates.
// A filter value {"me": true} is left as is (the caller substitutes the uid),
// so it never matches, as on iOS before resolvingMe.
