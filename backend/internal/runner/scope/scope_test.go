package scope

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func app() *dsl.App {
	return &dsl.App{Entities: []dsl.Entity{
		{ID: "rsvps", Scope: dsl.ScopeShared, Fields: []dsl.Field{{ID: "event", Type: dsl.FieldRef, RefEntity: "events"}}},
		{ID: "events", Scope: dsl.ScopeShared, Fields: []dsl.Field{{ID: "n", Type: dsl.FieldInteger}}},
	}}
}

func seed(t *testing.T, st *store.Memory, appID, id string) {
	t.Helper()
	if _, _, err := st.CreateRecord(context.Background(), store.PublicRecords(appID, "events"), store.Record{ID: id, Data: map[string]any{"n": 1}, Author: "a-" + appID}); err != nil {
		t.Fatal(err)
	}
}

// #337 layer 1-3: a handle of X reads only X's trigger record and its ref
// targets; Y's record with the same id is out of reach; a path outside X is
// refused and logged.
func TestHandleBoundToApp(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	seed(t, st, "X", "e1")
	seed(t, st, "Y", "e1")
	seed(t, st, "Y", "eY")
	var logs bytes.Buffer
	h, err := New(st, "test", store.OutboxEvent{AppID: "X", Scope: "shared", Entity: "rsvps", RecordID: "r1"}, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.Record(ctx, "events", "e1"); !errors.Is(err, ErrNotBound) {
		t.Fatalf("unbound read: %v", err)
	}
	h.Bind(app(), "rsvps", "r1", map[string]any{"event": "e1"})

	rec, found, err := h.Record(ctx, "events", "e1")
	if err != nil || !found || rec.Author != "a-X" {
		t.Fatalf("ref target: %+v %v %v", rec, found, err)
	}
	// Y's records are not readable, even by an id of X's shape
	if _, _, err := h.Record(ctx, "events", "eY"); !errors.Is(err, ErrNotReadable) {
		t.Fatalf("other record: %v", err)
	}
	if _, err := h.ApplyOnce(ctx, "events", "eY", "k", func(d map[string]any) map[string]any { d["n"] = 99; return d }); !errors.Is(err, ErrNotReadable) {
		t.Fatalf("write other record: %v", err)
	}
	applied, err := h.ApplyOnce(ctx, "events", "e1", "k1", func(d map[string]any) map[string]any { d["n"] = 2; return d })
	if err != nil || !applied {
		t.Fatalf("apply: %v %v", applied, err)
	}
	if again, err := h.ApplyOnce(ctx, "events", "e1", "k1", func(d map[string]any) map[string]any { d["n"] = 3; return d }); err != nil || again {
		t.Fatalf("apply twice: %v %v", again, err)
	}
	y, _ := st.GetRecord(ctx, store.PublicRecords("Y", "events"), "e1")
	x, _ := st.GetRecord(ctx, store.PublicRecords("X", "events"), "e1")
	if y.Data["n"] != 1 || x.Data["n"] != 2 {
		t.Fatalf("X %v / Y %v", x.Data, y.Data)
	}
	if author, found, err := h.RecordAuthor(ctx, "events", "e1"); err != nil || !found || author != "a-X" {
		t.Fatalf("author: %q %v %v", author, found, err)
	}

	// the single path check: a set of another app (or the private side) is refused and logged
	for _, rs := range []store.RecordSet{store.PublicRecords("Y", "events"), store.PrivateRecords("u", "X", "events")} {
		if err := h.check(ctx, rs); !errors.Is(err, ErrViolation) {
			t.Fatalf("check %+v: %v", rs, err)
		}
	}
	if !strings.Contains(logs.String(), ViolationEvent) || strings.Contains(logs.String(), `"u"`) {
		t.Fatalf("violation log: %s", logs.String())
	}
	if _, err := h.recordSet(ctx, "nope"); err == nil {
		t.Fatal("unknown entity accepted")
	}
	if _, err := h.PrivateApp(ctx); !errors.Is(err, ErrViolation) {
		t.Fatalf("shared handle read a private app: %v", err)
	}
}

// private handles read and write the user's own tree only.
func TestHandlePrivate(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	if _, _, err := st.EnsureApp(ctx, "u", "P", []byte(`{}`), "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := New(st, "test", store.OutboxEvent{AppID: "P", Scope: "private"}, nil); err == nil {
		t.Fatal("private handle without uid")
	}
	h, err := New(st, "test", store.OutboxEvent{AppID: "P", Scope: "private", UID: "u", Entity: "memo", RecordID: "m"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.Bind(&dsl.App{Entities: []dsl.Entity{{ID: "memo"}, {ID: "log"}}}, "memo", "m", nil)
	if owner, _ := h.Owner(ctx); owner != "u" {
		t.Fatalf("owner %q", owner)
	}
	if ms, _ := h.Members(ctx); len(ms) != 0 {
		t.Fatalf("members %v", ms)
	}
	created, err := h.CreateRecord(ctx, "log", "l1", map[string]any{"x": 1}, store.RecordOrigin{RuleID: "r", EventID: "e"})
	if err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	got, err := st.GetRecord(ctx, store.PrivateRecords("u", "P", "log"), "l1")
	if err != nil || got.Origin == nil || got.Origin.RuleID != "r" {
		t.Fatalf("created record: %+v %v", got, err)
	}
	if err := h.check(ctx, store.PrivateRecords("v", "P", "log")); !errors.Is(err, ErrViolation) {
		t.Fatalf("other user's tree: %v", err)
	}
}
