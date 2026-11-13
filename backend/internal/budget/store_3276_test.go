package budget

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryStoreAddTotal_3276(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	exp := time.Now().Add(time.Hour)

	if got, err := m.Total(ctx, "dev_2026-11-13_ai_cost"); err != nil || got != 0 {
		t.Fatalf("empty total = %v, %v", got, err)
	}

	// the same period adds up
	for _, usd := range []float64{0.25, 0.5, 1} {
		if err := m.Add(ctx, "dev_2026-11-13_ai_cost", usd, exp); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := m.Total(ctx, "dev_2026-11-13_ai_cost"); got != 1.75 {
		t.Fatalf("total = %v, want 1.75", got)
	}

	// another period (day / env) is counted apart
	if err := m.Add(ctx, "dev_2026-11-14_ai_cost", 2, exp); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(ctx, "prod_2026-11-13_ai_cost", 4, exp); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]float64{
		"dev_2026-11-13_ai_cost":  1.75,
		"dev_2026-11-14_ai_cost":  2,
		"prod_2026-11-13_ai_cost": 4,
		"dev_2026-11-15_ai_cost":  0,
	} {
		if got, err := m.Total(ctx, id); err != nil || got != want {
			t.Errorf("Total(%s) = %v, %v; want %v", id, got, err, want)
		}
	}
}

func TestMemoryStoreAddZeroAndNegative_3276(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	_ = m.Add(ctx, "id", 0, time.Time{})
	if got, _ := m.Total(ctx, "id"); got != 0 {
		t.Fatalf("total = %v", got)
	}
	_ = m.Add(ctx, "id", 1.5, time.Time{})
	_ = m.Add(ctx, "id", -0.5, time.Time{}) // a correction subtracts
	if got, _ := m.Total(ctx, "id"); got != 1 {
		t.Fatalf("total = %v, want 1", got)
	}
}

func TestMemoryStoreConcurrentAdd_3276(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	const workers, each = 16, 250
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				_ = m.Add(ctx, "shared", 0.25, time.Time{})
				_ = m.Add(ctx, "own"+string(rune('a'+w)), 0.5, time.Time{})
				_, _ = m.Total(ctx, "shared")
			}
		}(w)
	}
	wg.Wait()
	if got, _ := m.Total(ctx, "shared"); got != workers*each*0.25 {
		t.Fatalf("shared = %v, want %v", got, float64(workers*each)*0.25)
	}
	for w := 0; w < workers; w++ {
		if got, _ := m.Total(ctx, "own"+string(rune('a'+w))); got != each*0.5 {
			t.Fatalf("own%d = %v", w, got)
		}
	}
}
