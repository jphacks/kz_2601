package budget

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryStoreAddTotal_S7Backend007(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMemory()
	day1, day2 := "test_2026-11-07_ai_cost", "test_2026-11-08_ai_cost"
	exp := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)

	if got, err := m.Total(ctx, day1); err != nil || got != 0 {
		t.Fatalf("empty day: %v %v, want 0 nil", got, err)
	}
	for _, usd := range []float64{0.5, 0.25, 1} {
		if err := m.Add(ctx, day1, usd, exp); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Add(ctx, day2, 2, exp); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Total(ctx, day1); got != 1.75 {
		t.Fatalf("day1 = %v, want 1.75 (same period sums)", got)
	}
	if got, _ := m.Total(ctx, day2); got != 2 {
		t.Fatalf("day2 = %v, want 2 (other period counted apart)", got)
	}
}

// Concurrent adds must not lose any (run with -race).
func TestMemoryStoreAddTotal_S7Backend007_Concurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMemory()
	const id, n = "test_2026-11-07_ai_cost", 64
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.Add(ctx, id, 0.25, time.Time{}); err != nil {
				t.Error(err)
			}
			_, _ = m.Total(ctx, id)
		}()
	}
	wg.Wait()
	if got, _ := m.Total(ctx, id); got != n*0.25 {
		t.Fatalf("total = %v, want %v", got, n*0.25)
	}
}
