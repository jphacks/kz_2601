package appconfig

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// jst is used for "now" defaults and thisMonth/today filters (users are in Japan).
var jst = time.FixedZone("JST", 9*3600)

const (
	maxStringLen = 4000
	maxTextLen   = 20000
	dateLayout   = "2006-01-02"
	// maxRefLen bounds a type=ref value (a record id; server ids and iOS "local-<uuid>" are far shorter).
	maxRefLen = 200
)

func invalidRecord(field, msg string) error {
	return apiErr(http.StatusUnprocessableEntity, httpx.CodeInvalidRecord, fmt.Sprintf("field %q %s", field, msg)).With("field", field)
}

// normalizeData validates incoming data against the entity definition.
// create=true applies defaultValue and checks required fields; create=false
// (PATCH) validates only the provided keys, where null removes a key.
// Unknown keys are 400 (openapi), type/required problems are 422.
func normalizeData(ent *dsl.Entity, in map[string]any, create bool) (map[string]any, error) {
	out := make(map[string]any, len(ent.Fields))
	for k, v := range in {
		if ent.FieldByID(k) != nil {
			continue
		}
		if !isHistoryKey(ent, k) {
			return nil, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, fmt.Sprintf("unknown field %q", k)).With("field", k)
		}
		if v == nil {
			if !create {
				out[k] = nil // PATCH removes the history
			}
			continue
		}
		days, err := normalizeHistory(v)
		if err != nil {
			return nil, invalidRecord(k, err.Error())
		}
		out[k] = days
	}
	for i := range ent.Fields {
		f := &ent.Fields[i]
		v, present := in[f.ID]
		if !present {
			if !create {
				continue
			}
			if dv := defaultFor(f); dv != nil {
				out[f.ID] = dv
				continue
			}
			if f.Required {
				return nil, invalidRecord(f.ID, "is required")
			}
			continue
		}
		if v == nil {
			if f.Required {
				return nil, invalidRecord(f.ID, "is required")
			}
			if create {
				continue
			}
			out[f.ID] = nil // sentinel: PATCH removes the key
			continue
		}
		nv, err := normalizeValue(f, v)
		if err != nil {
			return nil, invalidRecord(f.ID, err.Error())
		}
		out[f.ID] = nv
	}
	return out, nil
}

// historySuffix marks the checked-day history iOS keeps next to the date
// field of a list.item.toggle with resets "daily" (#172): "<field>__dates"
// = ["YYYY-MM-DD", ...]. It is not declared in the DSL.
const (
	historySuffix = "__dates"
	maxHistory    = 400
)

// isHistoryKey reports whether k is "<date field>__dates" of ent.
func isHistoryKey(ent *dsl.Entity, k string) bool {
	base, ok := strings.CutSuffix(k, historySuffix)
	if !ok || base == "" {
		return false
	}
	f := ent.FieldByID(base)
	return f != nil && f.Type == dsl.FieldDate && f.Computed == nil
}

// normalizeHistory validates a history array: YYYY-MM-DD strings, sorted,
// de-duplicated, the newest maxHistory kept.
func normalizeHistory(v any) ([]any, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("must be an array of YYYY-MM-DD strings")
	}
	seen := make(map[string]bool, len(arr))
	days := make([]string, 0, len(arr))
	for _, x := range arr {
		s, ok := x.(string)
		if !ok {
			return nil, fmt.Errorf("must be an array of YYYY-MM-DD strings")
		}
		t, err := time.Parse(dateLayout, s)
		if err != nil {
			return nil, fmt.Errorf("has %q; must be YYYY-MM-DD", s)
		}
		d := t.Format(dateLayout)
		if !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
	}
	sort.Strings(days)
	if len(days) > maxHistory {
		days = days[len(days)-maxHistory:]
	}
	out := make([]any, len(days))
	for i, d := range days {
		out[i] = d
	}
	return out, nil
}

