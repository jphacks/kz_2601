package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"cloud.google.com/go/firestore"
)

// Sharded counter (the global cost breaker): every request of the environment
// charges it, and one Firestore document sustains about one write per second.
// The count is spread over Shards documents ({id}_{k}); the legacy single
// document {id} (written before sharding) is still read, never written.
//
// Fast path (far from the limit): the shards are read outside the
// transaction (no locks) and the transaction only adds the weight to one
// random shard with a blind increment, so requests no longer queue on one
// document. Strict path (the stale sum is within ShardStrictMargin of the
// limit): the transaction reads every shard, so the last units of the day are
// checked exactly, as before sharding.
//
// Worst case: requests that pass the fast path at the same time all see the
// same stale sum, which was at least ShardStrictMargin below the limit. The
// count can go over the limit only by what those concurrent requests add
// beyond the margin (max(0, in-flight fast-path weight - ShardStrictMargin)).

// ShardStrictMargin is how close (units) to its limit a sharded counter
// switches to the exact, all-shards transaction. 50 units ≈ 16 concurrent new
// apps admitted on one stale read before the cap can be passed.
const ShardStrictMargin = 50

var (
	errManySharded   = errors.New("ratelimit: at most one sharded counter per request")
	errRefundSharded = errors.New("ratelimit: refund a sharded counter through Usage.Charged")
)

// shardedIndex is the index of the counter with Shards > 1, or -1.
func shardedIndex(counters []Counter) (int, error) {
	g := -1
	for i, c := range counters {
		if c.Shards > 1 {
			if g >= 0 {
				return -1, errManySharded
			}
			g = i
		}
	}
	return g, nil
}

// shardID is the document of shard k of a counter.
func shardID(id string, k int) string {
	return fmt.Sprintf("%s_%d", id, k)
}

// nearLimit reports whether a charge of weight on a counter last seen at seen
// must be checked with the exact (strict) transaction.
func nearLimit(seen, weight, limit int) bool {
	return seen+weight > limit-ShardStrictMargin
}

// docsOf is every document holding c's count: the shards then the legacy one
// (just c.ID when c is not sharded).
func (f *Firestore) docsOf(c Counter) []*firestore.DocumentRef {
	col := f.c.Collection(f.col)
	if c.Shards <= 1 {
		return []*firestore.DocumentRef{col.Doc(c.ID)}
	}
	refs := make([]*firestore.DocumentRef, 0, c.Shards+1)
	for k := range c.Shards {
		refs = append(refs, col.Doc(shardID(c.ID, k)))
	}
	return append(refs, col.Doc(c.ID))
}

func sum(counts []int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}

// consumeSharded is Consume when counters[g] is sharded.
func (f *Firestore) consumeSharded(ctx context.Context, counters []Counter, g, weight int, expireAt time.Time) (Usage, error) {
	sc := counters[g]
	shardRefs := f.docsOf(sc)
	pre, err := f.c.GetAll(ctx, shardRefs)
	if err != nil {
		return Usage{}, err
	}
	seen := sum(readCounts(pre))
	strict := nearLimit(seen, weight, sc.Limit)
	shard := shardRefs[rand.IntN(sc.Shards)]

	plain := slices.Delete(slices.Clone(counters), g, g+1)
	plainRefs := f.refs(plain)
	var usage Usage
	err = f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		refs := plainRefs
		if strict {
			refs = append(slices.Clone(plainRefs), shardRefs...)
		}
		var snaps []*firestore.DocumentSnapshot
		if len(refs) > 0 {
			var err error
			if snaps, err = tx.GetAll(refs); err != nil {
				return err
			}
		}
		counts := readCounts(snaps)
		global := seen
		if strict {
			global = sum(counts[len(plainRefs):])
		}
		counts = slices.Insert(slices.Clone(counts[:len(plainRefs)]), g, global)
		usage = evaluate(counters, counts, weight)
		if usage.Blocked >= 0 {
			return nil
		}
		for i, ref := range plainRefs {
			j := i
			if i >= g {
				j++
			}
			if err := tx.Set(ref, map[string]any{
				"count":     int64(usage.Counts[j]),
				"scope":     plain[i].Scope,
				"expireAt":  expireAt,
				"updatedAt": firestore.ServerTimestamp,
			}); err != nil {
				return err
			}
		}
		// A blind increment: no read lock on the shard, so fast-path requests
		// on the same shard do not abort each other.
		return tx.Set(shard, map[string]any{
			"count":     firestore.Increment(weight),
			"scope":     sc.Scope,
			"expireAt":  expireAt,
			"updatedAt": firestore.ServerTimestamp,
		}, firestore.MergeAll)
	}, firestore.MaxAttempts(f.attempts))
	if err != nil || usage.Blocked >= 0 {
		return usage, err
	}
	usage.Charged = slices.Clone(counters)
	usage.Charged[g] = Counter{ID: shard.ID, Scope: sc.Scope, Limit: sc.Limit}
	return usage, nil
}
