package ratelimit

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
)

// DefaultCollection holds the counters (same database as appconfig).
//
//	rateLimits/{env}_{yyyy-mm-dd}_uid_{uid}    {count, scope, expireAt, updatedAt}
//	rateLimits/{env}_{yyyy-mm-dd}_ip_{sha256/8} (anonymous callers only)
//	rateLimits/{env}_{yyyy-mm-dd}_global
//
// expireAt is the Firestore TTL field (enable the TTL policy once:
// docs/backend/rate-limit.md); documents of past days then disappear on their own.
const DefaultCollection = "rateLimits"

// Firestore is the production Store: one transaction per request reads every
// counter and either adds the weight to all of them or writes nothing.
//
// Contention: the client retries an Aborted transaction itself (exponential
// backoff from 1s). txAttempts keeps the worst case of the request path near
// 15s; when it is exhausted Allow falls back to the in-memory store.
type Firestore struct {
	c        *firestore.Client
	col      string
	attempts int
}

// txAttempts is the transaction attempt budget (client default).
const txAttempts = firestore.DefaultTransactionMaxAttempts

// NewFirestore uses client (honours FIRESTORE_EMULATOR_HOST) and collection
// (empty = DefaultCollection).
func NewFirestore(client *firestore.Client, collection string) *Firestore {
	if collection == "" {
		collection = DefaultCollection
	}
	return &Firestore{c: client, col: collection, attempts: txAttempts}
}

// Consume implements Store.
func (f *Firestore) Consume(ctx context.Context, counters []Counter, weight int, expireAt time.Time) (Usage, error) {
	refs := f.refs(counters)
	var usage Usage
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snaps, err := tx.GetAll(refs)
		if err != nil {
			return err
		}
		counts := readCounts(snaps)
		usage = evaluate(counters, counts, weight)
		if usage.Blocked >= 0 {
			return nil
		}
		for i, ref := range refs {
			if err := tx.Set(ref, map[string]any{
				"count":     int64(usage.Counts[i]),
				"scope":     counters[i].Scope,
				"expireAt":  expireAt,
				"updatedAt": firestore.ServerTimestamp,
			}); err != nil {
				return err
			}
		}
		return nil
	}, firestore.MaxAttempts(f.attempts))
	return usage, err
}

// Peek implements Peeker: one batched read, no transaction and no write
// (a missing document counts as 0).
func (f *Firestore) Peek(ctx context.Context, counters []Counter) ([]int, error) {
	snaps, err := f.c.GetAll(ctx, f.refs(counters))
	if err != nil {
		return nil, err
	}
	return readCounts(snaps), nil
}

// Refund implements Store: one transaction, like Consume. A counter document
// that does not exist (TTL already removed it) is not created again.
func (f *Firestore) Refund(ctx context.Context, counters []Counter, weight int, expireAt time.Time) error {
	refs := f.refs(counters)
	return f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snaps, err := tx.GetAll(refs)
		if err != nil {
			return err
		}
		counts := readCounts(snaps)
		for i, ref := range refs {
			if !snaps[i].Exists() {
				continue
			}
			if err := tx.Set(ref, map[string]any{
				"count":     int64(refunded(counts[i], weight)),
				"scope":     counters[i].Scope,
				"expireAt":  expireAt,
				"updatedAt": firestore.ServerTimestamp,
			}); err != nil {
				return err
			}
		}
		return nil
	}, firestore.MaxAttempts(f.attempts))
}

func (f *Firestore) refs(counters []Counter) []*firestore.DocumentRef {
	refs := make([]*firestore.DocumentRef, len(counters))
	for i, c := range counters {
		refs[i] = f.c.Collection(f.col).Doc(c.ID)
	}
	return refs
}

// readCounts is the `count` of each snapshot (0 when missing).
func readCounts(snaps []*firestore.DocumentSnapshot) []int {
	counts := make([]int, len(snaps))
	for i, s := range snaps {
		if !s.Exists() {
			continue
		}
		if v, err := s.DataAt("count"); err == nil {
			if n, ok := v.(int64); ok {
				counts[i] = int(n)
			}
		}
	}
	return counts
}
