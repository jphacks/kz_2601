package appconfig

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #212: locationPicker saves the picked place as the same "lat,lng" string as
// locationButton (iOS formats it with %.6f) plus the place name in a string
// field. Neither may be rejected with 422 (#205: a missing type case did that).
func TestNormalizeDataLocationPicker(t *testing.T) {
	ent := &dsl.Entity{ID: "spot", Fields: []dsl.Field{
		{ID: "name", Type: dsl.FieldString},
		{ID: "place", Type: dsl.FieldLocation, Required: true},
	}}
	for _, tc := range []struct{ in, want string }{
		{"35.681236,139.767125", "35.681236,139.767125"},   // 東京駅 (search result)
		{"-33.856784,151.215297", "-33.856784,151.215297"}, // southern / eastern
		{"40.758000,-73.985500", "40.758,-73.9855"},        // western; trailing zeros trimmed
		{"36.578100, 136.648600", "36.5781,136.6486"},      // space after comma (map pin)
	} {
		out, err := normalizeData(ent, map[string]any{"name": "東京駅", "place": tc.in}, true)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", tc.in, err)
		}
		if out["place"] != tc.want || out["name"] != "東京駅" {
			t.Errorf("%q: got place=%v name=%v, want %q", tc.in, out["place"], out["name"], tc.want)
		}
	}
	for _, bad := range []string{"35.68", "91,0", "0,181", "東京駅", ""} {
		if _, err := normalizeData(ent, map[string]any{"place": bad}, true); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}
