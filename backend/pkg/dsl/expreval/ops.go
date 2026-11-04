package expreval

import (
	"math"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func finite(x float64) Value {
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return Value{}
	}
	return NumberValue(x)
}

// apply is ExprEvaluator.apply.
func (ev *Evaluator) apply(name string, args []dsl.Expr, row *Row) Value {
	// lazily evaluated ops first
	switch name {
	case "and":
		for _, a := range args {
			if !ev.Eval(a, row).truthy() {
				return BoolValue(false)
			}
		}
		return BoolValue(true)
	case "or":
		for _, a := range args {
			if ev.Eval(a, row).truthy() {
				return BoolValue(true)
			}
		}
		return BoolValue(false)
	case "coalesce":
		for _, a := range args {
			if v := ev.Eval(a, row); !v.IsNil() && !v.isEmptyString() {
				return v
			}
		}
		return Value{}
	case "len":
		// type refs (#231): number of chosen records still alive; none = all records of refEntity
		if len(args) > 0 && args[0].Kind == dsl.ExprRef && !strings.Contains(args[0].Ref, ".") && row != nil {
			if f := field(row.Entity, args[0].Ref); f != nil && f.Type == dsl.FieldRefs {
				ids := ev.liveRefIDs(get(row.Record, args[0].Ref), f)
				if len(ids) == 0 {
					return NumberValue(float64(len(ev.records(f.RefEntity))))
				}
				return NumberValue(float64(len(ids)))
			}
		}
	case "streak", "doneInDays":
		if len(args) == 0 || args[0].Kind != dsl.ExprRef || strings.Contains(args[0].Ref, ".") || row == nil {
			return Value{}
		}
		days := ev.dailyDates(row.Record, args[0].Ref)
		if name == "streak" {
			return NumberValue(float64(ev.streak(days)))
		}
		if len(args) < 2 {
			return Value{}
		}
		n, ok := ev.Eval(args[1], row).number()
		if !ok || math.IsInf(n, 0) || math.IsNaN(n) {
			return Value{}
		}
		return NumberValue(float64(ev.doneInDays(days, int(roundAway(n)))))
	}

	vals := make([]Value, len(args))
	for i, a := range args {
		vals[i] = ev.Eval(a, row)
	}
	num := func(i int) (float64, bool) {
		if i >= len(vals) {
			return 0, false
		}
		return vals[i].number()
	}
	date := func(i int) (time.Time, bool) {
		if i >= len(vals) {
			return time.Time{}, false
		}
		switch vals[i].Kind {
		case Date:
			return vals[i].Time, true
		case String:
			return parseDate(vals[i].Str, ev.loc)
		}
		return time.Time{}, false
	}
	secondsOfDay := func(i int) (float64, bool) {
		if i >= len(vals) {
			return 0, false
		}
		switch v := vals[i]; v.Kind {
		case String:
			if t, ok := timeOfDay(v.Str); ok {
				return t, true
			}
			d, ok := parseDate(v.Str, ev.loc)
			if !ok || !strings.Contains(v.Str, "T") {
				return 0, false
			}
			return clockSeconds(d, ev.loc), true
		case Date:
			return clockSeconds(v.Time, ev.loc), true
		}
		return 0, false
	}
	allNums := func() ([]float64, bool) {
		if len(vals) == 0 {
			return nil, false
		}
		ns := make([]float64, len(vals))
		for i := range vals {
			n, ok := vals[i].number()
			if !ok {
				return nil, false
			}
			ns[i] = n
		}
		return ns, true
	}
	two := func() (float64, float64, bool) {
		a, ok1 := num(0)
		b, ok2 := num(1)
		return a, b, ok1 && ok2
	}

	switch name {
	// arithmetic
	case "add", "mul":
		ns, ok := allNums()
		if !ok {
			return Value{}
		}
		acc := 0.0
		if name == "mul" {
			acc = 1
		}
		for _, n := range ns {
			if name == "add" {
				acc += n
			} else {
				acc *= n
			}
		}
		return NumberValue(acc) // not checked for overflow (iOS)
	case "sub":
		if a, b, ok := two(); ok {
			return finite(a - b)
		}
	case "div":
		if a, b, ok := two(); ok && b != 0 {
			return finite(a / b)
		}
	case "mod":
		if a, b, ok := two(); ok && b != 0 {
			return finite(math.Mod(a, b))
		}
	case "neg":
		if a, ok := num(0); ok {
			return NumberValue(-a)
		}
	case "abs":
		if a, ok := num(0); ok {
			return NumberValue(math.Abs(a))
		}
	case "sqrt":
		if a, ok := num(0); ok && a >= 0 {
			return NumberValue(math.Sqrt(a))
		}
	case "pow":
		if a, b, ok := two(); ok {
			return finite(pow(a, b))
		}
	case "round", "floor", "ceil":
		a, ok := num(0)
		if !ok {
			return Value{}
		}
		digits, _ := num(1) // missing or not a number = 0
		scale := pow10(roundAway(digits))
		x := a * scale
		switch name {
		case "floor":
			x = math.Floor(x)
		case "ceil":
			x = math.Ceil(x)
		default:
			x = roundAway(x)
		}
		return finite(x / scale)
	case "min", "max":
		ns, ok := allNums()
		if !ok {
			return Value{}
		}
		m := ns[0]
		for _, n := range ns[1:] {
			if (name == "min" && n < m) || (name == "max" && n > m) {
				m = n
			}
		}
		return NumberValue(m)
	case "clamp":
		x, ok1 := num(0)
		lo, ok2 := num(1)
		hi, ok3 := num(2)
		if ok1 && ok2 && ok3 && lo <= hi {
			return NumberValue(math.Min(math.Max(x, lo), hi))
		}
	// comparison
	case "eq", "neq":
		if len(vals) < 2 {
			return Value{}
		}
		same := ev.equal(vals[0], vals[1])
		return BoolValue(same == (name == "eq"))
	case "gt", "gte", "lt", "lte":
		if len(vals) < 2 || vals[0].IsNil() || vals[1].IsNil() {
			return Value{}
		}
		c, ok := ev.compare(vals[0], vals[1])
		if !ok {
			return Value{}
		}
		switch name {
		case "gt":
			return BoolValue(c > 0)
		case "gte":
			return BoolValue(c >= 0)
		case "lt":
			return BoolValue(c < 0)
		}
		return BoolValue(c <= 0)
	// logic
	case "not":
		return BoolValue(len(vals) == 0 || !vals[0].truthy())
	// strings
	case "concat":
		var b strings.Builder
		for _, v := range vals {
			b.WriteString(v.Text(ev.loc))
		}
		return StringValue(b.String())
	case "len":
		if len(vals) > 0 && !vals[0].IsNil() {
			return NumberValue(float64(graphemeCount(vals[0].Text(ev.loc))))
		}
	// dates
	case "now":
		return DateValue(ev.now)
	case "today":
		return DateValue(startOfDay(ev.now, ev.loc))
	case "daysBetween":
		a, ok1 := date(0)
		b, ok2 := date(1)
		if ok1 && ok2 {
			return NumberValue(float64(daysBetween(a, b, ev.loc)))
		}
	case "daysUntil":
		if d, ok := date(0); ok {
			return NumberValue(float64(daysBetween(ev.now, d, ev.loc)))
		}
	case "addDays":
		d, ok1 := date(0)
		n, ok2 := num(1)
		if ok1 && ok2 && !math.IsInf(n, 0) && !math.IsNaN(n) {
			return DateValue(addDays(d, int(roundAway(n)), ev.loc))
		}
	case "nextOccurrence":
		// #266: next monthly / yearly repeat on or after today (clamped to the month end)
		if d, ok := date(0); ok && len(vals) >= 2 && vals[1].Kind == String {
			if r, ok := nextOccurrence(d, vals[1].Str, ev.now, ev.loc); ok {
				return DateValue(r)
			}
		}
	case "hoursBetween":
		a, ok1 := secondsOfDay(0)
		b, ok2 := secondsOfDay(1)
		if ok1 && ok2 {
			diff := b - a
			if diff < 0 {
				diff += 86400
			}
			return NumberValue(diff / 3600)
		}
	case "year", "month", "day", "weekday":
		d, ok := date(0)
		if !ok {
			return Value{}
		}
		l := d.In(ev.loc)
		switch name {
		case "year":
			return NumberValue(float64(l.Year()))
		case "month":
			return NumberValue(float64(l.Month()))
		case "day":
			return NumberValue(float64(l.Day()))
		}
		return NumberValue(float64(l.Weekday()) + 1) // 1 = Sunday
	// conversion
	case "toNumber":
		if len(vals) == 0 {
			return Value{}
		}
		if vals[0].Kind == Bool {
			if vals[0].Bool {
				return NumberValue(1)
			}
			return NumberValue(0)
		}
		if n, ok := vals[0].number(); ok {
			return NumberValue(n)
		}
	case "toString":
		if len(vals) > 0 && !vals[0].IsNil() {
			return StringValue(vals[0].Text(ev.loc))
		}
	}
	return Value{}
}

