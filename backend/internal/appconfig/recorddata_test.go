package appconfig

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #172: "<date field>__dates" (checked-day history of a daily toggle) is
// accepted next to the date field without being declared in the DSL.
func TestNormalizeDataDailyHistory(t *testing.T) {
	ent := &dsl.Entity{ID: "medicine", Fields: []dsl.Field{
		{ID: "name", Type: dsl.FieldString},
		{ID: "taken_on", Type: dsl.FieldDate},
	}}

	out, err := normalizeData(ent, map[string]any{
		"taken_on":        "2026-10-24",
		"taken_on__dates": []any{"2026-10-24", "2026-10-22", "2026-10-24"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []any{"2026-10-22", "2026-10-24"}; !reflect.DeepEqual(out["taken_on__dates"], want) {
		t.Errorf("history = %v, want sorted + deduped %v", out["taken_on__dates"], want)
	}

	// PATCH null removes it (unchecking the last day)
	out, err = normalizeData(ent, map[string]any{"taken_on": nil, "taken_on__dates": nil}, false)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := out["taken_on__dates"]; !ok || v != nil {
		t.Errorf("PATCH null should keep a nil sentinel, got %v (present %v)", v, ok)
	}

	// cap: the newest maxHistory days are kept
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	many := make([]any, maxHistory+20)
	for i := range many {
		many[i] = start.AddDate(0, 0, i).Format(dateLayout)
	}
	out, err = normalizeData(ent, map[string]any{"taken_on__dates": many}, false)
	if err != nil {
		t.Fatal(err)
	}
	got := out["taken_on__dates"].([]any)
	if len(got) != maxHistory || got[len(got)-1] != many[len(many)-1] {
		t.Errorf("cap: len %d last %v", len(got), got[len(got)-1])
	}

	bad := []struct {
		data map[string]any
		want string
	}{
		{map[string]any{"taken_on__dates": "2026-10-24"}, "must be an array"},
		{map[string]any{"taken_on__dates": []any{"10/24"}}, "must be YYYY-MM-DD"},
		{map[string]any{"taken_on__dates": []any{1.0}}, "must be an array"},
		{map[string]any{"name__dates": []any{"2026-10-24"}}, `unknown field "name__dates"`}, // not a date field
		{map[string]any{"ghost__dates": []any{"2026-10-24"}}, `unknown field "ghost__dates"`},
		{map[string]any{"__dates": []any{"2026-10-24"}}, `unknown field "__dates"`},
	}
	for _, tc := range bad {
		_, err := normalizeData(ent, tc.data, false)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.data, err, tc.want)
		}
	}
}
