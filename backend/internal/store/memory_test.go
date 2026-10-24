package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestMemoryVersionsAndRecords(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	initial, _ := json.Marshal(dsl.Initial())

	app, created, err := m.EnsureApp(ctx, "u1", "a1", initial, "x")
	if err != nil || !created || app.CurrentVersion != 1 {
		t.Fatalf("ensure: %v created=%v app=%+v", err, created, app)
	}
	if _, created, _ = m.EnsureApp(ctx, "u1", "a1", initial, "x"); created {
		t.Fatal("second ensure must not create")
	}
	if _, err := m.GetApp(ctx, "u2", "a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other uid must not see app: %v", err)
	}

	if _, err := m.CommitVersion(ctx, "u1", "a1", Commit{BaseVersion: 5, DSL: initial}); err == nil {
		t.Fatal("stale base must conflict")
	} else {
		var ce *ConflictError
		if !errors.As(err, &ce) || ce.Current != 1 {
			t.Fatalf("want ConflictError{1}, got %v", err)
		}
	}
	v, err := m.CommitVersion(ctx, "u1", "a1", Commit{BaseVersion: 1, DSL: initial, Name: "n2", Source: &Source{Kind: "manual"}})
	if err != nil || v.Version != 2 || v.ParentVersion != 1 {
		t.Fatalf("commit: %v %+v", err, v)
	}
	vs, _ := m.ListVersions(ctx, "u1", "a1", 10, 0)
	if len(vs) != 2 || vs[0].Version != 2 {
		t.Fatalf("versions: %+v", vs)
	}

	r, created, err := m.CreateRecord(ctx, "u1", "a1", "expense", Record{ID: "r1", Data: map[string]any{"amount": int64(1)}})
	if err != nil || !created || r.CreatedAt.IsZero() {
		t.Fatalf("create: %v %+v", err, r)
	}
	if _, created, _ = m.CreateRecord(ctx, "u1", "a1", "expense", Record{ID: "r1"}); created {
		t.Fatal("same id must be idempotent")
	}
	if _, err := m.UpdateRecord(ctx, "u1", "a1", "expense", "r1", map[string]any{"amount": int64(2)}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetRecord(ctx, "u1", "a1", "expense", "r1")
	if got.Data["amount"] != int64(2) {
		t.Fatalf("update not applied: %+v", got)
	}
	if err := m.DeleteRecord(ctx, "u1", "a1", "expense", "r1"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteRecord(ctx, "u1", "a1", "expense", "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestIDs(t *testing.T) {
	a, b := NewID("rec_"), NewID("rec_")
	if a == b || len(a) != 30 {
		t.Fatalf("ids: %s %s", a, b)
	}
	a1, a2 := IDFromClientID("c1"), IDFromClientID("c1")
	if a1 != a2 || a1 == IDFromClientID("c2") {
		t.Fatal("client id derivation must be stable and distinct")
	}
}
