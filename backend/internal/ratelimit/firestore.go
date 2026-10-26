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
type Firestore struct {
	c   *firestore.Client
	col string
}

// NewFirestore uses client (honours FIRESTORE_EMULATOR_HOST) and collection
// (empty = DefaultCollection).
func NewFirestore(client *firestore.Client, collection string) *Firestore {
	if collection == "" {
		collection = DefaultCollection
	}
	return &Firestore{c: client, col: collection}
}

// Consume implements Store.
func (f *Firestore) Consume(ctx context.Context, counters []Counter, weight int, expireAt time.Time) (Usage, error) {
	refs := make([]*firestore.DocumentRef, len(counters))
	for i, c := range counters {
		refs[i] = f.c.Collection(f.col).Doc(c.ID)
	}
	var usage Usage
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snaps, err := tx.GetAll(refs)
		if err != nil {
			return err
		}
		counts := make([]int, len(counters))
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
	}, firestore.MaxAttempts(5))
	return usage, err
}
