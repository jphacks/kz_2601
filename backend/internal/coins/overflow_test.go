package coins

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

func overflowService(st Store, mode Mode, c *clock) *Service {
	s := newService(st, mode, c)
	s.Config.Allowance = map[string]int64{KindRuns: 10, KindWrites: 0}
	s.Config.Overflow = map[string]Block{KindRuns: {Size: 5, Coins: 3}}
	return s
}

func buy(s *Service, u string, blocks int64) (OverflowResult, error) {
	return s.BuyOverflow(context.Background(), OverflowRequest{UID: u, Anonymous: true, Kind: KindRuns, AppID: "app-1", Month: MonthKey(s.now()), Blocks: blocks})
}

func overflowRows(t *testing.T, st Store, u string) []Entry {
	t.Helper()
	es, _, err := st.Ledger(context.Background(), u, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []Entry
	for _, e := range es {
		if e.Kind == KindSpendOverflow {
			out = append(out, e)
		}
	}
	return out
}

func TestBuyOverflowOffAndDisabled(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		c := &clock{t: t0}
		u := uid(t, "u")
		fund(t, st, u, 20, t0)
		for _, m := range []Mode{ModeOff, ModeLog} {
			if _, err := buy(overflowService(st, m, c), u, 1); !errors.Is(err, ErrOff) {
				t.Fatalf("mode %s: %v", m, err)
			}
		}
		s := overflowService(st, ModeEnforce, c)
		if _, err := buy(s, u, 1); !errors.Is(err, ErrAllowanceExhausted) { // overflow is off by default
			t.Fatalf("default off: %v", err)
		}
		if _, err := s.SetOverflow(context.Background(), u, true, OverflowSettings{Enabled: true, MonthlyCap: 100}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.BuyOverflow(context.Background(), OverflowRequest{UID: u, Anonymous: true, Kind: KindWrites, Month: MonthKey(t0), Blocks: 1}); !errors.Is(err, ErrAllowanceExhausted) {
			t.Fatalf("kind without a block: %v", err)
		}
		if w := wallet(t, st, u, t0); w.Purchased != 20 || len(overflowRows(t, st, u)) != 0 {
			t.Fatalf("nothing is spent: %+v", w)
		}
	})
}

func TestBuyOverflowOnceThenCap(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := overflowService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 20, t0)
		if _, err := s.SetOverflow(ctx, u, true, OverflowSettings{Enabled: true, MonthlyCap: 7}); err != nil {
			t.Fatal(err)
		}
		r, err := buy(s, u, 1)
		if err != nil || r != (OverflowResult{Blocks: 1, Bought: 1, Spent: 3, Cap: 7}) {
			t.Fatalf("first block %+v %v", r, err)
		}
		// the same need again (another instance, a retry) buys nothing
		r, err = buy(s, u, 1)
		if err != nil || r != (OverflowResult{Blocks: 1, Spent: 3, Cap: 7}) {
			t.Fatalf("again %+v %v", r, err)
		}
		if r, err = buy(s, u, 2); err != nil || r.Bought != 1 || r.Spent != 6 {
			t.Fatalf("second block %+v %v", r, err)
		}
		var capErr *OverflowCapError
		if _, err := buy(s, u, 3); !errors.As(err, &capErr) || capErr.Cap != 7 || capErr.Spent != 6 {
			t.Fatalf("over the cap: %v", err)
		}
		w := wallet(t, st, u, t0)
		if w.Purchased != 14 || w.Overflow.Spent != 6 || w.Overflow.Blocks[KindRuns] != 2 || w.Overflow.Month != MonthKey(t0) {
			t.Fatalf("wallet %+v", w)
		}
		rows := overflowRows(t, st, u)
		if len(rows) != 2 || rows[0].Amount != -3 || rows[0].Op != KindRuns || rows[0].AppID != "app-1" {
			t.Fatalf("ledger %+v", rows)
		}
		v, err := s.Wallet(ctx, u, true)
		if err != nil || v.Overflow.Spent != 6 {
			t.Fatalf("view %+v %v", v.Overflow, err)
		}
		consistent(t, st, u, t0)
	})
}

func TestBuyOverflowInsufficient(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := overflowService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 2, t0)
		if _, err := s.SetOverflow(ctx, u, true, OverflowSettings{Enabled: true, MonthlyCap: 100}); err != nil {
			t.Fatal(err)
		}
		var short *InsufficientError
		if _, err := buy(s, u, 1); !errors.As(err, &short) || short.Required != 3 || short.Balance != 2 {
			t.Fatalf("short: %v", err)
		}
		if w := wallet(t, st, u, t0); w.Purchased != 2 || w.Overflow.Spent != 0 {
			t.Fatalf("wallet %+v", w)
		}
	})
}

