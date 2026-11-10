package coins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Log events.
const (
	EventWouldSpend   = "coins.would_spend"    // log mode: what a call would have cost
	EventWouldBlock   = "coins.would_block"    // log mode: the payer could not have paid the hold
	EventUnpriced     = "coins.unpriced_model" // the cost was unknown; the hold was charged
	EventOverReserve  = "coins.over_reserve"   // the cost was above the hold; the hold was charged
	EventHoldExpired  = "coins.hold_expired"   // an unsettled hold was returned
	EventCredited     = "coins.credited"       // a pack was credited
	EventNotCommitted = "coins.not_committed"  // Commit on a released / expired hold
)

// ReserveRequest is a hold for one AI call.
type ReserveRequest struct {
	// UID is the payer (the caller, or a shared app's creator in part 2).
	UID string
	// Anonymous: the payer is a Firebase anonymous account (no initial grant).
	Anonymous bool
	Op        string
	AppID     string
	// RequestID is X-Request-Id (httpx.RequestID): the same id holds once.
	RequestID string
	// Kind picks COIN_RESERVE_<OP>_<KIND> when that is set (TextKinds).
	Kind string
	// Max overrides the op's COIN_RESERVE_<OP> when > 0.
	Max int64
}

// Usage is the measured cost of a call (ai cost_tally / ai.usage).
type Usage struct {
	USD float64
	// Priced is false when the model had no price (EstimateCostUSD ok=false):
	// the hold is charged.
	Priced bool
}

// Charge is what Commit charged (X-Coins-Charged / X-Coins-Balance).
type Charge struct {
	Coins   int64
	Balance int64
}

// Biller is what the AI paths call (part 2 of #630). A nil *Hold (mode off)
// is valid for Commit and Release and does nothing.
//
// The ai service calls it from internal/ai/coins.go: generate / chat / text
// Reserve before any Claude call (after gateText for a shared app's
// creator), Commit with the request's cost_tally total, and Release on every
// failure and on a call without Claude.
type Biller interface {
	Reserve(ctx context.Context, r ReserveRequest) (*Hold, error)
	Commit(ctx context.Context, h *Hold, u Usage) (Charge, error)
	Release(ctx context.Context, h *Hold) error
}

// Service is the coin billing of one process. It implements Biller.
type Service struct {
	Config Config
	Store  Store
	// ProPeriodEnd returns the end of uid's active Pro billing period (zero
	// when not Pro). nil = nobody is Pro (no monthly grant).
	ProPeriodEnd func(ctx context.Context, uid string) (time.Time, error)
	// Verifier checks consumable JWS (its ProductIDs should be Config.PackIDs).
	Verifier *appstore.Verifier
	Now      func() time.Time
	Log      *slog.Logger
}

var _ Biller = (*Service)(nil)

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// Mode is the configured mode ("" counts as off).
func (s *Service) Mode() Mode {
	if s == nil || s.Config.Mode == "" {
		return ModeOff
	}
	return s.Config.Mode
}

func (s *Service) policy(ctx context.Context, uid string, anonymous bool) (GrantPolicy, error) {
	g := GrantPolicy{Initial: s.Config.InitialGrant, Anonymous: anonymous, Monthly: s.Config.ProMonthly}
	if g.Monthly > 0 && s.ProPeriodEnd != nil {
		end, err := s.ProPeriodEnd(ctx, uid)
		if err != nil {
			return g, fmt.Errorf("pro period: %w", err)
		}
		g.ProPeriodEnd = end
	}
	return g, nil
}

// View is GET /v1/me/coins.
type View struct {
	Mode           Mode      `json:"mode"`
	Total          int64     `json:"total"`
	Available      int64     `json:"available"`
	Purchased      int64     `json:"purchased"`
	Grant          int64     `json:"grant"`
	GrantPeriodEnd time.Time `json:"grantPeriodEnd,omitzero"`
	Initial        int64     `json:"initial"`
	Reserved       int64     `json:"reserved"`
}

func (s *Service) view(w Wallet) View {
	return View{Mode: s.Mode(), Total: w.Total(), Available: w.Available(), Purchased: w.Purchased,
		Grant: w.Grant, GrantPeriodEnd: w.GrantPeriodEnd, Initial: w.Initial, Reserved: w.Reserved}
}

// Wallet returns uid's balance, applying the grants due (the initial grant
// on the first read of a signed-in account, the monthly grant of a new Pro
// period) and returning expired holds first. Mode off reads nothing.
func (s *Service) Wallet(ctx context.Context, uid string, anonymous bool) (View, error) {
	if s.Mode() == ModeOff {
		return View{Mode: ModeOff}, nil
	}
	g, err := s.policy(ctx, uid, anonymous)
	if err != nil {
		return View{}, err
	}
	w, err := GetWallet(ctx, s.Store, uid, g, s.now())
	if err != nil {
		return View{}, err
	}
	if s.expireDue(ctx, uid, w) {
		if w, err = GetWallet(ctx, s.Store, uid, g, s.now()); err != nil {
			return View{}, err
		}
	}
	return s.view(w), nil
}

