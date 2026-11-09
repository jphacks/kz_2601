package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

// testSuffix keeps document ids unique across test processes sharing one
// emulator (a timestamp alone collides when two start at the same instant).
func testSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

// Runs against the local emulator only (make emulator); never staging/prod.
func TestFirestoreConsume(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	c, err := firestore.NewClientWithDatabase(ctx, "demo-kz2601", firestore.DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	s := NewFirestore(c, "rateLimitsTest")
	// The emulator locks pessimistically: concurrent transactions on one
	// document deadlock until its ~2s lock timeout aborts them, so each
	// collision burns a whole attempt.
	// Fewer writers and a larger budget keep the test about lost updates,
	// not about the emulator's timeout under a slow CI runner (#721).
	s.attempts = 10
	sfx := testSuffix(t)
	counters := []Counter{
		{ID: "u_" + sfx, Scope: ScopeUser, Limit: 10},
		{ID: "g_" + sfx, Scope: ScopeGlobal, Limit: 1000},
	}
	exp := time.Now().Add(48 * time.Hour)

	// 4 concurrent 1-unit charges + the transaction must not lose any.
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Consume(ctx, counters, 1, exp); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	u, err := s.Consume(ctx, counters, 6, exp)
	if err != nil || u.Blocked != -1 || u.Counts[0] != 10 || u.Counts[1] != 10 {
		t.Fatalf("after 4+6: %+v %v", u, err)
	}
	u, err = s.Consume(ctx, counters, 1, exp)
	if err != nil || u.Blocked != 0 || u.Counts[0] != 10 {
		t.Fatalf("over limit: %+v %v", u, err)
	}
	// Peek (undo, weight 0) reads without writing; a missing document is 0.
	got, err := s.Peek(ctx, append(counters, Counter{ID: "none_" + sfx, Scope: ScopeUser, Limit: 10}))
	if err != nil || len(got) != 3 || got[0] != 10 || got[1] != 10 || got[2] != 0 {
		t.Fatalf("peek: %v %v", got, err)
	}
	if snap, err := c.Collection("rateLimitsTest").Doc("none_" + sfx).Get(ctx); err == nil || snap.Exists() {
		t.Fatal("peek must not create a document")
	}
}

// Runs against the local emulator only (make emulator); never staging/prod.
func TestFirestoreRefund(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	c, err := firestore.NewClientWithDatabase(ctx, "demo-kz2601", firestore.DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	s := NewFirestore(c, "rateLimitsTest")
	sfx := testSuffix(t)
	counters := []Counter{
		{ID: "u_" + sfx, Scope: ScopeUser, Limit: 10},
		{ID: "g_" + sfx, Scope: ScopeGlobal, Limit: 1000},
	}
	exp := time.Now().Add(48 * time.Hour)
	if _, err := s.Consume(ctx, []Counter{counters[1]}, 4, exp); err != nil { // someone else's global
		t.Fatal(err)
	}
	if _, err := s.Consume(ctx, counters, 3, exp); err != nil {
		t.Fatal(err)
	}
	later := exp.Add(time.Hour).Truncate(time.Microsecond) // Firestore keeps microseconds
	if err := s.Refund(ctx, counters, 2, later); err != nil {
		t.Fatal(err)
	}
	read := func(id string) (int64, time.Time, bool) {
		snap, err := c.Collection("rateLimitsTest").Doc(id).Get(ctx)
		if err != nil {
			return 0, time.Time{}, false
		}
		n, _ := snap.DataAt("count")
		e, _ := snap.DataAt("expireAt")
		at, _ := e.(time.Time)
		return n.(int64), at, true
	}
	if n, at, _ := read(counters[0].ID); n != 1 || !at.Equal(later) {
		t.Fatalf("user after refund: %d %v", n, at)
	}
	if n, _, _ := read(counters[1].ID); n != 5 { // 4 + 3 - 2
		t.Fatalf("global after refund: %d", n)
	}
	// never below zero
	if err := s.Refund(ctx, counters, 9, later); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := read(counters[0].ID); n != 0 {
		t.Fatalf("user below zero: %d", n)
	}
	// a missing counter is not created
	missing := Counter{ID: "m_" + sfx, Scope: ScopeUser, Limit: 10}
	if err := s.Refund(ctx, []Counter{missing}, 1, later); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := read(missing.ID); ok {
		t.Fatal("refund created a counter")
	}
}

// newEmulatorStore is a Firestore store on the local emulator (skips without it).
func newEmulatorStore(t *testing.T) (*Firestore, *firestore.Client) {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	c, err := firestore.NewClientWithDatabase(context.Background(), "demo-kz2601", firestore.DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	s := NewFirestore(c, "rateLimitsTest")
	s.attempts = 10 // see TestFirestoreConsume (#721)
	return s, c
}

// shardTotal is the stored sum of a sharded counter (shards + legacy).
func shardTotal(t *testing.T, s *Firestore, g Counter) int {
	t.Helper()
	got, err := s.Peek(context.Background(), []Counter{g})
	if err != nil {
		t.Fatal(err)
	}
	return got[0]
}

// Runs against the local emulator only. Fast path: concurrent charges land on
// shards without losing any, the legacy document still counts (and is not
// written), and Refund takes a charge back from the shard it went to.
func TestFirestoreGlobalShards(t *testing.T) {
	s, c := newEmulatorStore(t)
	ctx := context.Background()
	sfx := testSuffix(t)
	g := Counter{ID: "gs_" + sfx, Scope: ScopeGlobal, Limit: 1000, Shards: GlobalShards}
	exp := time.Now().Add(48 * time.Hour)
	legacy := c.Collection("rateLimitsTest").Doc(g.ID)
	if _, err := legacy.Set(ctx, map[string]any{"count": int64(5), "scope": ScopeGlobal, "expireAt": exp}); err != nil {
		t.Fatal(err)
	}

	// 4 callers × 3 charges of 2, each caller with its own user counter.
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 3 {
				u := Counter{ID: "u_" + testSuffix(t), Scope: ScopeUser, Limit: 10}
				usage, err := s.Consume(ctx, []Counter{u, g}, 2, exp)
				if err != nil || usage.Blocked != -1 {
					t.Errorf("consume: %+v %v", usage, err)
					return
				}
				if len(usage.Charged) != 2 || usage.Charged[0] != u || usage.Charged[1].Shards != 0 {
					t.Errorf("charged: %+v", usage.Charged)
				}
			}
		})
	}
	wg.Wait()
	if got := shardTotal(t, s, g); got != 5+24 {
		t.Fatalf("total = %d, want 29", got)
	}
	if snap, err := legacy.Get(ctx); err != nil || snap.Data()["count"] != int64(5) {
		t.Fatalf("legacy document was written: %v %v", snap.Data(), err)
	}

	usage, err := s.Consume(ctx, []Counter{g}, 3, exp)
	if err != nil || usage.Blocked != -1 || usage.Counts[0] != 32 {
		t.Fatalf("consume: %+v %v", usage, err)
	}
	if err := s.Refund(ctx, []Counter{g}, 3, exp); !errors.Is(err, errRefundSharded) {
		t.Fatalf("refund of the sharded counter itself: %v", err)
	}
	if err := s.Refund(ctx, usage.Charged, 3, exp); err != nil {
		t.Fatal(err)
	}
	if got := shardTotal(t, s, g); got != 29 {
		t.Fatalf("after refund = %d, want 29", got)
	}
}

// Runs against the local emulator only. Near the limit every charge reads all
// shards in its transaction: concurrent callers never pass the limit.
func TestFirestoreGlobalShardsStrict(t *testing.T) {
	s, _ := newEmulatorStore(t)
	ctx := context.Background()
	// A limit within ShardStrictMargin: every charge takes the strict path.
	g := Counter{ID: "gs_" + testSuffix(t), Scope: ScopeGlobal, Limit: 8, Shards: GlobalShards}
	exp := time.Now().Add(48 * time.Hour)

	var mu sync.Mutex
	allowed, blocked := 0, 0
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 3 {
				usage, err := s.Consume(ctx, []Counter{g}, 1, exp)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if usage.Blocked == -1 {
					allowed++
				} else {
					blocked++
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if allowed != 8 || blocked != 4 {
		t.Fatalf("allowed %d blocked %d, want 8 / 4", allowed, blocked)
	}
	if got := shardTotal(t, s, g); got != 8 {
		t.Fatalf("total = %d, want 8", got)
	}
}
