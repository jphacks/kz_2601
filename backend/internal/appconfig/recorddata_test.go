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

// #235: the DSL has no array field type. An array sent to any declared field
// is a 422 invalid_record (never a 500 or a silent store), and the workaround
// "several values joined in one string" passes as a plain string.
func TestNormalizeDataRejectsArrays(t *testing.T) {
	ent := &dsl.Entity{ID: "book", Fields: []dsl.Field{
		{ID: "title", Type: dsl.FieldString},
		{ID: "memo", Type: dsl.FieldText},
		{ID: "genre", Type: dsl.FieldEnum, Options: []string{"小説", "技術"}},
		{ID: "author", Type: dsl.FieldRef, RefEntity: "authors"},
		{ID: "pages", Type: dsl.FieldInteger},
		{ID: "read_on", Type: dsl.FieldDate},
	}}
	for _, f := range ent.Fields {
		for _, v := range []any{[]any{"小説", "技術"}, []any{}} {
			_, err := normalizeData(ent, map[string]any{f.ID: v}, false)
			ae, ok := err.(*apiError)
			if !ok || ae.status != 422 || ae.code != "invalid_record" {
				t.Errorf("%s (%s) = %v: want 422 invalid_record, got %v", f.ID, f.Type, v, err)
			}
		}
	}
	out, err := normalizeData(ent, map[string]any{"title": "小説, 技術"}, false)
	if err != nil || out["title"] != "小説, 技術" {
		t.Errorf("joined string: %v %v", out, err)
	}
	// only the undeclared daily-toggle history key ("<date>__dates", #172) takes an array
	if _, err := normalizeData(ent, map[string]any{"read_on__dates": []any{"2026-10-24"}}, false); err != nil {
		t.Errorf("history array: %v", err)
	}
}
