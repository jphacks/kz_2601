package ratelimit

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
)

// DefaultCollection holds the counters (same database as appconfig).
//
//	rateLimits/{env}_{yyyy-mm-dd}_uid_{uid}    {count, scope, expireAt, updatedAt}
//	rateLimits/{env}_{yyyy-mm-dd}_ip_{sha256/8} (anonymous callers only)
//	rateLimits/{env}_{yyyy-mm-dd}_global_{0..GlobalShards-1}  (shards, firestore_shard.go)
//	rateLimits/{env}_{yyyy-mm-dd}_global  (legacy single document: read, no longer written)
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

// Consume implements Store. A counter with Shards > 1 is charged to one of
// its shards (consumeSharded); Usage.Charged then names that shard.
func (f *Firestore) Consume(ctx context.Context, counters []Counter, weight int, expireAt time.Time) (Usage, error) {
	if g, err := shardedIndex(counters); err != nil || g >= 0 {
		if err != nil {
			return Usage{}, err
		}
		return f.consumeSharded(ctx, counters, g, weight, expireAt)
	}
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
// (a missing document counts as 0; a sharded counter is the sum of its shards
// and its legacy document).
func (f *Firestore) Peek(ctx context.Context, counters []Counter) ([]int, error) {
	var refs []*firestore.DocumentRef
	for _, c := range counters {
		refs = append(refs, f.docsOf(c)...)
	}
	snaps, err := f.c.GetAll(ctx, refs)
	if err != nil {
		return nil, err
	}
	counts := readCounts(snaps)
	out := make([]int, len(counters))
	for i, c := range counters {
		n := len(f.docsOf(c))
		out[i] = sum(counts[:n])
		counts = counts[n:]
	}
	return out, nil
}

// Refund implements Store: one transaction, like Consume. A counter document
// that does not exist (TTL already removed it) is not created again.
// A sharded counter is refunded through Usage.Charged (the shard it was
// charged to); passing the sharded counter itself is an error.
func (f *Firestore) Refund(ctx context.Context, counters []Counter, weight int, expireAt time.Time) error {
	if g, err := shardedIndex(counters); err != nil || g >= 0 {
		return errors.Join(err, errRefundSharded)
	}
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
