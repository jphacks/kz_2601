package quota

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Pro's monthly server allowance (#630 part 3,
// docs/decisions/630-token-billing.md 「サーバーの枠と、超えた後のコイン払い」).
// Behind COIN_BILLING_MODE: off counts nothing (the daily limits only); log
// counts each Pro creator's runs / recipients / writes / reads per JST month
// and logs coins.would_overflow; enforce stops the work past
// ALLOWANCE_PRO_<KIND> unless a block of COIN_OVERFLOW_BLOCK_<KIND> units can
// be bought with the creator's coins (coins.Service.BuyOverflow). The daily
// counters stay as runaway limits and are checked first.
//
// Counter ids ({env}_{yyyymm}_…, coins.AllowanceCounterID):
//
//	ma_{kind}_{uid}                 the creator's use of kind this month (all apps, shared ones too)
//	mq_warn_{uid}_{kind}_{level}    the creator's monthly 80% / 100% push was sent (limit 1)

// Outcomes of a run the monthly allowance stopped.
const (
	// OutcomeSkippedAllowance: the allowance is used up and overflow is off
	// (or the kind has no block).
	OutcomeSkippedAllowance = "skipped_allowance"
	// OutcomeSkippedOverflowCap: buying the block would pass the owner's
	// monthly coin cap.
	OutcomeSkippedOverflowCap = "skipped_overflow_cap"
	// OutcomeSkippedCoins: the creator has fewer coins than the block costs.
	OutcomeSkippedCoins = "skipped_coins"
)

// NoticeCap is the Notice kind of the owner's monthly coin cap.
const NoticeCap = "cap"

// uncapped is the limit of a monthly counter that only counts.
const uncapped = math.MaxInt32

// AllowanceConfig is the monthly allowance (from coins.Config).
type AllowanceConfig struct {
	Mode coins.Mode
	// Limits per kind (ALLOWANCE_PRO_<KIND>); 0 = counted, not enforced.
	Limits map[string]int64
	// Blocks per kind (COIN_OVERFLOW_BLOCK_<KIND> / COIN_OVERFLOW_PRICE_<KIND>).
	Blocks map[string]coins.Block
}

// AllowanceFrom takes the allowance of the coin settings.
func AllowanceFrom(c coins.Config) AllowanceConfig {
	return AllowanceConfig{Mode: c.Mode, Limits: c.Allowance, Blocks: c.Overflow}
}

func (a AllowanceConfig) on() bool { return a.Mode == coins.ModeLog || a.Mode == coins.ModeEnforce }

// OverflowBuyer buys allowance blocks with coins (coins.Service).
type OverflowBuyer interface {
	BuyOverflow(ctx context.Context, r coins.OverflowRequest) (coins.OverflowResult, error)
}

// Notice is a monthly 80% / 100% level a run reached: of an allowance kind,
// or of the owner's coin cap (Kind NoticeCap). The runner pushes each to the
// creator once a month (ClaimNotice).
type Notice struct {
	Kind  string
	Level int
	// Stopped: work was stopped (no block was bought).
	Stopped bool
}

// month is the monthly part of one run's Decision (nil = not counted).
type month struct {
	key      string // coins.MonthKey
	expireAt time.Time
	mu       sync.Mutex
	notices  []Notice
}

func (m *month) add(n Notice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, o := range m.notices {
		if o.Kind == n.Kind && o.Level == n.Level {
			m.notices[i].Stopped = o.Stopped || n.Stopped
			return
		}
	}
	m.notices = append(m.notices, n)
}

// reached notes the allowance level of an allowed charge: 100% once used
// is past the allowance (paid with blocks), 80% before. The 100% of a stop
// is noted by allow.
func (m *month) reached(kind string, base, used int64) {
	switch {
	case base <= 0:
	case used > base:
		m.add(Notice{Kind: kind, Level: WarnFull})
	case used*5 >= base*4:
		m.add(Notice{Kind: kind, Level: WarnNear})
	}
}

// Notices are the monthly levels this run reached so far.
func (d Decision) Notices() []Notice {
	if d.month == nil {
		return nil
	}
	d.month.mu.Lock()
	defer d.month.mu.Unlock()
	return append([]Notice(nil), d.month.notices...)
}

// Month is the JST month (yyyymm) the run is counted in ("" = not counted).
func (d Decision) Month() string {
	if d.month == nil {
		return ""
	}
	return d.month.key
}

// newMonth starts the monthly part of a run: Pro creators only, first
// attempts only (retries count nothing, like the daily counters).
func (q *Quota) newMonth(now time.Time, plan Plan, r Run) *month {
	if !q.cfg.Allowance.on() || plan != PlanPro || r.CreatorUID == "" || r.Attempt > 1 {
		return nil
	}
	j := now.In(ratelimit.JST)
	y, mo, _ := j.Date()
	// TTL: a week past the month, so the month's last reads find it
	next := time.Date(y, mo+1, 1, 0, 0, 0, 0, ratelimit.JST)
	return &month{key: coins.MonthKey(now), expireAt: next.Add(7 * 24 * time.Hour)}
}

// blockCache remembers the blocks each creator owns this month on this
// instance, so a run past the allowance does not read the wallet each time.
// A stale (lower) value only costs one BuyOverflow that buys nothing.
type blockCache struct {
	mu    sync.Mutex
	month string
	n     map[string]int64
}

