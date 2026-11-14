package store

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"
)

// #2563: Memory.AgeConfirmed for confirmed / unconfirmed / unknown users.
func TestMemoryAgeConfirmedCases(t *testing.T) {
	ctx := t.Context()
	m := NewMemory()
	if err := m.ConfirmAge(ctx, "adult"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SeeUser(ctx, "minor"); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		uid  string
		want bool
	}{
		{"confirmed", "adult", true},
		{"seen but not confirmed", "minor", false},
		{"unknown user", "nobody", false},
		{"empty uid", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := m.AgeConfirmed(ctx, c.uid)
			if err != nil || got != c.want {
				t.Fatalf("AgeConfirmed(%q) = %v %v, want %v", c.uid, got, err, c.want)
			}
		})
	}
	// confirming again keeps it confirmed
	if err := m.ConfirmAge(ctx, "adult"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.AgeConfirmed(ctx, "adult"); !ok {
		t.Fatal("reconfirm dropped the flag")
	}
}

// #2563: Memory.ListApps orders by updatedAt desc, pages, drops purged apps
// and never shows another user's apps.
func TestMemoryListAppsCases(t *testing.T) {
	ctx := t.Context()
	m := NewMemory()
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { at = at.Add(time.Minute); return at }
	dsl := json.RawMessage(`{}`)
	for _, id := range []string{"A1", "A2", "A3", "A4"} {
		if _, _, err := m.EnsureApp(ctx, "u", id, dsl, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := m.EnsureApp(ctx, "other", "B1", dsl, "B1"); err != nil {
		t.Fatal(err)
	}
	// touching A1 makes it the newest
	if _, err := m.UpdateAppMeta(ctx, "u", "A1", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PurgeApp(ctx, "u", "A3"); err != nil {
		t.Fatal(err)
	}

	ids := func(apps []App) []string {
		out := []string{}
		for _, a := range apps {
			out = append(out, a.ID)
		}
		return out
	}
	cases := []struct {
		name          string
		uid           string
		limit, offset int
		want          []string
	}{
		{"all, newest first", "u", 0, 0, []string{"A1", "A4", "A2"}},
		{"limit", "u", 2, 0, []string{"A1", "A4"}},
		{"offset", "u", 0, 1, []string{"A4", "A2"}},
		{"limit and offset", "u", 1, 2, []string{"A2"}},
		{"offset past end", "u", 10, 3, []string{}},
		{"other user", "other", 0, 0, []string{"B1"}},
		{"unknown user", "nobody", 0, 0, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := m.ListApps(ctx, c.uid, c.limit, c.offset)
			if err != nil {
				t.Fatal(err)
			}
			if g := ids(got); !reflect.DeepEqual(g, c.want) {
				t.Fatalf("ListApps = %v, want %v", g, c.want)
			}
		})
	}

	// the listed apps are copies: changing them does not touch the store
	got, err := m.ListApps(ctx, "u", 0, 0)
	if err != nil || len(got) == 0 {
		t.Fatalf("list: %v %v", got, err)
	}
	got[0].Name = "changed"
	again, err := m.ListApps(ctx, "u", 0, 0)
	if err != nil || again[0].Name != "A1" {
		t.Fatalf("store changed through the result: %+v %v", again, err)
	}
}

// #2563: VersionDocID pads to 8 digits so ids sort like the numbers.
func TestVersionDocIDFormat(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "00000000"},
		{1, "00000001"},
		{42, "00000042"},
		{12345678, "12345678"},
		{123456789, "123456789"}, // wider than 8 digits is not cut
	}
	for _, c := range cases {
		if got := VersionDocID(c.n); got != c.want {
			t.Errorf("VersionDocID(%d) = %q, want %q", c.n, got, c.want)
		}
	}
	ns := []int{10, 2, 100, 1, 9}
	docIDs := make([]string, len(ns))
	for i, n := range ns {
		docIDs[i] = VersionDocID(n)
	}
	sort.Ints(ns)
	sort.Strings(docIDs)
	for i, n := range ns {
		if docIDs[i] != VersionDocID(n) {
			t.Fatalf("string order %v differs from number order %v", docIDs, ns)
		}
	}
}
