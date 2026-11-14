package budget

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Memory is a per-process Store (tests, local runs).
type Memory struct {
	mu  sync.Mutex
	usd map[string]float64
}

// NewMemory returns an empty Memory.
func NewMemory() *Memory { return &Memory{usd: map[string]float64{}} }

// Add implements Store.
func (m *Memory) Add(_ context.Context, id string, usd float64, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usd[id] += usd
	return nil
}

// Total implements Store.
func (m *Memory) Total(_ context.Context, id string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usd[id], nil
}

// Firestore keeps the total in the rate limit collection:
//
//	rateLimits/{env}_{yyyy-mm-dd}_ai_cost  {usd, expireAt, updatedAt}
//
// expireAt is the same TTL field as the rate limit counters.
type Firestore struct {
	c   *firestore.Client
	col string
}

// NewFirestore uses collection (empty = ratelimit.DefaultCollection).
func NewFirestore(client *firestore.Client, collection string) *Firestore {
	if collection == "" {
		collection = ratelimit.DefaultCollection
	}
	return &Firestore{c: client, col: collection}
}

// Add implements Store with an atomic increment (no transaction: every
// Claude call writes, and adds commute).
func (f *Firestore) Add(ctx context.Context, id string, usd float64, expireAt time.Time) error {
	_, err := f.c.Collection(f.col).Doc(id).Set(ctx, map[string]any{
		"usd":       firestore.Increment(usd),
		"expireAt":  expireAt,
		"updatedAt": firestore.ServerTimestamp,
	}, firestore.MergeAll)
	return err
}

// Total implements Store (0 when the day has no document yet).
func (f *Firestore) Total(ctx context.Context, id string) (float64, error) {
	snap, err := f.c.Collection(f.col).Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	switch v := snap.Data()["usd"].(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	}
	return 0, nil
}