func (c *blockCache) get(month, uid, kind string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.month != month {
		return 0
	}
	return c.n[uid+"\x00"+kind]
}

func (c *blockCache) set(month, uid, kind string, n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.month != month {
		c.month, c.n = month, map[string]int64{}
	}
	c.n[uid+"\x00"+kind] = max(c.n[uid+"\x00"+kind], n)
}

// allow counts n units of kind against the creator's month. stop != nil
// (Block.Stop set) means the units must not be used; nothing was counted.
func (q *Quota) allow(ctx context.Context, dec Decision, kind string, n int) (*ChargePart, *Block, error) {
	m := dec.month
	if m == nil || n <= 0 {
		return nil, nil, nil
	}
	a := q.cfg.Allowance
	base, uid := a.Limits[kind], dec.run.CreatorUID
	ctr := ratelimit.Counter{ID: coins.AllowanceCounterID(q.cfg.Env, m.key, kind, uid), Scope: coins.AllowanceScope(kind), Limit: uncapped}
	charged := func(mem bool) *ChargePart {
		return &ChargePart{Counters: []ratelimit.Counter{ctr}, Weight: n, ExpireAt: m.expireAt, Memory: mem}
	}
	if a.Mode != coins.ModeEnforce || base <= 0 {
		u, mem, err := q.consume(ctx, []ratelimit.Counter{ctr}, n, m.expireAt)
		if err != nil {
			return nil, nil, err
		}
		if a.Mode == coins.ModeLog && base > 0 {
			q.wouldOverflow(ctx, dec, kind, base, int64(u.Counts[0]), int64(n))
		}
		return charged(mem), nil, nil
	}
	blk := a.Blocks[kind]
	blocks := q.blocks.get(m.key, uid, kind)
	for range 3 { // a concurrent run may use the block just bought
		ctr.Limit = int(min(base+blocks*blk.Size, uncapped))
		u, mem, err := q.consume(ctx, []ratelimit.Counter{ctr}, n, m.expireAt)
		if err != nil {
			return nil, nil, err
		}
		used := int64(u.Counts[0])
		if u.Blocked < 0 {
			m.reached(kind, base, used)
			return charged(mem), nil, nil
		}
		stop := func(outcome string) (*ChargePart, *Block, error) {
			m.add(Notice{Kind: kind, Level: WarnFull, Stopped: true})
			return nil, &Block{Scope: ctr.Scope, CounterID: ctr.ID, Limit: ctr.Limit, Used: int(used), Stop: outcome}, nil
		}
		if !blk.On() || q.Buyer == nil {
			return stop(OutcomeSkippedAllowance)
		}
		need := used + int64(n) - base
		res, err := q.Buyer.BuyOverflow(ctx, coins.OverflowRequest{UID: uid, Anonymous: dec.run.CreatorAnonymous, Kind: kind,
			AppID: dec.run.AppID, Month: m.key, Blocks: (need + blk.Size - 1) / blk.Size})
		var capErr *coins.OverflowCapError
		var short *coins.InsufficientError
		switch {
		case err == nil:
		case errors.Is(err, coins.ErrAllowanceExhausted), errors.Is(err, coins.ErrOff):
			return stop(OutcomeSkippedAllowance)
		case errors.As(err, &capErr):
			part, blk, err := stop(OutcomeSkippedOverflowCap)
			m.add(Notice{Kind: NoticeCap, Level: WarnFull, Stopped: true})
			return part, blk, err
		case errors.As(err, &short):
			return stop(OutcomeSkippedCoins)
		default:
			return nil, nil, fmt.Errorf("allowance %s: %w", kind, err)
		}
		blocks = res.Blocks
		q.blocks.set(m.key, uid, kind, blocks)
		if res.Cap > 0 {
			switch {
			case res.Spent >= res.Cap:
				m.add(Notice{Kind: NoticeCap, Level: WarnFull})
			case res.Spent*5 >= res.Cap*4:
				m.add(Notice{Kind: NoticeCap, Level: WarnNear})
			}
		}
	}
	return nil, nil, fmt.Errorf("allowance %s: still over after buying blocks", kind)
}

// wouldOverflow logs coins.would_overflow (log mode) when this charge
// crossed the allowance or the next block.
func (q *Quota) wouldOverflow(ctx context.Context, dec Decision, kind string, base, used, n int64) {
	size := max(q.cfg.Allowance.Blocks[kind].Size, 1)
	blocks := func(u int64) int64 {
		if u <= base {
			return 0
		}
		return (u - base + size - 1) / size
	}
	after := blocks(used)
	if after <= blocks(used-n) {
		return
	}
	q.logger().InfoContext(ctx, coins.EventWouldOverflow, slog.String("event", coins.EventWouldOverflow),
		slog.String("creator_uid_hash", httpx.UIDHash(dec.run.CreatorUID)), slog.String("kind", kind),
		slog.String("month", dec.month.key), slog.Int64("used", used), slog.Int64("allowance", base),
		slog.Int64("blocks", after), slog.Int64("coins", after*q.cfg.Allowance.Blocks[kind].Coins))
}

// refundQuiet refunds what a run counted before the allowance stopped it.
func (q *Quota) refundQuiet(ctx context.Context, c Charge) {
	if err := q.Refund(ctx, c); err != nil {
		q.logger().WarnContext(ctx, "runner quota: refund after an allowance stop failed", slog.Any("error", err))
	}
}
