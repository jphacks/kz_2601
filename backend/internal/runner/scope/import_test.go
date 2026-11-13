package scope

import (
	"context"
	"errors"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func setN(n int) func(map[string]any) map[string]any {
	return func(d map[string]any) map[string]any { d["n"] = n; return d }
}

// #1712: UpsertRecord / ImportedRecord skip the readable check, so the
// app/side binding is their only guard. Writes and reads stay in X even
// when Y has a record with the same id.
func TestHandleImportStaysInApp(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	seed(t, st, "Y", "e1")
	h, err := New(st, "test", store.OutboxEvent{AppID: "X", Scope: "shared", Kind: store.OutboxSchedule}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.UpsertRecord(ctx, "events", "e1", map[string]any{"n": 5}, store.RecordOrigin{}, "k0", setN(0)); !errors.Is(err, ErrNotBound) {
		t.Fatalf("unbound upsert: %v", err)
	}
	if _, _, err := h.ImportedRecord(ctx, "events", "e1"); !errors.Is(err, ErrNotBound) {
		t.Fatalf("unbound read: %v", err)
	}
	h.Bind(app(), "", "", nil)

	if _, found, err := h.ImportedRecord(ctx, "events", "e1"); err != nil || found {
		t.Fatalf("Y's record seen from X: %v %v", found, err)
	}
	// data nil on a missing record: left missing
	if created, applied, err := h.UpsertRecord(ctx, "events", "e1", nil, store.RecordOrigin{}, "k1", setN(9)); err != nil || created || applied {
		t.Fatalf("update-only on missing: %v %v %v", created, applied, err)
	}
	if _, err := st.GetRecord(ctx, store.PublicRecords("X", "events"), "e1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update-only created a record: %v", err)
	}

	origin := store.RecordOrigin{RuleID: "import", EventID: "ev"}
	if created, applied, err := h.UpsertRecord(ctx, "events", "e1", map[string]any{"n": 5}, origin, "k2", setN(0)); err != nil || !created || applied {
		t.Fatalf("create: %v %v %v", created, applied, err)
	}
	data, found, err := h.ImportedRecord(ctx, "events", "e1")
	if err != nil || !found || data["n"] != 5 {
		t.Fatalf("imported: %v %v %v", data, found, err)
	}
	// an existing record gets fn once per key
	if created, applied, err := h.UpsertRecord(ctx, "events", "e1", map[string]any{"n": 5}, origin, "k3", setN(6)); err != nil || created || !applied {
		t.Fatalf("update: %v %v %v", created, applied, err)
	}
	if created, applied, err := h.UpsertRecord(ctx, "events", "e1", map[string]any{"n": 5}, origin, "k3", setN(7)); err != nil || created || applied {
		t.Fatalf("same key twice: %v %v %v", created, applied, err)
	}
	x, _ := st.GetRecord(ctx, store.PublicRecords("X", "events"), "e1")
	y, _ := st.GetRecord(ctx, store.PublicRecords("Y", "events"), "e1")
	if x.Data["n"] != 6 || y.Data["n"] != 1 || x.Origin == nil || x.Origin.RuleID != "import" {
		t.Fatalf("X %+v / Y %v", x, y.Data)
	}
	if _, _, err := h.UpsertRecord(ctx, "nope", "e1", map[string]any{}, origin, "k4", setN(0)); err == nil {
		t.Fatal("unknown entity accepted")
	}
	if _, _, err := h.ImportedRecord(ctx, "nope", "e1"); err == nil {
		t.Fatal("unknown entity read")
	}
}

// a private handle imports into the user's own tree only.
func TestHandleImportPrivate(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	for _, uid := range []string{"u", "v"} {
		if _, _, err := st.EnsureApp(ctx, uid, "P", []byte(`{}`), "p"); err != nil {
			t.Fatal(err)
		}
	}
	other := store.PrivateRecords("v", "P", "log")
	if _, _, err := st.CreateRecord(ctx, other, store.Record{ID: "l1", Data: map[string]any{"n": 1}}); err != nil {
		t.Fatal(err)
	}
	h, err := New(st, "test", store.OutboxEvent{AppID: "P", Scope: "private", UID: "u", Kind: store.OutboxSchedule}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.Bind(&dsl.App{Entities: []dsl.Entity{{ID: "log"}}}, "", "", nil)
	if _, found, err := h.ImportedRecord(ctx, "log", "l1"); err != nil || found {
		t.Fatalf("other user's record seen: %v %v", found, err)
	}
	if created, _, err := h.UpsertRecord(ctx, "log", "l1", map[string]any{"n": 2}, store.RecordOrigin{}, "k", setN(0)); err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	mine, _ := st.GetRecord(ctx, store.PrivateRecords("u", "P", "log"), "l1")
	theirs, _ := st.GetRecord(ctx, other, "l1")
	if mine == nil || mine.Data["n"] != 2 || theirs.Data["n"] != 1 {
		t.Fatalf("mine %v / theirs %v", mine, theirs.Data)
	}
}

// Mark / Marked are per env.
func TestHandleMark(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	h, _ := New(st, "test", store.OutboxEvent{AppID: "X", Scope: "shared"}, nil)
	other, _ := New(st, "prod", store.OutboxEvent{AppID: "X", Scope: "shared"}, nil)
	if done, err := h.Marked(ctx, "a"); err != nil || done {
		t.Fatalf("before: %v %v", done, err)
	}
	if err := h.Mark(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if done, _ := h.Marked(ctx, "a"); !done {
		t.Fatal("not marked")
	}
	if done, _ := other.Marked(ctx, "a"); done {
		t.Fatal("mark leaked to another env")
	}
}
