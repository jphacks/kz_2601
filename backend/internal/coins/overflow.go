package coins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Part 3 of #630: Pro's monthly server allowance and overflow paid with
// coins. The runner counts each creator's runs / recipients / writes / reads
// per JST month in ratelimit counters (AllowanceCounterID, next to its daily
// counters); past ALLOWANCE_PRO_<KIND> it buys a block of
// COIN_OVERFLOW_BLOCK_<KIND> units for COIN_OVERFLOW_PRICE_<KIND> coins with
// BuyOverflow, when the creator turned overflow on and the month's spend
// stays within their cap. The blocks bought are kept in wallet.overflow
// (same transaction as the coins and the ledger rows), so two runner
// instances never buy the same block twice.

// Log events of part 3.
const (
	EventOverflowBought = "coins.overflow_bought" // a block was bought
	EventWouldOverflow  = "coins.would_overflow"  // log mode: a block would have been bought
)

// ErrStaleMonth: BuyOverflow for a month older than the wallet's (a run that
// straddled the month boundary). Retry with the current month.
var ErrStaleMonth = errors.New("overflow month is over")

// AllowanceCounterID is the ratelimit counter of uid's use of kind in month
// (MonthKey). The runner writes it; GET /v1/me/coins reads it. env is APP_ENV
// ("" = local), like the runner's daily counters.
func AllowanceCounterID(env, month, kind, uid string) string {
	if env == "" {
		env = "local"
	}
	return env + "_" + month + "_ma_" + kind + "_" + strings.ReplaceAll(uid, "/", "_")
}

// AllowanceScope is the counter scope (and the runner's `limit` word) of kind.
func AllowanceScope(kind string) string { return "allowance_" + kind }

// OverflowRequest asks for blocks of Kind in Month.
type OverflowRequest struct {
	// UID pays: the creator of the app (shared apps included).
	UID       string
	Anonymous bool
	Kind      string
	// AppID is the app whose run needed the block (ledger rows only).
	AppID string
	Month string
	// Blocks is the number of blocks uid needs this month in all (not the
	// number to add): a second request for the same number buys nothing.
	Blocks int64
}

// OverflowResult is what uid owns after BuyOverflow.
type OverflowResult struct {
	// Blocks of the kind owned this month (≥ the request on success).
	Blocks int64
	// Bought by this call (0 = another instance had bought them).
	Bought int64
	// Spent is the month's overflow spend and Cap the owner's monthly cap.
	Spent int64
	Cap   int64
}

