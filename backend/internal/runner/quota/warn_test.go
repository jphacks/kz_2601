package quota

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// #345: the app's counter at 80% warns "near", stopping on it warns "full";
// other counters and retries warn nothing.
func TestDecisionWarning(t *testing.T) {
	q, _ := newQuota(nil, "test", Limits{AppExecPro: 5, CreatorExecPro: 100}, everyonePro{})
	var got []int
	for range 7 {
		got = append(got, begin(t, q, appA).Warning())
	}
	want := []int{0, 0, 0, WarnNear, WarnNear, WarnFull, WarnFull}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("levels %v, want %v", got, want)
		}
	}
	retry := appB
	retry.Attempt = 2
	if w := begin(t, q, retry).Warning(); w != 0 {
		t.Fatalf("retry: %d", w)
	}
	// the creator's counter near its limit is not the app's
	q2, _ := newQuota(nil, "test", Limits{AppExecPro: 100, CreatorExecPro: 1}, everyonePro{})
	if d := begin(t, q2, appA); !d.NearLimit || d.Warning() != 0 {
		t.Fatalf("creator counter: %+v", d)
	}
}

// #345: one warning per app, level and JST day, across instances.
func TestClaimWarningOncePerDay(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		c := &clock{t: time.Date(2026, 11, 6, 23, 0, 0, 0, ratelimit.JST)}
		if st == nil {
			st = ratelimit.NewMemoryClock(c.now)
		}
		cfg := Config{Limits: Limits{AppExecPro: 5}, Env: env, Now: c.now}
		a, b := New(cfg, st, everyonePro{}), New(cfg, st, everyonePro{}) // two runner instances
		ctx := context.Background()
		claim := func(q *Quota, r Run, level int) bool {
			t.Helper()
			ok, err := q.ClaimWarning(ctx, begin(t, q, r), level)
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		if !claim(a, appA, WarnNear) || claim(a, appA, WarnNear) || claim(b, appA, WarnNear) {
			t.Fatal("near must be claimed once")
		}
		if !claim(b, appA, WarnFull) || claim(a, appA, WarnFull) {
			t.Fatal("full must be claimed once, apart from near")
		}
		if !claim(a, appB, WarnNear) {
			t.Fatal("another app has its own warning")
		}
		c.t = c.t.Add(2 * time.Hour) // 01:00 JST, the next day
		if !claim(a, appA, WarnNear) || !claim(b, appA, WarnFull) {
			t.Fatal("the next day warns again")
		}
	})
}

// #345: concurrent instances claim one warning once.
func TestClaimWarningConcurrent(t *testing.T) {
	q, _ := newQuota(nil, "test", Limits{AppExecPro: 100}, everyonePro{})
	dec := begin(t, q, appA)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			// a fresh instance each time: only the store dedupes
			q2 := New(q.cfg, q.store, everyonePro{})
			if ok, err := q2.ClaimWarning(context.Background(), dec, WarnNear); err == nil && ok {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if n := wins.Load(); n != 1 {
		t.Fatalf("claimed %d times", n)
	}
}
