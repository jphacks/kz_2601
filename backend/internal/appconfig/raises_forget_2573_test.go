package appconfig

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/plan"
)

func TestRaises2573(t *testing.T) {
	cases := []struct {
		name      string
		free, pro plan.Limits
		resource  string
		want      bool
	}{
		{"same limit", plan.Limits{RecordsPerApp: 100}, plan.Limits{RecordsPerApp: 100}, plan.RecordsPerApp, false},
		{"pro has more", plan.Limits{RecordsPerApp: 100}, plan.Limits{RecordsPerApp: 1000}, plan.RecordsPerApp, true},
		{"pro has less", plan.Limits{PhotosPerApp: 100}, plan.Limits{PhotosPerApp: 10}, plan.PhotosPerApp, false},
		{"pro not counted", plan.Limits{PhotosPerApp: 100}, plan.Limits{}, plan.PhotosPerApp, true},
		{"free not counted", plan.Limits{}, plan.Limits{RecordsPerApp: 1000}, plan.RecordsPerApp, false},
		{"unknown resource", plan.Limits{RecordsPerApp: 100}, plan.Limits{RecordsPerApp: 1000}, "unknown", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := raises(plan.Table{Free: c.free, Pro: c.pro}, c.resource); got != c.want {
				t.Fatalf("raises = %v, want %v", got, c.want)
			}
		})
	}
}

func TestActivityForget2573(t *testing.T) {
	var a activity
	const day = "2026-11-13"
	if !a.first(day, "k") {
		t.Fatal("first open of the day is not first")
	}
	if a.first(day, "k") {
		t.Fatal("second open of the day is first")
	}
	a.forget("k")
	if !a.first(day, "k") {
		t.Fatal("forgotten key is not first again")
	}
	if a.first(day, "k") {
		t.Fatal("key after re-marking is first")
	}
	// Forgetting one key leaves the others; forgetting an unknown key is a no-op.
	if !a.first(day, "other") {
		t.Fatal("other key is not first")
	}
	a.forget("k")
	a.forget("never-seen")
	if a.first(day, "other") {
		t.Fatal("forget removed another key")
	}
}
