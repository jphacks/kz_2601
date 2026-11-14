package appconfig

import (
	"reflect"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/plan"
)

// The three per-hour readers share one rule: a non-negative int is taken
// as-is, anything else falls back to the default.
func TestPerHourFromEnvTable2586(t *testing.T) {
	readers := []struct {
		name string
		key  string
		def  int
		read func(func(string) string) int
	}{
		{"bookLookupsPerHour", "BOOK_LOOKUP_USER_PER_HOUR", DefaultBookLookupsPerHour, bookLookupsPerHour},
		{"extPerHour", "EXT_USER_PER_HOUR", DefaultExtPerHour, extPerHour},
		{"feedsPerHour", "FEEDS_USER_PER_HOUR", DefaultFeedsPerHour, feedsPerHour},
	}
	cases := []struct {
		name  string
		val   string
		unset bool
		want  int // -1 = the reader's default
	}{
		{name: "unset", unset: true, want: -1},
		{name: "empty", val: "", want: -1},
		{name: "zero turns it off", val: "0", want: 0},
		{name: "one", val: "1", want: 1},
		{name: "plus sign", val: "+7", want: 7},
		{name: "large", val: "1000000", want: 1000000},
		{name: "negative", val: "-1", want: -1},
		{name: "decimal", val: "3.5", want: -1},
		{name: "word", val: "many", want: -1},
		{name: "hex", val: "0x10", want: -1},
		{name: "padded", val: " 5", want: -1},
		{name: "overflow", val: "99999999999999999999", want: -1},
	}
	for _, rd := range readers {
		for _, tc := range cases {
			t.Run(rd.name+"/"+tc.name, func(t *testing.T) {
				getenv := func(k string) string {
					if k != rd.key {
						t.Errorf("read %q, want %q", k, rd.key)
					}
					if tc.unset {
						return ""
					}
					return tc.val
				}
				want := tc.want
				if want == -1 {
					want = rd.def
				}
				if got := rd.read(getenv); got != want {
					t.Fatalf("%s(%q) = %d, want %d", rd.name, tc.val, got, want)
				}
			})
		}
	}
}

// Other keys do not leak into a reader.
func TestPerHourIgnoresOtherKeys2586(t *testing.T) {
	all := map[string]string{"BOOK_LOOKUP_USER_PER_HOUR": "3", "EXT_USER_PER_HOUR": "4", "FEEDS_USER_PER_HOUR": "5"}
	getenv := func(k string) string { return all[k] }
	if got := bookLookupsPerHour(getenv); got != 3 {
		t.Errorf("books = %d", got)
	}
	if got := extPerHour(getenv); got != 4 {
		t.Errorf("ext = %d", got)
	}
	if got := feedsPerHour(getenv); got != 5 {
		t.Errorf("feeds = %d", got)
	}
}

func TestPlanTableTable2586(t *testing.T) {
	custom := plan.Default
	custom.Free.RecordsPerApp = 7
	cases := []struct {
		name        string
		limits      *plan.Table
		wantTable   plan.Table
		wantCounted bool
	}{
		{name: "not set: default, not counted", limits: nil, wantTable: plan.Default, wantCounted: false},
		{name: "enforce", limits: withMode2586(custom, plan.ModeEnforce), wantTable: withModeVal2586(custom, plan.ModeEnforce), wantCounted: true},
		{name: "log still counts", limits: withMode2586(custom, plan.ModeLog), wantTable: withModeVal2586(custom, plan.ModeLog), wantCounted: true},
		{name: "off", limits: withMode2586(custom, plan.ModeOff), wantTable: withModeVal2586(custom, plan.ModeOff), wantCounted: false},
		{name: "zero mode counts as enforce", limits: withMode2586(custom, ""), wantTable: withModeVal2586(custom, ""), wantCounted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{planLimits: tc.limits}
			got, counted := h.planTable()
			if counted != tc.wantCounted {
				t.Errorf("counted = %v, want %v", counted, tc.wantCounted)
			}
			if !reflect.DeepEqual(got, tc.wantTable) {
				t.Errorf("table = %+v, want %+v", got, tc.wantTable)
			}
		})
	}
}

// planTable hands out a copy: changing it leaves the handler's table alone.
func TestPlanTableReturnsCopy2586(t *testing.T) {
	tbl := withMode2586(plan.Default, plan.ModeEnforce)
	h := &Handler{planLimits: tbl}
	got, _ := h.planTable()
	got.Free.RecordsPerApp = -42
	if h.planLimits.Free.RecordsPerApp == -42 {
		t.Fatal("planTable returned the handler's own table")
	}
}

func withModeVal2586(t plan.Table, m plan.Mode) plan.Table {
	t.Mode = m
	return t
}

func withMode2586(t plan.Table, m plan.Mode) *plan.Table {
	v := withModeVal2586(t, m)
	return &v
}
