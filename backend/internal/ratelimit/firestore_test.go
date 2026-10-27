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
	c, err := firestore.NewClient(ctx, "demo-kz2601")
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
}
