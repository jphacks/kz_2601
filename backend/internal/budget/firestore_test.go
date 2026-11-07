package budget

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
func TestFirestoreStore(t *testing.T) {
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
	id := "cost_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if got, err := s.Total(ctx, id); err != nil || got != 0 {
		t.Fatalf("missing day: %v %v", got, err)
	}
	// concurrent increments must not lose any
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Add(ctx, id, 0.25, time.Now().Add(48*time.Hour)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got, err := s.Total(ctx, id); err != nil || got != 2 {
		t.Fatalf("total = %v %v, want 2", got, err)
	}
}
