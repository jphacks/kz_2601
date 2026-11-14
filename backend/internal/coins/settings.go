package coins

import (
	"context"
	"errors"
	"time"
)

// MaxOverflowCap bounds Overflow.MonthlyCap (a typo guard, not a price).
const MaxOverflowCap = 1_000_000_000

// ErrBadCap: a monthly cap below 0 or above MaxOverflowCap.
var ErrBadCap = errors.New("monthlyCap must be 0-1000000000")

// Overflow is wallet.overflow (design: "サーバーの枠と、超えた後のコイン払い").
// Enabled is off by default; MonthlyCap is the owner's monthly coin limit for
// overflow; Spent counts the coins spent on overflow in Month (JST yyyymm)
// and Blocks the blocks bought per allowance kind in Month (BuyOverflow).
type Overflow struct {
	Enabled    bool             `firestore:"enabled"`
	MonthlyCap int64            `firestore:"monthlyCap"`
	Month      string           `firestore:"month,omitempty"`
	Spent      int64            `firestore:"spent"`
	Blocks     map[string]int64 `firestore:"blocks,omitempty"`
}

// inMonth returns o as of month: a new month starts with nothing spent or bought.
func (o Overflow) inMonth(month string) Overflow {
	if o.Month != month {
		o.Month, o.Spent, o.Blocks = month, 0, nil
	}
	return o
}

// OverflowView is View.overflow.
type OverflowView struct {
	Enabled    bool  `json:"enabled"`
	MonthlyCap int64 `json:"monthlyCap"`
	// Spent is this JST month's overflow spend (0 after the month changed).
	Spent int64 `json:"spent"`
}

var jst = time.FixedZone("JST", 9*60*60)

// MonthKey is the JST month (yyyymm) of now: the allowance month of the
// design, shared by the runner's monthly counters and wallet.overflow.
func MonthKey(now time.Time) string { return now.In(jst).Format("200601") }

func overflowMonth(now time.Time) string { return MonthKey(now) }

func (o Overflow) view(now time.Time) OverflowView {
	v := OverflowView{Enabled: o.Enabled, MonthlyCap: o.MonthlyCap}
	if o.Month == overflowMonth(now) {
		v.Spent = o.Spent
	}
	return v
}

// OverflowSettings is the body of PUT /v1/me/coins/settings.
type OverflowSettings struct {
	Enabled    bool  `json:"enabled"`
	MonthlyCap int64 `json:"monthlyCap"`
}

// SetOverflow stores uid's overflow opt-in and monthly cap (the month's
// spend is kept) and returns the wallet. Errors: ErrOff, ErrBadCap.
func (s *Service) SetOverflow(ctx context.Context, uid string, anonymous bool, o OverflowSettings) (View, error) {
	if s.Mode() == ModeOff {
		return View{}, ErrOff
	}
	if o.MonthlyCap < 0 || o.MonthlyCap > MaxOverflowCap {
		return View{}, ErrBadCap
	}
	g, err := s.policy(ctx, uid, anonymous)
	if err != nil {
		return View{}, err
	}
	now := storedTime(s.now())
	var out Wallet
	err = s.Store.Update(ctx, Keys{UID: uid}, func(tx *Tx) error {
		es, err := applyGrants(tx, uid, g, now)
		if err != nil {
			return err
		}
		tx.Entries = stamp(es, now)
		tx.Wallet.Overflow.Enabled = o.Enabled
		tx.Wallet.Overflow.MonthlyCap = o.MonthlyCap
		tx.Wallet.UpdatedAt = now
		tx.WalletDirty = true
		out = tx.Wallet
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return s.view(out), nil
}
