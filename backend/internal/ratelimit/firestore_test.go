package ratelimit

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

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
	sfx := strconv.FormatInt(time.Now().UnixNano(), 36)
	counters := []Counter{
		{ID: "u_" + sfx, Scope: ScopeUser, Limit: 10},
		{ID: "g_" + sfx, Scope: ScopeGlobal, Limit: 1000},
	}
	exp := time.Now().Add(48 * time.Hour)

	// 8 concurrent 1-unit charges + the transaction must not lose any.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Consume(ctx, counters, 1, exp); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	u, err := s.Consume(ctx, counters, 2, exp)
	if err != nil || u.Blocked != -1 || u.Counts[0] != 10 || u.Counts[1] != 10 {
		t.Fatalf("after 8+2: %+v %v", u, err)
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
	sfx := strconv.FormatInt(time.Now().UnixNano(), 36)
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
