package store

import (
	"context"
	"errors"
	"testing"
)

// #341: a runner write applies once per mark; origin round-trips; the mark
// of a missing record is not set.
func TestStoreRunnerWrites(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		ctx := context.Background()
		w := s.(RunnerWrites)
		env := "rw" + sfx
		rs := PublicRecords("RW"+sfx, "events")
		origin := &RecordOrigin{RuleID: "r", Version: 4, EventID: "evt_1", ActorUID: "u"}
		if _, _, err := s.CreateRecord(ctx, rs, Record{ID: "e1", Data: map[string]any{"n": int64(1)}, Origin: origin}); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetRecord(ctx, rs, "e1")
		if err != nil || got.Origin == nil || *got.Origin != *origin {
			t.Fatalf("origin: %+v %v", got, err)
		}
		inc := func(d map[string]any) map[string]any {
			n, _ := d["n"].(int64)
			d["n"] = n + 1
			return d
		}
		for i, want := range []bool{true, false} {
			applied, err := w.ApplyRecordOnce(ctx, rs, "e1", env, "evt_1_r_0", inc)
			if err != nil || applied != want {
				t.Fatalf("apply %d: %v %v", i, applied, err)
			}
		}
		got, _ = s.GetRecord(ctx, rs, "e1")
		if got.Data["n"] != int64(2) || got.Origin == nil {
			t.Fatalf("after apply: %+v", got)
		}
		if _, err := w.ApplyRecordOnce(ctx, rs, "nope", env, "k2", inc); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing record: %v", err)
		}
		if marked, err := w.RunnerMarked(ctx, env, "k2"); err != nil || marked {
			t.Fatalf("mark of a missing record: %v %v", marked, err)
		}
		if err := w.MarkRunnerDone(ctx, env, "a/b"); err != nil {
			t.Fatal(err)
		}
		if marked, err := w.RunnerMarked(ctx, env, "a/b"); err != nil || !marked {
			t.Fatalf("marked: %v %v", marked, err)
		}
		// a user's edit keeps the origin
		if _, err := s.UpdateRecord(ctx, rs, "e1", map[string]any{"n": int64(5)}); err != nil {
			t.Fatal(err)
		}
		if got, _ = s.GetRecord(ctx, rs, "e1"); got.Origin == nil || got.Origin.EventID != "evt_1" {
			t.Fatalf("origin after edit: %+v", got)
		}
	})
}
