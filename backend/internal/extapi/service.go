package extapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
)

// Errors of Call (Reason maps them to the appconfig error codes).
var (
	// ErrUnknown: no such provider or op, or an argument it does not take.
	ErrUnknown = errors.New("extapi: unknown provider, op or argument")
	// ErrUnavailable is 503 ext_unavailable: EXT_DISABLED, the provider is
	// off, has no key, or (paid, enforce) no COIN_RESERVE_EXT_<PROVIDER>.
	ErrUnavailable = errors.New("extapi: provider unavailable")
	// ErrBusy is 503 ext_busy: this instance runs EXT_MAX_CONCURRENT calls.
	ErrBusy = errors.New("extapi: busy")
	// ErrProRequired is 402 pro_required: external APIs are Pro only (決定 1).
	ErrProRequired = errors.New("extapi: pro required")
	// ErrDuplicateRequest is 409 duplicate_request: the request id already
	// held coins for a paid call (a retry does not run the call again).
	ErrDuplicateRequest = errors.New("extapi: request id already used")
)

// AllowanceError is a call the creator's monthly allowance stopped
// (402 allowance_exhausted / overflow_cap_reached / coins_insufficient).
type AllowanceError struct{ Block quota.Block }

func (e *AllowanceError) Error() string { return "extapi: allowance: " + e.Block.Stop }

// LogEvent is the line of every call (Cloud Logging filter:
// jsonPayload.event="ext.call"). It never has a token, an argument or an
// answer: provider, op, result, time, how it was paid and the uid hash.
const LogEvent = "ext.call"

// RunsCounter counts a call as one `runs` (*quota.Quota).
type RunsCounter interface {
	ExtRun(ctx context.Context, r quota.Run, in *quota.Decision) (quota.Charge, *quota.Block, error)
	// ExtPro is ExtRun's Pro check alone (a cached answer counts nothing
	// but is still Pro only).
	ExtPro(ctx context.Context, r quota.Run, in *quota.Decision) bool
	Refund(ctx context.Context, charges ...quota.Charge) error
}

// CoinBiller holds and charges coins (*coins.Service).
type CoinBiller interface {
	coins.Biller
	Mode() coins.Mode
}

// Service runs external API calls in the runner.
type Service struct {
	cfg   Config
	ops   map[string]map[string]Operation
	oauth map[string]OAuthClient
	sem   chan struct{}
	cache *callCache
	// refreshing serialises token refreshes per uid + provider.
	refreshing keyedLock

	Runs  RunsCounter
	Coins CoinBiller
	Conns *Connections
	// States keeps OAuth state / PKCE (oauth.go).
	States StateStore
	Log    *slog.Logger
	Now    func() time.Time
}

