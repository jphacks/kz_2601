package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #1774: a restore commit keeps restoredFrom, a normal save has none, and
// ListVersions returns both as committed.
func TestMemoryRestoredFrom(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	initial, _ := json.Marshal(dsl.Initial())
	if _, _, err := m.EnsureApp(ctx, "u1", "a1", initial, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CommitVersion(ctx, "u1", "a1", Commit{BaseVersion: 1, DSL: initial, Source: &Source{Kind: "manual", Mode: "full"}}); err != nil {
		t.Fatal(err)
	}
	v, err := m.CommitVersion(ctx, "u1", "a1", Commit{BaseVersion: 2, DSL: initial, Source: &Source{Kind: "restore", Mode: "full", RestoredFrom: 1}})
	if err != nil || v.Source == nil || v.Source.RestoredFrom != 1 {
		t.Fatalf("restore commit: %v %+v", err, v)
	}
	vs, err := m.ListVersions(ctx, "u1", "a1", 10, 0)
	if err != nil || len(vs) != 3 {
		t.Fatalf("list: %v %+v", err, vs)
	}
	if s := vs[0].Source; vs[0].Version != 3 || s == nil || s.Kind != "restore" || s.RestoredFrom != 1 {
		t.Fatalf("v3 must be restore from 1: %+v", vs[0])
	}
	if s := vs[1].Source; vs[1].Version != 2 || s == nil || s.RestoredFrom != 0 {
		t.Fatalf("v2 is a normal save: %+v", vs[1])
	}
}
