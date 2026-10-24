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

	rs := PrivateRecords("u1", "a1", "expense")
	r, created, err := m.CreateRecord(ctx, rs, Record{ID: "r1", Data: map[string]any{"amount": int64(1)}})
	if err != nil || !created || r.CreatedAt.IsZero() {
		t.Fatalf("create: %v %+v", err, r)
	}
	if _, created, _ = m.CreateRecord(ctx, rs, Record{ID: "r1"}); created {
		t.Fatal("same id must be idempotent")
	}
	if _, err := m.UpdateRecord(ctx, rs, "r1", map[string]any{"amount": int64(2)}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetRecord(ctx, rs, "r1")
	if got.Data["amount"] != int64(2) {
		t.Fatalf("update not applied: %+v", got)
	}
	if _, err := m.GetRecord(ctx, PrivateRecords("u2", "a1", "expense"), "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other uid must not see private records: %v", err)
	}
	if err := m.DeleteRecord(ctx, rs, "r1"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteRecord(ctx, rs, "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestMemoryPublicAppsAndRecords(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	initial, _ := json.Marshal(dsl.Initial())

	if _, err := m.GetPublicApp(ctx, "a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unpublished: %v", err)
	}
	if ok, err := m.PublishApp(ctx, "u1", "a1", initial, "board", 2); err != nil || !ok {
		t.Fatalf("publish: %v %v", ok, err)
	}
	if ok, _ := m.PublishApp(ctx, "u2", "a1", initial, "hijack", 1); ok {
		t.Fatal("another uid must not overwrite the published app")
	}
	if ok, _ := m.PublishApp(ctx, "u1", "a1", initial, "board2", 3); !ok {
		t.Fatal("owner must be able to republish")
	}
	pub, err := m.GetPublicApp(ctx, "a1")
	if err != nil || pub.OwnerUID != "u1" || pub.Name != "board2" || pub.Version != 3 || string(pub.DSL) != string(initial) {
		t.Fatalf("public app: %v %+v", err, pub)
	}

	// public records need no owning app document and are visible to every uid.
	rs := PublicRecords("a1", "post")
	if _, _, err := m.CreateRecord(ctx, rs, Record{ID: "p1", Author: "u1", AuthorName: "A", Data: map[string]any{"text": "hi"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.CreateRecord(ctx, PublicRecords("a2", "post"), Record{ID: "p1", Author: "u2"}); err != nil {
		t.Fatal(err)
	}
	list, _ := m.ListRecords(ctx, rs, 10)
	if len(list) != 1 || list[0].Author != "u1" || list[0].AuthorName != "A" || list[0].Entity != "post" {
		t.Fatalf("public list: %+v", list)
	}
	if _, err := m.ListRecords(ctx, PrivateRecords("u1", "a1", "post"), 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private set of an unknown app must be not found: %v", err)
	}
	if err := m.DeleteAllRecords(ctx, rs); err != nil {
		t.Fatal(err)
	}
	if list, _ = m.ListRecords(ctx, rs, 10); len(list) != 0 {
		t.Fatalf("after delete all: %+v", list)
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