// pow10 is Foundation.pow(10, n) for the integral n that round/floor/ceil use.
func pow10(n float64) float64 {
	if n >= -323 && n <= 308 {
		return math.Pow10(int(n))
	}
	return math.Pow(10, n)
}

// equal is ExprEvaluator.equal: nil == nil; numbers and dates by value;
// otherwise as numbers when both convert, else as text.
func (ev *Evaluator) equal(a, b Value) bool {
	switch {
	case a.IsNil() && b.IsNil():
		return true
	case a.IsNil() || b.IsNil():
		return false
	case a.Kind == Number && b.Kind == Number:
		return a.Num == b.Num
	case a.Kind == Date && b.Kind == Date:
		return a.Time.Equal(b.Time)
	}
	// a date and a date string compare as dates (other strings as text)
	if d, s, ok := dateAndString(a, b); ok {
		if t, ok := parseDate(s, ev.loc); ok {
			return d.Equal(t)
		}
	}
	if x, ok := a.number(); ok {
		if y, ok := b.number(); ok {
			return x == y
		}
	}
	return a.Text(ev.loc) == b.Text(ev.loc)
}

// compare is ExprEvaluator.compare: dates with dates or date strings; two
// strings as text unless both parse as numbers (commas removed); otherwise
// numbers.
func (ev *Evaluator) compare(a, b Value) (int, bool) {
	if a.Kind == Date && b.Kind == Date {
		return a.Time.Compare(b.Time), true
	}
	if d, s, ok := dateAndString(a, b); ok {
		t, ok := parseDate(s, ev.loc)
		if !ok {
			return 0, false
		}
		if a.Kind == Date {
			return d.Compare(t), true
		}
		return t.Compare(d), true
	}
	if a.Kind == String && b.Kind == String {
		_, okA := parseNumber(a.Str)
		_, okB := parseNumber(b.Str)
		if !okA || !okB {
			return compareUTF16(a.Str, b.Str), true
		}
	}
	x, ok1 := a.number()
	y, ok2 := b.number()
	if !ok1 || !ok2 {
		return 0, false
	}
	return cmpFloat(x, y), true
}

// dateAndString returns the date and the string when one is a date and the
// other a string.
func dateAndString(a, b Value) (time.Time, string, bool) {
	switch {
	case a.Kind == Date && b.Kind == String:
		return a.Time, b.Str, true
	case a.Kind == String && b.Kind == Date:
		return b.Time, a.Str, true
	}
	return time.Time{}, "", false
}

func cmpFloat(x, y float64) int {
	switch {
	case x == y:
		return 0
	case x < y:
		return -1
	}
	return 1
}

// compareUTF16 orders like NSString.compare without options (UTF-16 code units).
func compareUTF16(a, b string) int {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(x) < len(y):
		return -1
	case len(x) > len(y):
		return 1
	}
	return 0
}