// New builds the enabled providers (EXT_<PROVIDER>_ENABLED, not
// EXT_DISABLED, fakes not in production). A provider whose New fails stays
// off and is logged; a NeedsOAuth provider stays off without a sealer or a
// callback base.
func New(cfg Config, runs RunsCounter, biller CoinBiller, conns *Connections, states StateStore, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = DefaultMaxConcurrent
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	s := &Service{cfg: cfg, ops: map[string]map[string]Operation{}, oauth: map[string]OAuthClient{},
		sem: make(chan struct{}, cfg.MaxConcurrent), cache: newCallCache(), Runs: runs, Coins: biller, Conns: conns, States: states, Log: logger}
	if cfg.Disabled {
		logger.Info("extapi: EXT_DISABLED: every provider is off")
		return s
	}
	for _, p := range Providers() {
		if !cfg.Enabled[p.ID] {
			continue
		}
		off := func(why string, err error) {
			attrs := []any{slog.String("provider", p.ID), slog.String("why", why)}
			if err != nil {
				attrs = append(attrs, slog.Any("error", err))
			}
			logger.Warn("extapi: provider stays off", attrs...)
		}
		if p.Fake && cfg.Production {
			off("fake provider in production", nil)
			continue
		}
		sec := cfg.Secrets[p.ID]
		if p.NeedsOAuth {
			if conns == nil || conns.Sealer == nil || states == nil || cfg.CallbackBase == "" {
				off("OAuth needs EXT_TOKEN_KMS_KEY and EXT_OAUTH_CALLBACK_BASE", nil)
				continue
			}
			cl, err := p.NewOAuth(sec)
			if err != nil {
				off("oauth client", err)
				continue
			}
			s.oauth[p.ID] = cl
		}
		ops, err := p.New(sec)
		if err != nil {
			off("no key", err)
			delete(s.oauth, p.ID)
			continue
		}
		s.ops[p.ID] = ops
		logger.Info("extapi: provider on", slog.String("provider", p.ID), slog.String("billing", string(p.Billing)), slog.Bool("oauth", p.NeedsOAuth))
	}
	return s
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Enabled reports whether provider can be called now.
func (s *Service) Enabled(provider string) bool {
	_, ok := s.ops[provider]
	return ok && !s.cfg.Disabled
}

// EnabledProviders are the provider ids that can be called now, sorted.
func (s *Service) EnabledProviders() []string {
	out := []string{}
	for _, p := range Providers() {
		if s.Enabled(p.ID) {
			out = append(out, p.ID)
		}
	}
	return out
}

// CallRequest is one call.
type CallRequest struct {
	Provider string
	Op       string
	Args     map[string]string
	AppID    string
	// CreatorUID owns the app: its Pro, its `runs`.
	CreatorUID       string
	CreatorAnonymous bool
	// UserUID pressed the button (a person's call): the paid coins and the
	// OAuth token are theirs. "" for a server rule: the creator's.
	UserUID       string
	UserAnonymous bool
	// RequestID makes the coin hold idempotent (X-Request-Id).
	RequestID string
	// Run is the server rule run the call is part of (nil for a person's call).
	Run *quota.Decision
}

func (r CallRequest) actor() (string, bool) {
	if r.UserUID != "" {
		return r.UserUID, r.UserAnonymous
	}
	return r.CreatorUID, r.CreatorAnonymous
}

// CallResult is the answer and what it cost.
type CallResult struct {
	Output Output
	// Charge is the coins of a paid call (zero for a free one).
	Charge coins.Charge
}

// Call checks the provider and op, counts one `runs` (and holds coins for a
// paid provider), calls the op with the person's token when it needs one,
// then commits the cost or, on failure, gives everything back.
func (s *Service) Call(ctx context.Context, r CallRequest) (res CallResult, err error) {
	start := s.now()
	p, spec, err := s.check(r)
	if err != nil {
		return res, err
	}
	billing := "runs"
	var charged int64
	defer func() {
		result := "ok"
		if err != nil {
			result = Reason(err)
		}
		uid, _ := r.actor()
		s.Log.InfoContext(ctx, LogEvent, slog.String("event", LogEvent), slog.String("provider", r.Provider), slog.String("op", r.Op),
			slog.String("result", result), slog.Int64("ms", s.now().Sub(start).Milliseconds()), slog.String("billing", billing),
			slog.Int64("coins", charged), slog.String("app_id", r.AppID), slog.String("uid_hash", httpx.UIDHash(uid)))
	}()

	run := quota.Run{AppID: r.AppID, CreatorUID: r.CreatorUID, CreatorAnonymous: r.CreatorAnonymous, ActorUID: r.UserUID, Attempt: 1}

	// 0. the same arguments answered a moment ago: nothing is counted
	// (still Pro only)
	var cacheKeyOf string
	if spec.CacheTTL > 0 {
		cacheKeyOf = cacheKey(p.ID, spec.Name, r.Args)
		if out, ok := s.cache.get(cacheKeyOf, s.now()); ok {
			if s.Runs != nil && !s.Runs.ExtPro(ctx, run, r.Run) {
				return res, ErrProRequired
			}
			billing = "cache"
			res.Output = out
			return res, nil
		}
	}

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		return res, ErrBusy
	}

	// 1. one `runs` of the creator's month (Pro only)
	var runs quota.Charge
	if s.Runs != nil {
		ch, blk, err := s.Runs.ExtRun(ctx, run, r.Run)
		if err != nil {
			return res, fmt.Errorf("runs: %w", err)
		}
		if blk != nil {
			if blk.Scope == quota.ScopeExtNeedsPro {
				return res, ErrProRequired
			}
			return res, &AllowanceError{Block: *blk}
		}
		runs = ch
	}
	refundRuns := func() {
		if s.Runs == nil || runs.Empty() {
			return
		}
		if err := s.Runs.Refund(context.WithoutCancel(ctx), runs); err != nil {
			s.Log.WarnContext(ctx, "extapi: refunding the run failed", slog.String("provider", r.Provider), slog.Any("error", err))
		}
	}

	// 2. the person's token (the creator's for a server rule)
	in := Input{Args: r.Args, RequestID: r.RequestID}
	who, anon := r.actor()
	var sealed []byte // the stored form of the token used
	if p.NeedsOAuth {
		var t Token
		t, sealed, err = s.token(ctx, who, p.ID)
		if err != nil {
			refundRuns()
			return res, err
		}
		in.Token = &t
	}

	// 3. hold coins (paid)
	var hold *coins.Hold
	if p.Billing == BillingPaid && s.Coins != nil {
		billing = "coins"
		hold, err = s.Coins.Reserve(ctx, coins.ReserveRequest{UID: who, Anonymous: anon, Op: coins.OpExt, AppID: r.AppID,
			RequestID: r.RequestID, Kind: p.ID, Max: s.cfg.Reserve[p.ID]})
		if err != nil {
			refundRuns()
			if errors.Is(err, coins.ErrUnknownOp) {
				return res, ErrUnavailable
			}
			return res, err
		}
		if hold != nil && hold.Existing {
			// the first hold is not this call's: it is neither run nor settled again
			refundRuns()
			return res, ErrDuplicateRequest
		}
	}
	release := func() {
		if err := s.Coins.Release(context.WithoutCancel(ctx), hold); err != nil {
			s.Log.WarnContext(ctx, "extapi: releasing the hold failed", slog.String("provider", r.Provider), slog.Any("error", err))
		}
	}

	// 4. the call
	cctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	out, err := s.ops[p.ID][spec.Name].Do(cctx, in)
	cancel()
	var perr *ProviderError
	billable := errors.As(err, &perr) && perr.Billable
	if err != nil && !billable {
		if hold != nil {
			release()
		}
		refundRuns()
		if errors.Is(err, ErrInvalidGrant) && s.Conns != nil {
			_ = s.Conns.MarkError(context.WithoutCancel(ctx), who, p.ID, sealed)
			return res, ErrNotConnected
		}
		return res, err
	}

	// 5. commit the cost (the rest of the hold goes back)
	usage := out.Usage
	if perr != nil {
		usage = perr.Usage
	}
	if !usage.Priced {
		if c, ok := s.cfg.CostUSD[p.ID+"."+spec.Name]; ok {
			usage = Usage{USD: c, Priced: true}
		}
	}
	if hold != nil {
		// the provider was called: the cost is settled even when the caller is gone
		ch, cerr := s.Coins.Commit(context.WithoutCancel(ctx), hold, coins.Usage{USD: usage.USD, Priced: usage.Priced})
		if cerr != nil {
			s.Log.WarnContext(ctx, "extapi: committing the hold failed", slog.String("provider", r.Provider), slog.Any("error", cerr))
		}
		res.Charge, charged = ch, ch.Coins
	}
	if perr != nil {
		return res, err
	}
	if cacheKeyOf != "" {
		now := s.now()
		s.cache.put(cacheKeyOf, out, now.Add(spec.CacheTTL), now)
	}
	res.Output = out
	return res, nil
}

