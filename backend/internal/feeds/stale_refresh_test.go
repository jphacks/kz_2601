package feeds

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// A gap provider's expired entry (past the TTL, within staleFor) is served
// stale at once and starts exactly one refresh; the next caller reads it.
func TestGapStaleTriggersOneRefresh(t *testing.T) {
	src := newSource(t)
	c := &clock{t: testNow}
	s := src.svc(c, Options{})
	q := Query{Provider: dsl.FeedArxiv, FeedID: "cs.AI"}
	if _, err := s.Get(context.Background(), q, 5); err != nil {
		t.Fatal(err)
	}
	c.add(providerLimits[dsl.FeedArxiv].ttl + time.Minute)
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := s.Get(context.Background(), q, 5)
			if err != nil || len(res.Items) == 0 {
				t.Errorf("expired entry: %+v %v", res, err)
			}
		}()
	}
	wg.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := s.Get(context.Background(), q, 5)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Stale {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no refresh after the TTL")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(src.requests()); n != 2 {
		t.Errorf("requests = %d, want 2 (first fetch + one refresh)", n)
	}
}

// Search-word fetches are counted per client address too (new uids from one
// address share it), and the check and the count are one step: callers that
// wait in line at once cannot all pass the check first.
func TestDietMissesPerIPAndAtomic(t *testing.T) {
	src := newSource(t)
	c := &clock{t: testNow}
	s := src.svc(c, Options{UserMissesPerHour: 2, IPMissesPerHour: 3, DietPerMinute: 100})
	ctx := context.Background()
	for i, w := range []string{"予算", "決算", "税"} {
		if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: w, UID: "u" + w, IP: "ip1"}, 5); err != nil {
			t.Fatalf("word %d: %v", i, err)
		}
	}
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "年金", UID: "fresh", IP: "ip1"}, 5); !errors.Is(err, ErrUserLimit) {
		t.Errorf("4th word from the address: %v, want ErrUserLimit", err)
	}
	if _, err := s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: "年金", UID: "fresh", IP: "ip2"}, 5); err != nil {
		t.Errorf("another address: %v", err)
	}

	before := len(src.requests())
	var wg sync.WaitGroup
	for _, w := range []string{"a1", "a2", "a3", "a4", "a5"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Get(ctx, Query{Provider: dsl.FeedDiet, Q: w, UID: "same", IP: "ip3"}, 5)
		}()
	}
	wg.Wait()
	if n := len(src.requests()) - before; n != 2 {
		t.Errorf("concurrent words of one uid reached the source %d times, want 2", n)
	}
}
