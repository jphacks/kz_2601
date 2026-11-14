package store

import (
	"slices"
	"testing"
)

func TestSortedKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   map[string]int
		want []string
	}{
		{"nil map", nil, []string{}},
		{"no keys", map[string]int{}, []string{}},
		{"one key", map[string]int{"apps": 3}, []string{"apps"}},
		{
			"several keys come back in name order, not insertion order",
			map[string]int{"versions": 1, "apps": 2, "records": 3},
			[]string{"apps", "records", "versions"},
		},
		{
			"a zero count is still a key",
			map[string]int{"records": 0, "apps": 0},
			[]string{"apps", "records"},
		},
		{
			"order is by byte, so upper case and digits sort before lower case",
			map[string]int{"apps": 1, "Apps": 1, "1apps": 1, "apps_public": 1},
			[]string{"1apps", "Apps", "apps", "apps_public"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SortedKeys(tc.in)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("SortedKeys(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSortedKeysReturnsAnEmptySliceRatherThanNil(t *testing.T) {
	for _, in := range []map[string]int{nil, {}} {
		if got := SortedKeys(in); got == nil {
			t.Fatalf("SortedKeys(%v) is nil; the stg-cleanup breakdown ranges over it", in)
		}
	}
}

func TestSortedKeysIsStableAcrossCalls(t *testing.T) {
	in := map[string]int{"d": 1, "b": 2, "a": 3, "c": 4, "e": 5, "f": 6, "g": 7, "h": 8}
	first := SortedKeys(in)
	for range 20 {
		if got := SortedKeys(in); !slices.Equal(got, first) {
			t.Fatalf("SortedKeys differs between calls: %v then %v", first, got)
		}
	}
}

func TestSortedKeysLeavesTheMapAlone(t *testing.T) {
	in := map[string]int{"apps": 2, "records": 3}
	_ = SortedKeys(in)
	if len(in) != 2 || in["apps"] != 2 || in["records"] != 3 {
		t.Fatalf("map changed: %v", in)
	}
}