// check finds the provider and op and checks the arguments.
func (s *Service) check(r CallRequest) (Provider, OpSpec, error) {
	p, ok := Lookup(r.Provider)
	if !ok {
		return p, OpSpec{}, ErrUnknown
	}
	spec, err := p.CheckArgs(r.Op, r.Args)
	if err != nil {
		return p, spec, err
	}
	if r.RequestID == "" {
		return p, spec, fmt.Errorf("%w: a call needs a request id", ErrUnknown)
	}
	if !s.Enabled(p.ID) {
		return p, spec, ErrUnavailable
	}
	if p.Billing == BillingPaid && s.Coins != nil && s.Coins.Mode() == coins.ModeEnforce && s.cfg.Reserve[p.ID] <= 0 {
		return p, spec, ErrUnavailable // would run free under enforce (like coins.CheckReserves)
	}
	if p.NeedsOAuth && (s.oauth[p.ID] == nil || s.Conns == nil) {
		return p, spec, ErrUnavailable
	}
	return p, spec, nil
}

// token reads uid's token of provider, refreshing it when it expired. It
// also returns the stored (sealed) form of the token, which MarkError takes.
func (s *Service) token(ctx context.Context, uid, provider string) (Token, []byte, error) {
	if uid == "" {
		return Token{}, nil, ErrNotConnected
	}
	t, sealed, err := s.Conns.read(ctx, uid, provider)
	if err != nil {
		return t, nil, err
	}
	if !t.expired(s.now()) {
		return t, sealed, nil
	}
	// One refresh at a time per connection on this instance: providers that
	// rotate the refresh token refuse the old one once it was used.
	unlock, err := s.refreshing.lock(ctx, uid+"\x00"+provider)
	if err != nil {
		return Token{}, nil, err
	}
	defer unlock()
	if t, sealed, err = s.Conns.read(ctx, uid, provider); err != nil || !t.expired(s.now()) {
		return t, sealed, err // renewed by the call this one waited for
	}
	nt, err := s.oauth[provider].Refresh(ctx, t)
	if errors.Is(err, ErrInvalidGrant) {
		// another instance may have renewed (and rotated) it meanwhile
		if cur, cs, cerr := s.Conns.read(ctx, uid, provider); cerr == nil && !cur.expired(s.now()) && !bytes.Equal(cs, sealed) {
			return cur, cs, nil
		}
		_ = s.Conns.MarkError(ctx, uid, provider, sealed)
		return Token{}, nil, ErrNotConnected
	}
	if err != nil {
		return Token{}, nil, fmt.Errorf("refresh: %w", err)
	}
	if nt.RefreshToken == "" {
		nt.RefreshToken = t.RefreshToken // providers that do not rotate it
	}
	ns, err := s.Conns.SaveRefreshed(ctx, uid, provider, sealed, nt)
	if errors.Is(err, ErrNotConnected) {
		return Token{}, nil, err // cut (or connected again) while it was renewed
	}
	if err != nil {
		s.Log.WarnContext(ctx, "extapi: saving the refreshed token failed", slog.String("provider", provider), slog.Any("error", err))
	}
	return nt, ns, nil
}

