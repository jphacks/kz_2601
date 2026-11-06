package scope

import (
	"context"
	"errors"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #340 reads: List reads only the handle's app (and side), caps the count,
// and makes the listed records and their ref targets readable.
func TestHandleList(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	seed(t, st, "X", "e1")
	seed(t, st, "Y", "eY")
	if _, _, err := st.CreateRecord(ctx, store.PublicRecords("X", "rsvps"), store.Record{ID: "r9", Data: map[string]any{"event": "e1"}}); err != nil {
		t.Fatal(err)
	}
	h, err := New(st, "test", store.OutboxEvent{AppID: "X", Scope: "shared", Kind: store.OutboxSchedule}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.List(ctx, "events", 0); !errors.Is(err, ErrNotBound) {
		t.Fatalf("unbound: %v", err)
	}
	h.Bind(app(), "", "", nil)
	if _, _, err := h.Record(ctx, "rsvps", "r9"); !errors.Is(err, ErrNotReadable) {
		t.Fatalf("before List: %v", err)
	}
	recs, err := h.List(ctx, "rsvps", 10)
	if err != nil || len(recs) != 1 || recs[0].ID != "r9" {
		t.Fatalf("list: %+v %v", recs, err)
	}
	if _, found, err := h.Record(ctx, "events", "e1"); err != nil || !found {
		t.Fatalf("ref target of a listed record: %v %v", found, err)
	}
	if _, _, err := h.Record(ctx, "events", "eY"); !errors.Is(err, ErrNotReadable) {
		t.Fatalf("other app: %v", err)
	}
	seed(t, st, "X", "e2")
	if _, err := h.List(ctx, "events", 1); !errors.Is(err, ErrTooMany) {
		t.Fatalf("cap: %v", err)
	}
	// a private handle cannot list shared data
	p, _ := New(st, "test", store.OutboxEvent{AppID: "X", Scope: "private", UID: "u1", Kind: store.OutboxSchedule}, nil)
	p.Bind(&dsl.App{Entities: app().Entities}, "", "", nil)
	if _, err := p.List(ctx, "events", 0); !errors.Is(err, ErrViolation) {
		t.Fatalf("private handle listing shared: %v", err)
	}
}
