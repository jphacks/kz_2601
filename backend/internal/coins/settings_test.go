package coins

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSetOverflowStoresSetting(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 20, t0)

		v, err := s.Wallet(ctx, u, true)
		if err != nil || v.Overflow != (OverflowView{}) {
			t.Fatalf("default overflow %+v %v (want off)", v.Overflow, err)
		}
		v, err = s.SetOverflow(ctx, u, true, OverflowSettings{Enabled: true, MonthlyCap: 30})
		if err != nil || v.Overflow != (OverflowView{Enabled: true, MonthlyCap: 30}) || v.Purchased != 20 {
			t.Fatalf("set %+v %v", v, err)
		}
		if w := wallet(t, st, u, t0); !w.Overflow.Enabled || w.Overflow.MonthlyCap != 30 || w.Purchased != 20 {
			t.Fatalf("stored %+v", w)
		}
		if _, err := s.SetOverflow(ctx, u, true, OverflowSettings{MonthlyCap: -1}); !errors.Is(err, ErrBadCap) {
			t.Fatalf("negative cap %v", err)
		}
		if _, err := s.SetOverflow(ctx, u, true, OverflowSettings{MonthlyCap: MaxOverflowCap + 1}); !errors.Is(err, ErrBadCap) {
			t.Fatalf("huge cap %v", err)
		}
		consistent(t, st, u, t0)
	})
}

func TestOverflowSpentIsThisMonthOnly(t *testing.T) {
	o := Overflow{Enabled: true, MonthlyCap: 10, Month: overflowMonth(t0), Spent: 4}
	if v := o.view(t0); v.Spent != 4 {
		t.Fatalf("this month %+v", v)
	}
	// 2026-11-30 15:00 UTC is 2026-12-01 in JST
	next := t0.AddDate(0, 0, 20).Add(12 * time.Hour)
	if v := o.view(next); v.Spent != 0 || v.MonthlyCap != 10 || !v.Enabled {
		t.Fatalf("next month %+v", v)
	}
}

func TestSetOverflowModeOff(t *testing.T) {
	s := newService(failStore{t}, ModeOff, &clock{t: t0})
	if _, err := s.SetOverflow(context.Background(), "u", false, OverflowSettings{Enabled: true}); !errors.Is(err, ErrOff) {
		t.Fatalf("off %v", err)
	}
}