// Reasons are the internal answer's reason (the runner → appconfig body);
// appconfig turns them into its error codes (the contract child issue).
const (
	ReasonOK                 = "ok"
	ReasonBadRequest         = "bad_request"
	ReasonNotConnected       = "ext_not_connected"
	ReasonUnavailable        = "ext_unavailable"
	ReasonBusy               = "ext_busy"
	ReasonDuplicateRequest   = "duplicate_request"
	ReasonProRequired        = "pro_required"
	ReasonAllowanceExhausted = "allowance_exhausted"
	ReasonOverflowCap        = "overflow_cap_reached"
	ReasonCoinsInsufficient  = "coins_insufficient"
	ReasonUpstreamTimeout    = "upstream_timeout"
	ReasonUpstream           = "upstream_unavailable"
	ReasonInternal           = "internal"
)

// Reason is err's reason.
func Reason(err error) string {
	var ae *AllowanceError
	var ins *coins.InsufficientError
	var pe *ProviderError
	switch {
	case err == nil:
		return ReasonOK
	case errors.Is(err, ErrUnknown):
		return ReasonBadRequest
	case errors.Is(err, ErrNotConnected):
		return ReasonNotConnected
	case errors.Is(err, ErrUnavailable):
		return ReasonUnavailable
	case errors.Is(err, ErrBusy):
		return ReasonBusy
	case errors.Is(err, ErrDuplicateRequest):
		return ReasonDuplicateRequest
	case errors.Is(err, ErrProRequired):
		return ReasonProRequired
	case errors.As(err, &ae):
		switch ae.Block.Stop {
		case quota.OutcomeSkippedOverflowCap:
			return ReasonOverflowCap
		case quota.OutcomeSkippedCoins:
			return ReasonCoinsInsufficient
		}
		return ReasonAllowanceExhausted
	case errors.As(err, &ins):
		return ReasonCoinsInsufficient
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonUpstreamTimeout
	case errors.As(err, &pe):
		return ReasonUpstream
	}
	return ReasonInternal
}