// expireDue returns uid's holds past their expireAt; true when one was.
func (s *Service) expireDue(ctx context.Context, uid string, w Wallet) bool {
	now, expired := s.now(), false
	for id, at := range w.Holds {
		if at.After(now) {
			continue
		}
		if err := s.expire(ctx, HoldRef{UID: uid, ID: id}); err != nil {
			s.log().WarnContext(ctx, "coins: expire hold failed", slog.Any("error", err))
			continue
		}
		expired = true
	}
	return expired
}

func (s *Service) expire(ctx context.Context, ref HoldRef) error {
	h, _, err := SettleHold(ctx, s.Store, ref.UID, ref.ID, Settlement{Status: HoldExpired}, s.now())
	if err != nil {
		return err
	}
	if h.Status == HoldExpired {
		s.log().WarnContext(ctx, EventHoldExpired, slog.String("event", EventHoldExpired),
			slog.String("uid_hash", httpx.UIDHash(ref.UID)), slog.String("op", h.Op), slog.Int64("coins", h.Amount))
	}
	return nil
}

// Sweep returns up to limit expired holds of anyone. Nothing calls it yet
// (no scheduled job runs it): expired holds are returned when their payer's
// wallet is next read (Wallet, Reserve). Wiring it to a scheduler is an infra
// change.
func (s *Service) Sweep(ctx context.Context, limit int) (int, error) {
	if s.Mode() != ModeEnforce {
		return 0, nil
	}
	refs, err := s.Store.Expired(ctx, s.now(), limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range refs {
		if err := s.expire(ctx, r); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Ledger is uid's history, newest first (limit 1..100, default 20).
func (s *Service) Ledger(ctx context.Context, uid string, limit int, cursor string) ([]Entry, string, error) {
	if s.Mode() == ModeOff {
		return []Entry{}, "", nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.Store.Ledger(ctx, uid, limit, cursor)
}

// Reserve implements Biller. Mode off returns nil; log returns a DryRun hold
// (nothing taken; coins.would_block when the payer is short); enforce takes
// the hold or fails with *InsufficientError.
func (s *Service) Reserve(ctx context.Context, r ReserveRequest) (*Hold, error) {
	mode := s.Mode()
	if mode == ModeOff {
		return nil, nil
	}
	amount, ok := s.Config.Reserve[r.Op]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownOp, r.Op)
	}
	if v, ok := s.Config.Reserve[r.Op+"_"+r.Kind]; ok && r.Kind != "" {
		amount = v
	}
	if r.Max > 0 {
		amount = r.Max
	}
	if r.RequestID == "" {
		return nil, errors.New("coins: reserve needs a request id")
	}
	now := s.now()
	h := Hold{Op: r.Op, AppID: r.AppID, RequestID: r.RequestID, Amount: amount, ExpireAt: now.Add(s.Config.HoldTTL)}
	g, err := s.policy(ctx, r.UID, r.Anonymous)
	if err != nil {
		return nil, err
	}
	// grants due and expired holds back first
	w, err := GetWallet(ctx, s.Store, r.UID, g, now)
	if err != nil {
		return nil, err
	}
	if s.expireDue(ctx, r.UID, w) && mode == ModeLog {
		if w, err = GetWallet(ctx, s.Store, r.UID, g, now); err != nil {
			return nil, err
		}
	}
	if mode == ModeLog {
		if w.Available() < amount {
			s.log().InfoContext(ctx, EventWouldBlock, slog.String("event", EventWouldBlock),
				slog.String("uid_hash", httpx.UIDHash(r.UID)), slog.String("op", r.Op),
				slog.Int64("required", amount), slog.Int64("balance", w.Available()))
		}
		h.ID, h.UID, h.DryRun, h.Status = HoldID(r.UID, r.RequestID), r.UID, true, HoldHeld
		return &h, nil
	}
	out, _, err := ReserveHold(ctx, s.Store, r.UID, h, g, now)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Commit implements Biller: the call's cost in coins (Config.Price), capped
// at the hold, is charged and the rest goes back. A hold that already
// expired is not charged (ErrNotHeld is logged, not returned).
func (s *Service) Commit(ctx context.Context, h *Hold, u Usage) (Charge, error) {
	if h == nil {
		return Charge{}, nil
	}
	coins := s.Config.Price(u.USD)
	attrs := []any{slog.String("uid_hash", httpx.UIDHash(h.UID)), slog.String("op", h.Op), slog.Int64("reserved", h.Amount)}
	if !u.Priced {
		coins = h.Amount
		s.log().WarnContext(ctx, EventUnpriced, append(attrs, slog.String("event", EventUnpriced))...)
	} else if coins > h.Amount {
		s.log().WarnContext(ctx, EventOverReserve, append(attrs, slog.String("event", EventOverReserve), slog.Int64("coins", coins))...)
		coins = h.Amount
	}
	if h.DryRun {
		s.log().InfoContext(ctx, EventWouldSpend, append(attrs, slog.String("event", EventWouldSpend), slog.Int64("coins", coins), slog.Float64("usd", u.USD))...)
		return Charge{}, nil
	}
	out, w, err := SettleHold(ctx, s.Store, h.UID, h.ID, Settlement{Commit: true, Charge: coins, USD: u.USD}, s.now())
	if errors.Is(err, ErrNotHeld) {
		s.log().WarnContext(ctx, EventNotCommitted, append(attrs, slog.String("event", EventNotCommitted))...)
		return Charge{Balance: w.Available()}, nil
	}
	if err != nil {
		return Charge{}, err
	}
	*h = out
	return Charge{Coins: out.Charged, Balance: w.Available()}, nil
}

// Release implements Biller: the whole hold goes back (refund_failed in
// the design: the user got nothing).
func (s *Service) Release(ctx context.Context, h *Hold) error {
	if h == nil || h.DryRun {
		return nil
	}
	out, _, err := SettleHold(ctx, s.Store, h.UID, h.ID, Settlement{Status: HoldReleased}, s.now())
	if err != nil {
		return err
	}
	if out.ID != "" {
		*h = out
	}
	return nil
}

// Credit is POST /v1/me/coins/apple.
type Credit struct {
	// Credited is false when this transaction was already credited (a retry).
	Credited      bool   `json:"credited"`
	TransactionID string `json:"transactionId"`
	ProductID     string `json:"productId"`
	Coins         int64  `json:"coins"`
	Wallet        View   `json:"wallet"`
}

// CreditApple verifies a consumable JWS and credits its pack once per
// transactionId. Errors: ErrOff, ErrAnonymous, *appstore.Error,
// ErrOwnedByOther.
func (s *Service) CreditApple(ctx context.Context, uid string, anonymous bool, signedJWS string) (Credit, error) {
	if s.Mode() == ModeOff {
		return Credit{}, ErrOff
	}
	if anonymous {
		return Credit{}, ErrAnonymous
	}
	if s.Verifier == nil {
		return Credit{}, errors.New("coins: no verifier")
	}
	tx, err := s.Verifier.VerifyConsumable(signedJWS)
	if err != nil {
		return Credit{}, err
	}
	coins, ok := s.Config.PackCoins(tx.ProductID)
	if !ok { // the verifier allows only pack ids; a mismatch is a configuration error
		return Credit{}, &appstore.Error{Reason: appstore.ReasonProduct, Err: fmt.Errorf("productId %q is not a coin pack", tx.ProductID)}
	}
	g, err := s.policy(ctx, uid, false)
	if err != nil {
		return Credit{}, err
	}
	p := Purchase{TransactionID: tx.TransactionID, ProductID: tx.ProductID, Coins: coins, Environment: tx.Environment}
	w, credited, err := CreditPurchase(ctx, s.Store, uid, p, g, s.now())
	if err != nil {
		if errors.Is(err, ErrOwnedByOther) {
			s.log().WarnContext(ctx, "coins: transaction credited to another account", slog.String("uid_hash", httpx.UIDHash(uid)))
		}
		return Credit{}, err
	}
	s.log().InfoContext(ctx, EventCredited, slog.String("event", EventCredited), slog.String("uid_hash", httpx.UIDHash(uid)),
		slog.String("productId", tx.ProductID), slog.String("environment", tx.Environment), slog.Bool("credited", credited))
	return Credit{Credited: credited, TransactionID: tx.TransactionID, ProductID: tx.ProductID, Coins: coins, Wallet: s.view(w)}, nil
}

// PublicConfig is GET /v1/coins/config: the numbers the iOS app shows, so
// it never hardcodes them. It never includes COIN_PER_USD.
type PublicConfig struct {
	Mode         Mode             `json:"mode"`
	Packs        []Pack           `json:"packs"`
	Estimates    map[string]int64 `json:"estimates"`
	InitialGrant int64            `json:"initialGrant"`
	ProMonthly   int64            `json:"proMonthly"`
	Allowance    map[string]int64 `json:"allowance"`
}

// Public returns the display config.
func (c Config) Public() PublicConfig {
	packs := c.Packs
	if packs == nil {
		packs = []Pack{}
	}
	nz := func(m map[string]int64) map[string]int64 {
		out := map[string]int64{}
		for k, v := range m {
			if v > 0 {
				out[k] = v
			}
		}
		return out
	}
	mode := c.Mode
	if mode == "" {
		mode = ModeOff
	}
	return PublicConfig{Mode: mode, Packs: packs, Estimates: nz(c.Estimates), InitialGrant: c.InitialGrant, ProMonthly: c.ProMonthly, Allowance: nz(c.Allowance)}
}