// BuyOverflow buys the missing blocks of r.Kind for r.Month in one wallet
// transaction: the coins (grant → initial → purchased), one spend_overflow
// ledger row per block (id overflow_{month}_{kind}_{n}) and
// wallet.overflow.blocks / spent. Errors: ErrOff (mode is not enforce),
// ErrAllowanceExhausted (overflow is off, or the kind has no block),
// *OverflowCapError, *InsufficientError, ErrStaleMonth.
func (s *Service) BuyOverflow(ctx context.Context, r OverflowRequest) (OverflowResult, error) {
	if s.Mode() != ModeEnforce {
		return OverflowResult{}, ErrOff
	}
	b := s.Config.Overflow[r.Kind]
	if !b.On() {
		return OverflowResult{}, ErrAllowanceExhausted
	}
	g, err := s.policy(ctx, r.UID, r.Anonymous)
	if err != nil {
		return OverflowResult{}, err
	}
	now := storedTime(s.now())
	var out OverflowResult
	err = s.Store.Update(ctx, Keys{UID: r.UID}, func(tx *Tx) error {
		w := &tx.Wallet
		if w.Overflow.Month > r.Month {
			return ErrStaleMonth
		}
		es := stamp(refresh(w, g, now), now)
		o := w.Overflow.inMonth(r.Month)
		have := o.Blocks[r.Kind]
		out = OverflowResult{Blocks: have, Spent: o.Spent, Cap: o.MonthlyCap}
		if have >= r.Blocks {
			if len(es) > 0 {
				tx.Entries, w.UpdatedAt, tx.WalletDirty = es, now, true
			}
			return nil
		}
		if !o.Enabled {
			return ErrAllowanceExhausted
		}
		n := r.Blocks - have
		cost := n * b.Coins
		if o.Spent+cost > o.MonthlyCap {
			return &OverflowCapError{Cap: o.MonthlyCap, Spent: o.Spent}
		}
		if w.Available() < cost {
			return &InsufficientError{Required: cost, Balance: w.Available(), Payer: "self"}
		}
		for i := have + 1; i <= r.Blocks; i++ {
			d := spend(w, b.Coins)
			es = append(es, Entry{ID: fmt.Sprintf("overflow_%s_%s_%d", r.Month, r.Kind, i), Kind: KindSpendOverflow, Amount: -b.Coins,
				Delta: Split{Grant: -d.Grant, Initial: -d.Initial, Purchased: -d.Purchased}, Op: r.Kind, AppID: r.AppID, At: now})
		}
		blocks := make(map[string]int64, len(o.Blocks)+1)
		for k, v := range o.Blocks {
			blocks[k] = v
		}
		blocks[r.Kind] = r.Blocks
		o.Blocks, o.Spent = blocks, o.Spent+cost
		w.Overflow, w.UpdatedAt = o, now
		tx.Entries, tx.WalletDirty = es, true
		out = OverflowResult{Blocks: r.Blocks, Bought: n, Spent: o.Spent, Cap: o.MonthlyCap}
		return nil
	})
	if err != nil {
		return OverflowResult{}, err
	}
	if out.Bought > 0 {
		s.log().InfoContext(ctx, EventOverflowBought, slog.String("event", EventOverflowBought),
			slog.String("uid_hash", httpx.UIDHash(r.UID)), slog.String("kind", r.Kind), slog.String("month", r.Month),
			slog.Int64("blocks", out.Blocks), slog.Int64("bought", out.Bought), slog.Int64("coins", out.Bought*b.Coins))
	}
	return out, nil
}

// spend takes amount from w's sources in the spend order (the caller checked
// Available).
func spend(w *Wallet, amount int64) Split {
	var p Split
	rest := amount
	p.Grant = min(w.Grant, rest)
	rest -= p.Grant
	p.Initial = min(w.Initial, rest)
	rest -= p.Initial
	p.Purchased = rest
	w.Grant -= p.Grant
	w.Initial -= p.Initial
	w.Purchased -= p.Purchased
	return p
}

// AllowanceUse is one kind of View.allowance.
type AllowanceUse struct {
	// Used this JST month.
	Used int64 `json:"used"`
	// Limit is the allowance plus the blocks bought this month; omitted
	// when the allowance of the kind is 0 (counted, not enforced).
	Limit int64 `json:"limit,omitempty"`
	// Allowance is ALLOWANCE_PRO_<KIND>; Blocks the blocks bought this month.
	Allowance int64 `json:"allowance,omitempty"`
	Blocks    int64 `json:"blocks,omitempty"`
}

// allowance reads uid's monthly counters (nil when Counters is unset or the
// read failed: the wallet is still answered).
func (s *Service) allowance(ctx context.Context, uid string, w Wallet) map[string]AllowanceUse {
	if s.Counters == nil {
		return nil
	}
	month := MonthKey(s.now())
	cs := make([]ratelimit.Counter, len(AllowanceKinds))
	for i, k := range AllowanceKinds {
		cs[i] = ratelimit.Counter{ID: AllowanceCounterID(s.Env, month, k, uid), Scope: AllowanceScope(k)}
	}
	counts, err := s.Counters.Peek(ctx, cs)
	if err != nil {
		s.log().WarnContext(ctx, "coins: reading the allowance failed", slog.Any("error", err))
		return nil
	}
	o := w.Overflow.inMonth(month)
	out := make(map[string]AllowanceUse, len(AllowanceKinds))
	for i, k := range AllowanceKinds {
		u := AllowanceUse{Used: int64(counts[i]), Allowance: s.Config.Allowance[k], Blocks: o.Blocks[k]}
		if u.Allowance > 0 {
			u.Limit = u.Allowance + u.Blocks*s.Config.Overflow[k].Size
		}
		out[k] = u
	}
	return out
}