func defaultFor(f *dsl.Field) any {
	if f.DefaultValue == nil {
		return nil
	}
	if s, ok := f.DefaultValue.(string); ok && s == "now" {
		switch f.Type {
		case dsl.FieldDate:
			return time.Now().In(jst).Format(dateLayout)
		case dsl.FieldDatetime:
			return time.Now().In(jst).Format(time.RFC3339)
		}
	}
	v, err := normalizeValue(f, f.DefaultValue)
	if err != nil {
		return nil
	}
	return v
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func normalizeValue(f *dsl.Field, v any) (any, error) {
	switch f.Type {
	case dsl.FieldString, dsl.FieldText:
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		limit := maxStringLen
		if f.Type == dsl.FieldText {
			limit = maxTextLen
		}
		if utf8.RuneCountInString(s) > limit {
			return nil, fmt.Errorf("exceeds %d characters", limit)
		}
		return s, nil
	case dsl.FieldInteger, dsl.FieldCurrency:
		n, ok := asFloat(v)
		if !ok || n != math.Trunc(n) || math.Abs(n) > math.MaxInt64/2 {
			return nil, fmt.Errorf("must be an integer")
		}
		return int64(n), nil
	case dsl.FieldNumber:
		n, ok := asFloat(v)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("must be a number")
		}
		return n, nil
	case dsl.FieldBoolean:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("must be a boolean")
		}
		return b, nil
	case dsl.FieldDate:
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("must be a YYYY-MM-DD string")
		}
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t.Format(dateLayout), nil
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.Format(dateLayout), nil
		}
		return nil, fmt.Errorf("must be YYYY-MM-DD")
	case dsl.FieldDatetime:
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("must be an RFC 3339 string")
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.Format(time.RFC3339), nil
		}
		if t, err := time.ParseInLocation(dateLayout, s, jst); err == nil {
			return t.Format(time.RFC3339), nil
		}
		return nil, fmt.Errorf("must be RFC 3339")
	case dsl.FieldEnum:
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		if len(f.Options) > 0 {
			for _, o := range f.Options {
				if o == s {
					return s, nil
				}
			}
			return nil, fmt.Errorf("must be one of %s", strings.Join(f.Options, ", "))
		}
		return s, nil
	case dsl.FieldRef:
		// id of a record of RefEntity. Existence is not checked: iOS may send a
		// not-yet-synced "local-<clientId>" and the target can be deleted later.
		s, ok := asString(v)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("must be a record id string")
		}
		if len(s) > maxRefLen {
			return nil, fmt.Errorf("exceeds %d bytes", maxRefLen)
		}
		return s, nil
	case dsl.FieldPhoto:
		s, ok := asString(v)
		if !ok || (!strings.HasPrefix(s, "data:image/") && !strings.HasPrefix(s, "https://")) {
			return nil, fmt.Errorf("must be a data:image/... URL or https URL")
		}
		return s, nil
	case dsl.FieldLocation:
		s, ok := asString(v)
		if !ok {
			return nil, fmt.Errorf("must be a \"lat,lng\" string")
		}
		parts := strings.Split(s, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("must be \"lat,lng\"")
		}
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			return nil, fmt.Errorf("must be \"lat,lng\" within range")
		}
		return strconv.FormatFloat(lat, 'f', -1, 64) + "," + strconv.FormatFloat(lng, 'f', -1, 64), nil
	}
	return nil, fmt.Errorf("has unsupported type %q", f.Type)
}

// parseFilterValue turns the query-string value into the field's native type.
func parseFilterValue(f *dsl.Field, s string) (any, error) {
	switch f.Type {
	case dsl.FieldInteger, dsl.FieldCurrency, dsl.FieldNumber:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("filterValue must be numeric")
		}
		return n, nil
	case dsl.FieldBoolean:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, fmt.Errorf("filterValue must be a boolean")
		}
		return b, nil
	}
	return s, nil
}

// compareValues orders two record values (numbers, strings, bools). Nil
// sorts last; ok=false when the types are incomparable.
func compareValues(a, b any) (int, bool) {
	if a == nil || b == nil {
		switch {
		case a == nil && b == nil:
			return 0, true
		case a == nil:
			return 1, true
		default:
			return -1, true
		}
	}
	if fa, ok := asFloat(a); ok {
		fb, ok := asFloat(b)
		if !ok {
			return 0, false
		}
		switch {
		case fa < fb:
			return -1, true
		case fa > fb:
			return 1, true
		}
		return 0, true
	}
	if sa, ok := a.(string); ok {
		sb, ok := b.(string)
		if !ok {
			return 0, false
		}
		return strings.Compare(sa, sb), true
	}
	if ba, ok := a.(bool); ok {
		bb, ok := b.(bool)
		if !ok {
			return 0, false
		}
		switch {
		case ba == bb:
			return 0, true
		case !ba:
			return -1, true
		}
		return 1, true
	}
	return 0, false
}

// dateOf extracts a JST calendar date from a date/datetime value.
func dateOf(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	if t, err := time.Parse(dateLayout, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		t = t.In(jst)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
	}
	return time.Time{}, false
}
