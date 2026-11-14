package ai

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Coin billing of the AI calls (#630, docs/decisions/630-token-billing.md).
// COIN_BILLING_MODE: off = nothing here runs; log = the would-be charge is
// logged (coins.would_spend / coins.would_block) and nothing blocks;
// enforce = a hold (COIN_RESERVE_<OP>) is taken from the caller before any
// Claude call (402 coins_insufficient when short), the call's cost
// (cost_tally, est_cost_usd) is charged at the end, capped at the hold, and
// a failure gives the whole hold back. The rate limits, the creator-paid caps
// and the global budget stay as runaway limits next to it.
//
// The AI is always paid by its caller, inside a shared app too: a creator's
// Pro may let a participant use the AI (gateText) but never pays the
// participant's coins. Server costs (runner, notifications, storage) stay
// with the creator.

// CoinBiller is the coin wallet (*coins.Service).
type CoinBiller interface {
	coins.Biller
	Mode() coins.Mode
}

// payerSelf is 402 details.payer: the AI is always paid by the caller.
const payerSelf = "self"

// coinSettleTimeout bounds the charge / release writes after the call.
const coinSettleTimeout = 5 * time.Second

// coinLateWait bounds how long a charge waits for a call that outlives the
// response (an early chat reply). A hold left past it expires and comes back.
const coinLateWait = 2 * time.Minute

// coinBill is the hold of one request. A nil bill (billing off, or log
// mode without a payer) does nothing.
type coinBill struct {
	s     *Service
	hold  *coins.Hold
	tally *costTally
}

// coinPayment is who pays a call: always the caller.
type coinPayment struct {
	op, kind, appID string
	uid             string // the caller
	anonymous       bool
}

// reserveCoins takes the hold of p before any Claude call and returns ctx
// with the request's cost tally. Errors are *Error: 401 without a payer,
// 402 coins_insufficient, 409 duplicate_request (the request id already
// held coins), 500 when the wallet cannot be read (enforce only; log mode
// never fails).
func (s *Service) reserveCoins(ctx context.Context, p coinPayment) (context.Context, *coinBill, error) {
	if s.Coins == nil || s.Coins.Mode() == coins.ModeOff || s.Coins.Mode() == "" {
		return ctx, nil, nil
	}
	enforce := s.Coins.Mode() == coins.ModeEnforce
	if p.uid == "" {
		if enforce {
			return ctx, nil, &Error{Status: http.StatusUnauthorized, Code: httpx.CodeUnauthenticated, Message: "missing or invalid bearer token"}
		}
		return ctx, nil, nil
	}
	reqID := httpx.RequestID(ctx) // set by httpx.WithRequestID (cmd/ai)
	if reqID == "" {
		reqID = rand.Text()
	}
	h, err := s.Coins.Reserve(ctx, coins.ReserveRequest{
		UID: p.uid, Anonymous: p.anonymous, Op: p.op, Kind: p.kind, AppID: p.appID, RequestID: reqID,
	})
	var short *coins.InsufficientError
	switch {
	case errors.As(err, &short):
		return ctx, nil, &Error{Status: http.StatusPaymentRequired, Code: httpx.CodeCoinsInsufficient, Message: "not enough coins",
			Details: map[string]any{"required": short.Required, "balance": short.Balance, "payer": payerSelf}}
	case err != nil && enforce:
		s.logger().ErrorContext(ctx, "ai: coin hold failed", slog.Any("error", err), slog.String("op", p.op))
		return ctx, nil, &Error{Status: http.StatusInternalServerError, Code: httpx.CodeInternal, Message: "coin wallet unavailable"}
	case err != nil:
		s.logger().WarnContext(ctx, "ai: coin hold failed (log mode)", slog.Any("error", err), slog.String("op", p.op))
		return ctx, nil, nil
	case h == nil:
		return ctx, nil, nil
	case h.Existing:
		// the same X-Request-Id already held coins: a retry must not run the
		// call again on the first hold (nothing is taken twice)
		return ctx, nil, &Error{Status: http.StatusConflict, Code: httpx.CodeDuplicateRequest, Message: "this request id was already used",
			Details: map[string]any{"status": h.Status}}
	}
	ctx = withCostTally(ctx)
	return ctx, &coinBill{s: s, hold: h, tally: tallyOf(ctx)}, nil
}

// settle closes the hold: a failed call or one without any Claude call
// gives it all back; otherwise the call's cost is charged. The charge is
// returned for the response (nil when nothing was charged in enforce mode,
// or when it is settled after the response because a Claude call is still
// running: an early chat reply). A request whose client is already gone
// (ctx cancelled) never gets the result, so it is released like a failure.
func (b *coinBill) settle(ctx context.Context, failed bool) CoinCharge {
	if b == nil {
		return CoinCharge{}
	}
	failed = failed || ctx.Err() != nil
	ctx = context.WithoutCancel(ctx)
	if !failed && b.tally.busy() {
		go func() {
			wctx, cancel := context.WithTimeout(ctx, coinLateWait)
			b.tally.wait(wctx)
			cancel()
			b.close(ctx, false)
		}()
		return CoinCharge{}
	}
	return b.close(ctx, failed)
}

func (b *coinBill) close(ctx context.Context, failed bool) CoinCharge {
	ctx, cancel := context.WithTimeout(ctx, coinSettleTimeout)
	defer cancel()
	calls, usd, priced := b.tally.total()
	if failed || calls == 0 {
		if err := b.s.Coins.Release(ctx, b.hold); err != nil {
			b.s.logger().WarnContext(ctx, "ai: coin release failed", slog.Any("error", err), slog.String("op", b.hold.Op))
		}
		return CoinCharge{}
	}
	c, err := b.s.Coins.Commit(ctx, b.hold, coins.Usage{USD: usd, Priced: priced})
	if err != nil {
		// the hold stays and expires (coins.hold_expired): the payer is not charged
		b.s.logger().WarnContext(ctx, "ai: coin charge failed", slog.Any("error", err), slog.String("op", b.hold.Op))
		return CoinCharge{}
	}
	if b.hold.DryRun {
		return CoinCharge{}
	}
	return CoinCharge{Coins: &c.Coins, CoinsBalance: &c.Balance}
}

// setHeaders writes X-Coins-Charged / X-Coins-Balance of a JSON response.
func (c CoinCharge) setHeaders(w http.ResponseWriter) {
	if c.Coins == nil {
		return
	}
	w.Header().Set(coins.HeaderCharged, strconv.FormatInt(*c.Coins, 10))
	if c.CoinsBalance != nil {
		w.Header().Set(coins.HeaderBalance, strconv.FormatInt(*c.CoinsBalance, 10))
	}
}

// generateCoinOp is the op a /v1/generate request holds for: undo calls no
// Claude (none), edit holds the edit amount, create / auto the create one
// (auto may turn out an edit or undo: the charge is the real cost).
func generateCoinOp(m Mode) string {
	switch m {
	case ModeUndo:
		return ""
	case ModeEdit:
		return coins.OpEdit
	}
	return coins.OpGenerate
}

// textCoinKind is the heaviest kind of a /v1/text request (same order as
// the rate limit weights): image, long (aiChat too), task, plain.
func textCoinKind(req TextRequest) string {
	switch {
	case req.Image != nil:
		return "image"
	case req.isLong() || req.isChat():
		return "long"
	case req.Task != "" && req.Task != "generate":
		return "task"
	}
	return ""
}

// callerPayment is the caller paying for op.
func callerPayment(ctx context.Context, op, appID string) coinPayment {
	return coinPayment{op: op, appID: appID, uid: auth.UID(ctx), anonymous: auth.Anonymous(ctx)}
}

// textPayment is who pays a /v1/text request gateText let through: the
// caller, also when the creator's Pro let a shared app's participant in
// (payerOf decides access and the creator-paid caps, not the coins).
func textPayment(ctx context.Context, req TextRequest) coinPayment {
	p := callerPayment(ctx, coins.OpText, req.AppID)
	p.kind = textCoinKind(req)
	return p
}

// refuseBeforeClaude gives back what the request was counted for when it
// ends before any Claude call (402 / 409 of the coins): the rate limit units
// and a creator-paid count.
func refuseBeforeClaude(ctx context.Context) {
	ratelimit.Waive(ctx)
	if fn, _ := ctx.Value(creatorRefundKey{}).(func(context.Context)); fn != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), coinSettleTimeout)
		defer cancel()
		fn(rctx)
	}
}
