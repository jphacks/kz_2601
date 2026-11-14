package coins

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func slotService(st Store, mode Mode, c *clock, price int64) *Service {
	s := newService(st, mode, c)
	s.Config.SlotPrice = price
	return s
}

func slotRows(t *testing.T, st Store, u string) []Entry {
	t.Helper()
	es, _, err := st.Ledger(context.Background(), u, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []Entry
	for _, e := range es {
		if e.Kind == KindSpendSlot {
			out = append(out, e)
		}
	}
	return out
}

func TestConfigSlotPrice(t *testing.T) {
	c, err := ConfigFromEnv(env(map[string]string{"COIN_BILLING_MODE": "enforce", "COIN_PRICE_APP_SLOT": "40"}))
	if err != nil || c.SlotPrice != 40 || c.Public().AppSlotCoins != 40 {
		t.Fatalf("%+v %v", c, err)
	}
	if c, _ := ConfigFromEnv(env(map[string]string{"COIN_BILLING_MODE": "log", "COIN_PRICE_APP_SLOT": "40"})); c.Public().AppSlotCoins != 0 {
		t.Fatal("log mode does not show a slot price")
	}
	if _, err := ConfigFromEnv(env(map[string]string{"COIN_PRICE_APP_SLOT": "-1"})); err == nil {
		t.Fatal("negative price must fail")
	}
}

// One slot per request id: a retry spends nothing; the ledger and the wallet
// agree; short coins buy nothing.
func TestBuySlot(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx := context.Background()
		c := &clock{t: t0}
		u := uid(t, "u")
		fund(t, st, u, 25, t0)
		s := slotService(st, ModeEnforce, c, 10)

		p, err := s.BuySlot(ctx, u, true, "req-1")
		if err != nil || !p.Bought || p.AppSlots != 1 || p.Coins != 10 || p.Wallet.Available != 15 || p.Wallet.AppSlots != 1 {
			t.Fatalf("first: %+v %v", p, err)
		}
		again, err := s.BuySlot(ctx, u, true, "req-1")
		if err != nil || again.Bought || again.AppSlots != 1 || again.Wallet.Available != 15 {
			t.Fatalf("retry: %+v %v", again, err)
		}
		if p, err = s.BuySlot(ctx, u, true, "req-2"); err != nil || !p.Bought || p.AppSlots != 2 || p.Wallet.Available != 5 {
			t.Fatalf("second: %+v %v", p, err)
		}
		var ins *InsufficientError
		if _, err := s.BuySlot(ctx, u, true, "req-3"); !errors.As(err, &ins) || ins.Required != 10 || ins.Balance != 5 {
			t.Fatalf("short: %v", err)
		}
		if rows := slotRows(t, st, u); len(rows) != 2 || rows[0].Amount != -10 || rows[0].Op != OpAppSlot {
			t.Fatalf("ledger %+v", rows)
		}
		if w := wallet(t, st, u, t0); w.AppSlots != 2 || w.Purchased != 5 {
			t.Fatalf("wallet %+v", w)
		}
		consistent(t, st, u, t0)
		info, err := s.AppSlots(ctx, u)
		if err != nil || info.Purchased != 2 || info.Price != 10 || info.Available != 5 {
			t.Fatalf("AppSlots %+v %v", info, err)
		}
	})
}

// The same request id from two instances at once buys one slot.
func TestBuySlotConcurrentRetry(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx := context.Background()
		c := &clock{t: t0}
		u := uid(t, "u")
		fund(t, st, u, 100, t0)
		s := slotService(st, ModeEnforce, c, 10)
		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.BuySlot(ctx, u, true, "same"); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if w := wallet(t, st, u, t0); w.AppSlots != 1 || w.Purchased != 90 || len(slotRows(t, st, u)) != 1 {
			t.Fatalf("wallet %+v", w)
		}
	})
}

// Only enforce with a price buys; log still reads the slots bought before;
// off reads nothing.
func TestBuySlotModes(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx := context.Background()
		c := &clock{t: t0}
		u := uid(t, "u")
		fund(t, st, u, 30, t0)
		if _, err := slotService(st, ModeEnforce, c, 0).BuySlot(ctx, u, true, "r"); !errors.Is(err, ErrSlotsOff) {
			t.Fatalf("price 0: %v", err)
		}
		for _, m := range []Mode{ModeOff, ModeLog} {
			if _, err := slotService(st, m, c, 10).BuySlot(ctx, u, true, "r"); !errors.Is(err, ErrSlotsOff) {
				t.Fatalf("%s: %v", m, err)
			}
		}
		if _, err := slotService(st, ModeEnforce, c, 10).BuySlot(ctx, u, true, "r"); err != nil {
			t.Fatal(err)
		}
		if info, err := slotService(st, ModeLog, c, 10).AppSlots(ctx, u); err != nil || info.Purchased != 1 || info.Price != 0 {
			t.Fatalf("log: %+v %v", info, err)
		}
		if info, err := slotService(st, ModeOff, c, 10).AppSlots(ctx, u); err != nil || info != (SlotInfo{}) {
			t.Fatalf("off: %+v %v", info, err)
		}
	})
}