// A new JST month starts with nothing bought or spent; a request for a
// month the wallet already left is refused.
func TestBuyOverflowMonthBoundary(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx := context.Background()
		end := time.Date(2026, 11, 30, 23, 59, 0, 0, jst)
		c := &clock{t: end}
		s := overflowService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 20, end)
		if _, err := s.SetOverflow(ctx, u, true, OverflowSettings{Enabled: true, MonthlyCap: 3}); err != nil {
			t.Fatal(err)
		}
		if r, err := buy(s, u, 1); err != nil || r.Bought != 1 {
			t.Fatalf("november %+v %v", r, err)
		}
		c.add(2 * time.Minute) // 2026-12-01 00:01 JST
		r, err := buy(s, u, 1)
		if err != nil || r.Bought != 1 || r.Spent != 3 {
			t.Fatalf("december: a new month buys again within the cap %+v %v", r, err)
		}
		old := OverflowRequest{UID: u, Anonymous: true, Kind: KindRuns, Month: "202611", Blocks: 2}
		if _, err := s.BuyOverflow(ctx, old); !errors.Is(err, ErrStaleMonth) {
			t.Fatalf("stale month: %v", err)
		}
		if w := wallet(t, st, u, c.now()); w.Overflow.Month != "202612" || w.Overflow.Blocks[KindRuns] != 1 || w.Purchased != 14 {
			t.Fatalf("wallet %+v", w)
		}
		consistent(t, st, u, c.now())
	})
}

// Two runner instances needing the same block buy it once.
func TestBuyOverflowConcurrent(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := overflowService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 30, t0)
		if _, err := s.SetOverflow(ctx, u, true, OverflowSettings{Enabled: true, MonthlyCap: 100}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		bought := int64(0)
		for range 5 {
			wg.Go(func() {
				r, err := buy(s, u, 1)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				bought += r.Bought
				mu.Unlock()
			})
		}
		wg.Wait()
		if bought != 1 || len(overflowRows(t, st, u)) != 1 {
			t.Fatalf("bought %d, rows %d", bought, len(overflowRows(t, st, u)))
		}
		if w := wallet(t, st, u, t0); w.Purchased != 27 {
			t.Fatalf("wallet %+v", w)
		}
	})
}

func TestWalletAllowance(t *testing.T) {
	ctx, c := context.Background(), &clock{t: t0}
	st := NewMemory()
	counters := ratelimit.NewMemoryClock(c.now)
	s := overflowService(st, ModeEnforce, c)
	s.Env = "test"
	u := "creator"
	fund(t, st, u, 20, t0)
	if v, err := s.Wallet(ctx, u, true); err != nil || v.Allowance != nil {
		t.Fatalf("no counters: %+v %v", v.Allowance, err)
	}
	s.Counters = counters
	ctr := []ratelimit.Counter{{ID: AllowanceCounterID("test", MonthKey(t0), KindRuns, u), Limit: 1000}}
	if _, err := counters.Consume(ctx, ctr, 12, t0.Add(40*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetOverflow(ctx, u, true, OverflowSettings{Enabled: true, MonthlyCap: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := buy(s, u, 1); err != nil {
		t.Fatal(err)
	}
	v, err := s.Wallet(ctx, u, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Allowance[KindRuns]; got != (AllowanceUse{Used: 12, Limit: 15, Allowance: 10, Blocks: 1}) {
		t.Fatalf("runs %+v", got)
	}
	if got := v.Allowance[KindWrites]; got != (AllowanceUse{}) {
		t.Fatalf("writes (counted, not enforced) %+v", got)
	}
	if len(v.Allowance) != len(AllowanceKinds) {
		t.Fatalf("kinds %+v", v.Allowance)
	}
	// mode off reads nothing
	s.Config.Mode = ModeOff
	if v, err := s.Wallet(ctx, u, true); err != nil || v.Allowance != nil {
		t.Fatalf("off %+v %v", v, err)
	}
}

func TestOverflowConfigFromEnv(t *testing.T) {
	env := map[string]string{"COIN_OVERFLOW_BLOCK_RUNS": "100", "COIN_OVERFLOW_PRICE_RUNS": "7", "COIN_OVERFLOW_BLOCK_READS": "50"}
	c, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.Overflow[KindRuns] != (Block{Size: 100, Coins: 7}) || c.Overflow[KindReads].On() || c.Overflow[KindWrites].On() {
		t.Fatalf("blocks %+v", c.Overflow)
	}
	p := c.Public()
	if len(p.Overflow.Blocks) != 1 || p.Overflow.BlockRuns != 100 || p.Overflow.BlockCoins != 7 {
		t.Fatalf("public %+v", p.Overflow)
	}
	env["COIN_OVERFLOW_PRICE_WRITES"] = "-1"
	if _, err := ConfigFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("negative price accepted")
	}
}
