package queue

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

type queueStore interface {
	store.Store
	store.OutboxQueue
}

func stores(t *testing.T, f func(t *testing.T, st queueStore, sfx string)) {
	t.Run("memory", func(t *testing.T) { f(t, store.NewMemory(), "") })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		fs, err := store.NewFirestore(t.Context(), "demo-kz2601", "(default)")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = fs.Close() })
		f(t, fs, "-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	})
}

func emit(env string) store.EmitFunc {
	return func(_ *store.Record, next store.Record) []store.OutboxEvent {
		return []store.OutboxEvent{{Env: env, Kind: store.OutboxRecordCreated, AppID: "Q", Scope: "shared", Entity: next.Entity, RecordID: next.ID, Version: 1, RuleIDs: []string{"r"}}}
	}
}

type outboxLister interface {
	OutboxEvents(ctx context.Context, env string, limit int) ([]store.OutboxEvent, error)
}

// #341: claim leases each due event once, attempts count up, a finished
// event leaves the queue but stays as a document with its outcome.
func TestQueueClaimFinish(t *testing.T) {
	stores(t, func(t *testing.T, st queueStore, sfx string) {
		ctx := t.Context()
		env := "q" + sfx
		for _, id := range []string{"a", "b"} {
			if _, _, err := st.CreateRecordEmit(ctx, store.PublicRecords("Q"+sfx, "e"), store.Record{ID: id, Data: map[string]any{}}, emit(env)); err != nil {
				t.Fatal(err)
			}
		}
		now := time.Now().Add(time.Second)
		q := New(st, env)
		q.Now = func() time.Time { return now }
		got, err := q.Claim(ctx, 10)
		if err != nil || len(got) != 2 || got[0].Attempts != 1 || got[1].Attempts != 1 {
			t.Fatalf("claim: %+v %v", got, err)
		}
		// leased: not claimable again until the lease ends
		if again, err := q.Claim(ctx, 10); err != nil || len(again) != 0 {
			t.Fatalf("claimed twice: %+v %v", again, err)
		}
		if err := q.Done(ctx, got[0], "ok", map[string]string{"r": "ok"}); err != nil {
			t.Fatal(err)
		}
		// a stale attempt cannot overwrite
		stale := got[1]
		stale.Attempts = 9
		if err := st.FinishOutbox(ctx, env, stale.EventID, stale.Attempts, store.OutboxResult{Status: store.OutboxDone}); err != store.ErrLeaseLost {
			t.Fatalf("stale finish: %v", err)
		}
		if err := q.Retry(ctx, got[1], "boom"); err != nil {
			t.Fatal(err)
		}
		// after the lease, only the retried one comes back (finished stays out)
		now = now.Add(q.Lease + time.Minute)
		back, err := q.Claim(ctx, 10)
		if err != nil || len(back) != 1 || back[0].EventID != got[1].EventID || back[0].Attempts != 2 || back[0].Status != store.OutboxRetry || back[0].LastError != "boom" {
			t.Fatalf("after retry: %+v %v", back, err)
		}
		all, err := st.(outboxLister).OutboxEvents(ctx, env, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range all {
			if ev.EventID == got[0].EventID && (ev.Status != store.OutboxDone || ev.Outcome != "ok" || ev.RuleOutcomes["r"] != "ok" || !ev.AvailableAt.IsZero() || ev.FinishedAt.IsZero()) {
				t.Fatalf("finished event: %+v", ev)
			}
		}
	})
}

// #341: the delay grows and the 4th claim is exhausted.
func TestQueueGiveUp(t *testing.T) {
	stores(t, func(t *testing.T, st queueStore, sfx string) {
		ctx := t.Context()
		env := "g" + sfx
		if _, _, err := st.CreateRecordEmit(ctx, store.PublicRecords("G"+sfx, "e"), store.Record{ID: "a", Data: map[string]any{}}, emit(env)); err != nil {
			t.Fatal(err)
		}
		now := time.Now().Add(time.Second)
		q := New(st, env)
		q.Now = func() time.Time { return now }
		if q.Delay(1) != 30*time.Second || q.Delay(2) != 2*time.Minute || q.Delay(3) != 8*time.Minute {
			t.Fatalf("delays: %v %v %v", q.Delay(1), q.Delay(2), q.Delay(3))
		}
		for i := 1; i <= 4; i++ {
			got, err := q.Claim(ctx, 10)
			if err != nil || len(got) != 1 || got[0].Attempts != i {
				t.Fatalf("claim %d: %+v %v", i, got, err)
			}
			if q.Exhausted(got[0]) != (i == 4) {
				t.Fatalf("exhausted at %d", i)
			}
			if i == 4 {
				if err := q.GiveUp(ctx, got[0], ""); err != nil {
					t.Fatal(err)
				}
				break
			}
			if err := q.Retry(ctx, got[0], "x"); err != nil {
				t.Fatal(err)
			}
			now = now.Add(q.Delay(i) + time.Second)
		}
		now = now.Add(time.Hour)
		if got, err := q.Claim(ctx, 10); err != nil || len(got) != 0 {
			t.Fatalf("gave up but claimed: %+v %v", got, err)
		}
		all, _ := st.(outboxLister).OutboxEvents(ctx, env, 0)
		if len(all) != 1 || all[0].Status != store.OutboxGaveUp || all[0].Outcome != OutcomeGaveUp || all[0].LastError != "x" {
			t.Fatalf("gave up event: %+v", all)
		}
	})
}
