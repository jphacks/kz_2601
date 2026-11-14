package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type outboxLister interface {
	OutboxEvents(ctx context.Context, env string, limit int) ([]OutboxEvent, error)
}

func outboxOf(t *testing.T, s Store, env string) []OutboxEvent {
	t.Helper()
	evs, err := s.(outboxLister).OutboxEvents(t.Context(), env, 0)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func eventsFor(t *testing.T, s Store, env, recordID string) []OutboxEvent {
	t.Helper()
	var out []OutboxEvent
	for _, ev := range outboxOf(t, s, env) {
		if ev.RecordID == recordID {
			out = append(out, ev)
		}
	}
	return out
}

func oneEvent(env, kind string) EmitFunc {
	return func(prev *Record, next Record) []OutboxEvent {
		ev := OutboxEvent{Env: env, Kind: kind, AppID: "A", Scope: "shared", Entity: next.Entity, RecordID: next.ID, Version: 3, RuleIDs: []string{"r1"}, ActorUID: "u", Data: map[string]any{"n": next.Data["n"]}}
		if prev != nil {
			ev.Prev = map[string]any{"n": prev.Data["n"]}
		}
		return []OutboxEvent{ev}
	}
}

// #341: the record and its outbox event are written together.
func TestStoreOutboxEmit(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		env := "test" + sfx
		rs := PublicRecords("OUTBOX"+sfx, "post")
		out, created, err := s.CreateRecordEmit(ctx, rs, Record{ID: "r1", Data: map[string]any{"n": "a"}, Author: "u"}, oneEvent(env, OutboxRecordCreated))
		if err != nil || !created || out.Data["n"] != "a" || out.Author != "u" {
			t.Fatalf("create: %+v %v %v", out, created, err)
		}
		evs := eventsFor(t, s, env, "r1")
		if len(evs) != 1 {
			t.Fatalf("events after create: %+v", evs)
		}
		ev := evs[0]
		if ev.EventID == "" || ev.Kind != OutboxRecordCreated || ev.Status != OutboxPending || ev.Version != 3 || !reflect.DeepEqual(ev.RuleIDs, []string{"r1"}) || ev.Prev != nil ||
			ev.CreatedAt.IsZero() || !ev.AvailableAt.Equal(ev.CreatedAt) || !ev.ExpireAt.Equal(ev.CreatedAt.Add(OutboxTTL)) {
			t.Fatalf("created event: %+v", ev)
		}

		// creating the same id again returns the record and emits nothing
		if _, created, err = s.CreateRecordEmit(ctx, rs, Record{ID: "r1", Data: map[string]any{"n": "b"}}, oneEvent(env, OutboxRecordCreated)); err != nil || created {
			t.Fatalf("idempotent create: %v %v", created, err)
		}
		if n := len(eventsFor(t, s, env, "r1")); n != 1 {
			t.Fatalf("idempotent create emitted: %d events", n)
		}

		// update: emit sees the record before and after
		up, err := s.UpdateRecordEmit(ctx, rs, "r1", map[string]any{"n": "b"}, oneEvent(env, OutboxRecordUpdated))
		if err != nil || up.Data["n"] != "b" || up.Author != "u" {
			t.Fatalf("update: %+v %v", up, err)
		}
		got, err := s.GetRecord(ctx, rs, "r1")
		if err != nil || got.Data["n"] != "b" || !got.UpdatedAt.Equal(up.UpdatedAt) {
			t.Fatalf("read back: %+v %v (returned %+v)", got, err, up)
		}
		var upd *OutboxEvent
		for _, ev := range eventsFor(t, s, env, "r1") {
			if ev.Kind == OutboxRecordUpdated {
				upd = &ev
			}
		}
		if upd == nil || upd.Data["n"] != "b" || upd.Prev["n"] != "a" {
			t.Fatalf("updated event: %+v", upd)
		}

		// an emit with no events writes only the record
		if _, err = s.UpdateRecordEmit(ctx, rs, "r1", map[string]any{"n": "c"}, func(*Record, Record) []OutboxEvent { return nil }); err != nil {
			t.Fatal(err)
		}
		if n := len(eventsFor(t, s, env, "r1")); n != 2 {
			t.Fatalf("empty emit added events: %d", n)
		}
		if _, err = s.UpdateRecordEmit(ctx, rs, "missing", map[string]any{}, oneEvent(env, OutboxRecordUpdated)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing record: %v", err)
		}
	})
}

// #341: when the outbox write fails, the record is not written either.
func TestStoreOutboxAtomic(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		env := "test" + sfx
		rs := PublicRecords("OUTBOX-ATOMIC"+sfx, "post")
		fixed := func(id string) EmitFunc {
			return func(prev *Record, next Record) []OutboxEvent {
				evs := oneEvent(env, OutboxRecordCreated)(prev, next)
				evs[0].EventID = id
				return evs
			}
		}
		if _, _, err := s.CreateRecordEmit(ctx, rs, Record{ID: "r1", Data: map[string]any{"n": "a"}}, fixed("evt_dup")); err != nil {
			t.Fatal(err)
		}

		// the event id is taken: neither the second record nor its event is stored
		if _, _, err := s.CreateRecordEmit(ctx, rs, Record{ID: "r2", Data: map[string]any{"n": "x"}}, fixed("evt_dup")); !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("duplicate event: want ErrAlreadyExists, got %v", err)
		}
		if _, err := s.GetRecord(ctx, rs, "r2"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("record written without its event: %v", err)
		}
		// an update whose event fails keeps the old data
		if _, err := s.UpdateRecordEmit(ctx, rs, "r1", map[string]any{"n": "z"}, fixed("evt_dup")); !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("duplicate event on update: %v", err)
		}
		if r, err := s.GetRecord(ctx, rs, "r1"); err != nil || r.Data["n"] != "a" {
			t.Fatalf("update applied without its event: %+v %v", r, err)
		}
		// a malformed event (no env) fails the write the same way
		if _, _, err := s.CreateRecordEmit(ctx, rs, Record{ID: "r3", Data: map[string]any{}}, oneEvent("", OutboxRecordCreated)); !errors.Is(err, ErrOutboxEvent) {
			t.Fatalf("bad event: %v", err)
		}
		if _, err := s.GetRecord(ctx, rs, "r3"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("record written with a bad event: %v", err)
		}
		if evs := outboxOf(t, s, env); len(evs) != 1 || evs[0].EventID != "evt_dup" || evs[0].RecordID != "r1" {
			t.Fatalf("outbox: %+v", evs)
		}
	})
}
