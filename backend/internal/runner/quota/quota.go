// Package quota meters and caps what the runner does (#345, #338): executions,
// records written, notification recipients and scheduled rules. Counters reuse
// ratelimit.Store (Firestore `rateLimits` in production, one document per
// counter per JST day); when the store fails the quota keeps counting in
// memory, like ratelimit. Over a limit the work is stopped, never delayed.
//
// Counter ids ({env}_{yyyy-mm-dd}_…):
//
//	rx_app_{appId}                executions of one app
//	rx_creator_{uid}              executions of all apps of one creator
//	rx_actor_{appId}_{uid}        executions one actor triggered in one app
//	rx_global                     executions in the environment
//	rw_app_{appId}                records written by one app
//	rn_app_{appId}                notification recipients of one app
//	rn_rcpt_app_{appId}_{uid}     notifications one person got from one app
//	rn_rcpt_total_{uid}           notifications one person got in total
//	rr_app_{appId}                records read by one app's rules (each / agg)
package quota

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// Plan is the creator's plan; it picks the free or Pro column of Limits.
type Plan string

const (
	PlanFree Plan = "free"
	PlanPro  Plan = "pro"
	// PlanNone: the creator may not run rules (anonymous, or no Pro: server
	// rules are a Pro feature, #316).
	PlanNone Plan = "none"
)

// Outcomes (the `outcome` of runner.usage and of the run record, #347).
const (
	OutcomeOK            = "ok"
	OutcomeError         = "error"
	OutcomeSkippedQuota  = "skipped_quota"
	OutcomeSkippedGlobal = "skipped_global"
	OutcomeSkippedPaused = "skipped_paused"
	OutcomeSkippedPlan   = "skipped_plan"
)

// Scopes name the counter or cap that stopped something (the `limit` field).
const (
	ScopeAppExec        = "app_exec"
	ScopeCreatorExec    = "creator_exec"
	ScopeActorExec      = "actor_exec"
	ScopeGlobal         = "global"
	ScopeAppWrites      = "app_writes"
	ScopeAppNotify      = "app_notify"
	ScopeRecipientApp   = "recipient_app"
	ScopeRecipientTotal = "recipient_total"
	ScopeRunRecipients  = "run_recipients"
	ScopeRunWrites      = "run_writes"
	ScopeScheduledRules = "scheduled_rules"
	ScopeAppReads       = "app_reads"
	ScopeRunReads       = "run_reads"
	ScopeAnonymous      = "anonymous"
	ScopeSharedNeedsPro = "shared_needs_pro"
	// ScopeNeedsPro: the creator of a private app's rules has no Pro (#316).
	ScopeNeedsPro = "needs_pro"
)

// ProChecker is entitlement.ProChecker (same as ratelimit.ProChecker); nil = nobody is Pro.
type ProChecker interface {
	IsPro(ctx context.Context, uid string) bool
}

// Config configures a Quota.
type Config struct {
	Limits Limits
	// Env prefixes counter ids: staging and production share one Firestore.
	Env string
	// Location is the day boundary; nil = JST.
	Location *time.Location
	// Now is the clock; nil = time.Now.
	Now func() time.Time
	// ProMode is PRO_GATE_MODE (#316): enforce ("" too) runs rules only for
	// Pro creators; log and off run private apps on the free plan as before
	// #316 (log also writes pro_gate.would_block). Shared data needs Pro in
	// every mode (#338).
	ProMode paid.Mode
}

// Run is one execution of one rule (an outbox entry or a scheduled tick).
type Run struct {
	AppID            string
	CreatorUID       string
	CreatorAnonymous bool
	// Shared: the rule acts on shared data (public / shared), which needs Pro.
	Shared bool
	// ActorUID triggered the run; empty for a scheduled trigger.
	ActorUID string
	// Attempt is 1-based. Only attempt 1 is counted; retries charge nothing.
	Attempt int
}

// Block says which counter or cap stopped something.
type Block struct {
	Scope     string
	CounterID string // empty for a per-run cap or a plan check
	Limit     int
	Used      int
}

// Decision is the answer of Begin.
type Decision struct {
	Allowed bool
	Outcome string // OutcomeOK or a skipped_* outcome
	Plan    Plan
	Block   *Block // nil when allowed
	// NearLimit: an execution counter reached 80% (the creator's warning).
	NearLimit bool
	ResetAt   time.Time
	// Day is the quota's day of the run (yyyy-mm-dd in Config.Location).
	Day string
	// Charge is what Begin counted; give it back with Quota.Refund when the
	// run fails for good.
	Charge Charge
	// AppExec is the app's execution counter as Begin saw it (after its own
	// charge when allowed); nil when Begin did not read it (a plan block, a
	// retry, or the counter is off). The creator's screen shows it (#347).
	AppExec *Block

	run      Run
	expireAt time.Time
}

// Charge is what was counted. Exported so the runner can keep it with the
// outbox entry and refund from a later attempt (in-memory charges can only be
// refunded by the same instance).
type Charge struct {
	Parts []ChargePart
}

// ChargePart is one Store.Consume that went through.
type ChargePart struct {
	Counters []ratelimit.Counter
	Weight   int
	ExpireAt time.Time
	// Memory: counted by the in-memory fallback (Firestore failed).
	Memory bool
}

// Empty reports whether nothing was charged.
func (c Charge) Empty() bool { return len(c.Parts) == 0 }

// Quota decides whether runner work may happen and counts it.
type Quota struct {
	cfg      Config
	store    ratelimit.Store
	fallback ratelimit.Store
	pro      ProChecker
	// Logger receives runner.usage and fallback warnings; nil = slog.Default().
	Logger *slog.Logger
}

// New builds a Quota. store nil = memory only.
func New(cfg Config, store ratelimit.Store, pro ProChecker) *Quota {
	if cfg.Location == nil {
		cfg.Location = ratelimit.JST
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if store == nil {
		store = ratelimit.NewMemoryClock(cfg.Now)
	}
	return &Quota{cfg: cfg, store: store, fallback: ratelimit.NewMemoryClock(cfg.Now), pro: pro}
}

// Config returns the effective configuration.
func (q *Quota) Config() Config { return q.cfg }

func (q *Quota) logger() *slog.Logger {
	if q.Logger != nil {
		return q.Logger
	}
	return slog.Default()
}

// Plan decides the creator's plan. A verified Pro wins over anonymity (#314).
// Server rules are a Pro feature (#316, owner's decision 2026-11-06): without
// Pro nothing runs, private apps included: the free allowance is 0. PlanFree
// and the *Free limits stay (a 0 limit means "no counter", not "nothing") for
// PRO_GATE_MODE=log / off, the only cases Plan returns PlanFree.
func (q *Quota) Plan(ctx context.Context, r Run) (Plan, *Block) {
	switch {
	case q.pro != nil && r.CreatorUID != "" && q.pro.IsPro(ctx, r.CreatorUID):
		return PlanPro, nil
	case r.CreatorAnonymous || r.CreatorUID == "":
		return PlanNone, &Block{Scope: ScopeAnonymous}
	case r.Shared:
		return PlanNone, &Block{Scope: ScopeSharedNeedsPro}
	case !q.cfg.ProMode.Checks():
		return PlanFree, nil
	case q.cfg.ProMode.Stop(ctx, q.logger(), "runner", r.CreatorUID, []string{paid.ServerRules}):
		return PlanNone, &Block{Scope: ScopeNeedsPro}
	}
	return PlanFree, nil // PRO_GATE_MODE=log: the pre-#316 free allowance
}

// Begin checks the plan and counts one execution against the app, creator,
// actor and global counters (all or none). A blocked run charges nothing.
func (q *Quota) Begin(ctx context.Context, r Run) (Decision, error) {
	now := q.cfg.Now().In(q.cfg.Location)
	y, m, d := now.Date()
	reset := time.Date(y, m, d+1, 0, 0, 0, 0, q.cfg.Location)
	dec := Decision{
		run: r, Day: now.Format("2006-01-02"), ResetAt: reset,
		// TTL: a day of slack so the document outlives its own day.
		expireAt: reset.Add(24 * time.Hour),
	}
	plan, blk := q.Plan(ctx, r)
	dec.Plan = plan
	if blk != nil {
		dec.Outcome, dec.Block = OutcomeSkippedPlan, blk
		return dec, nil
	}
	dec.Allowed, dec.Outcome = true, OutcomeOK
	if r.Attempt > 1 {
		return dec, nil
	}
	l := q.cfg.Limits
	var counters []ratelimit.Counter
	add := func(scope, id string, limit int) {
		if limit > 0 {
			counters = append(counters, ratelimit.Counter{ID: q.id(dec.Day, id), Scope: scope, Limit: limit})
		}
	}
	add(ScopeAppExec, "rx_app_"+docSafe(r.AppID), pick(plan, l.AppExecFree, l.AppExecPro))
	add(ScopeCreatorExec, "rx_creator_"+docSafe(r.CreatorUID), pick(plan, l.CreatorExecFree, l.CreatorExecPro))
	if r.ActorUID != "" {
		add(ScopeActorExec, "rx_actor_"+docSafe(r.AppID)+"_"+docSafe(r.ActorUID), l.ActorPerApp)
	}
	add(ScopeGlobal, "rx_global", l.Global)
	if len(counters) == 0 {
		return dec, nil
	}
	u, mem, err := q.consume(ctx, counters, 1, dec.expireAt)
	if err != nil {
		return dec, err
	}
	for i, c := range counters {
		if c.Scope == ScopeAppExec {
			dec.AppExec = &Block{Scope: c.Scope, CounterID: c.ID, Limit: c.Limit, Used: u.Counts[i]}
		}
	}
	if u.Blocked >= 0 {
		c := counters[u.Blocked]
		dec.Allowed = false
		dec.Outcome = OutcomeSkippedQuota
		if c.Scope == ScopeGlobal {
			dec.Outcome = OutcomeSkippedGlobal
		}
		dec.Block = &Block{Scope: c.Scope, CounterID: c.ID, Limit: c.Limit, Used: u.Counts[u.Blocked]}
		return dec, nil
	}
	dec.Charge.Parts = append(dec.Charge.Parts, ChargePart{Counters: counters, Weight: 1, ExpireAt: dec.expireAt, Memory: mem})
	for i, c := range counters {
		if c.Scope != ScopeGlobal && u.Counts[i]*5 >= c.Limit*4 {
			dec.NearLimit = true
		}
	}
	return dec, nil
}

// Grant is the answer of Writes: Allowed of the asked n may be written.
type Grant struct {
	Allowed int
	Dropped int
	Block   *Block // why some were dropped (nil when none)
	Charge  Charge
}

// Writes counts n records written by an allowed run: up to the per-run cap,
// then up to what is left of the app's daily writes. The rest must not be
// written (records keep the dropped count, #338).
func (q *Quota) Writes(ctx context.Context, dec Decision, n int) (Grant, error) {
	if !dec.Allowed || n <= 0 {
		return Grant{Dropped: max(0, n)}, nil
	}
	l := q.cfg.Limits
	g := Grant{Allowed: n}
	if c := pick(dec.Plan, l.RunWritesFree, l.RunWritesPro); c > 0 && n > c {
		g.Allowed, g.Block = c, &Block{Scope: ScopeRunWrites, Limit: c, Used: n}
	}
	limit := pick(dec.Plan, l.AppWritesFree, l.AppWritesPro)
	if dec.run.Attempt > 1 || limit <= 0 {
		g.Dropped = n - g.Allowed
		return g, nil
	}
	ctr := ratelimit.Counter{ID: q.id(dec.Day, "rw_app_"+docSafe(dec.run.AppID)), Scope: ScopeAppWrites, Limit: limit}
	got, part, blk, err := q.fill(ctx, ctr, g.Allowed, dec.expireAt)
	if err != nil {
		return Grant{Dropped: n}, err
	}
	if blk != nil {
		g.Block = blk
	}
	g.Allowed = got
	g.Dropped = n - got
	if part != nil {
		g.Charge.Parts = append(g.Charge.Parts, *part)
	}
	return g, nil
}

// RunReadCap is how many records one run may read (each / agg); 0 = no cap.
func (q *Quota) RunReadCap(plan Plan) int {
	l := q.cfg.Limits
	return pick(plan, l.RunReadsFree, l.RunReadsPro)
}

// Reads counts n records read by an allowed run against the app's daily
// reads, all or nothing: blk != nil means the run must stop (a total over part
// of the records would be wrong). Retries count nothing.
func (q *Quota) Reads(ctx context.Context, dec Decision, n int) (Charge, *Block, error) {
	if !dec.Allowed {
		return Charge{}, &Block{Scope: ScopeAppReads}, nil
	}
	limit := pick(dec.Plan, q.cfg.Limits.AppReadsFree, q.cfg.Limits.AppReadsPro)
	if n <= 0 || dec.run.Attempt > 1 || limit <= 0 {
		return Charge{}, nil, nil
	}
	c := ratelimit.Counter{ID: q.id(dec.Day, "rr_app_"+docSafe(dec.run.AppID)), Scope: ScopeAppReads, Limit: limit}
	u, mem, err := q.consume(ctx, []ratelimit.Counter{c}, n, dec.expireAt)
	if err != nil {
		return Charge{}, nil, err
	}
	if u.Blocked >= 0 {
		return Charge{}, &Block{Scope: c.Scope, CounterID: c.ID, Limit: c.Limit, Used: u.Counts[0]}, nil
	}
	return Charge{Parts: []ChargePart{{Counters: []ratelimit.Counter{c}, Weight: n, ExpireAt: dec.expireAt, Memory: mem}}}, nil, nil
}

// Drop is one recipient left out and why.
type Drop struct {
	UID   string
	Block Block
}

// RecipientGrant is the answer of Recipients.
type RecipientGrant struct {
	Allowed []string
	Dropped []Drop
	Charge  Charge
}

// Recipients counts the notifications of an allowed run: the recipients are
// de-duplicated, capped per run, then counted against the app's daily
// recipients and each person's per-app (20) and total (100) limits. Only
// Allowed may be notified; nothing dropped is sent later.
func (q *Quota) Recipients(ctx context.Context, dec Decision, uids []string) (RecipientGrant, error) {
	var g RecipientGrant
	seen := map[string]bool{}
	var list []string
	for _, u := range uids {
		if u != "" && !seen[u] {
			seen[u] = true
			list = append(list, u)
		}
	}
	if !dec.Allowed {
		for _, u := range list {
			g.Dropped = append(g.Dropped, Drop{UID: u, Block: blockOr(dec.Block)})
		}
		return g, nil
	}
	l := q.cfg.Limits
	if c := pick(dec.Plan, l.RunRecipientsFree, l.RunRecipientsPro); c > 0 && len(list) > c {
		for _, u := range list[c:] {
			g.Dropped = append(g.Dropped, Drop{UID: u, Block: Block{Scope: ScopeRunRecipients, Limit: c, Used: len(list)}})
		}
		list = list[:c]
	}
	if dec.run.Attempt > 1 || len(list) == 0 {
		g.Allowed = list
		return g, nil
	}

	// each person's limits first (one transaction per person), so people over
	// their own limit never use the app's recipients
	type taken struct {
		uid  string
		part ChargePart
	}
	var ok []taken
	for _, u := range list {
		var counters []ratelimit.Counter
		if l.RecipientPerApp > 0 {
			counters = append(counters, ratelimit.Counter{ID: q.id(dec.Day, "rn_rcpt_app_"+docSafe(dec.run.AppID)+"_"+docSafe(u)), Scope: ScopeRecipientApp, Limit: l.RecipientPerApp})
		}
		if l.RecipientTotal > 0 {
			counters = append(counters, ratelimit.Counter{ID: q.id(dec.Day, "rn_rcpt_total_"+docSafe(u)), Scope: ScopeRecipientTotal, Limit: l.RecipientTotal})
		}
		if len(counters) == 0 {
			ok = append(ok, taken{uid: u})
			continue
		}
		res, mem, err := q.consume(ctx, counters, 1, dec.expireAt)
		if err != nil {
			for _, t := range ok {
				_ = q.refundPart(ctx, t.part)
			}
			return RecipientGrant{}, err
		}
		if res.Blocked >= 0 {
			c := counters[res.Blocked]
			g.Dropped = append(g.Dropped, Drop{UID: u, Block: Block{Scope: c.Scope, CounterID: c.ID, Limit: c.Limit, Used: res.Counts[res.Blocked]}})
			continue
		}
		ok = append(ok, taken{uid: u, part: ChargePart{Counters: counters, Weight: 1, ExpireAt: dec.expireAt, Memory: mem}})
	}

	// then the app's daily recipients, as many as fit
	got := len(ok)
	if limit := pick(dec.Plan, l.AppNotifyFree, l.AppNotifyPro); limit > 0 && got > 0 {
		ctr := ratelimit.Counter{ID: q.id(dec.Day, "rn_app_"+docSafe(dec.run.AppID)), Scope: ScopeAppNotify, Limit: limit}
		n, part, blk, err := q.fill(ctx, ctr, got, dec.expireAt)
		if err != nil {
			for _, t := range ok {
				_ = q.refundPart(ctx, t.part)
			}
			return RecipientGrant{}, err
		}
		for _, t := range ok[n:] {
			if err := q.refundPart(ctx, t.part); err != nil {
				q.logger().WarnContext(ctx, "runner quota: refund of a dropped recipient failed", slog.Any("error", err))
			}
			g.Dropped = append(g.Dropped, Drop{UID: t.uid, Block: *blk})
		}
		ok = ok[:n]
		g.Charge = appendPart(g.Charge, part)
	}
	for _, t := range ok {
		g.Allowed = append(g.Allowed, t.uid)
		g.Charge = appendPart(g.Charge, &t.part)
	}
	return g, nil
}

// ScheduledRules checks the per-app number of scheduled rules (free 1 / Pro 5).
// The generation stage rejects more; the runner runs only the first allowed.
func (q *Quota) ScheduledRules(plan Plan, n int) (allowed int, blk *Block) {
	if plan == PlanNone {
		return 0, &Block{Scope: ScopeNeedsPro}
	}
	l := q.cfg.Limits
	c := pick(plan, l.ScheduledRulesFree, l.ScheduledRulesPro)
	if c > 0 && n > c {
		return c, &Block{Scope: ScopeScheduledRules, Limit: c, Used: n}
	}
	return n, nil
}

// Refund gives back what the charges counted (a run that failed for good
// after its retries; failures are not billed, #338). Memory parts go to this
// instance's fallback.
func (q *Quota) Refund(ctx context.Context, charges ...Charge) error {
	var first error
	for _, c := range charges {
		for _, p := range c.Parts {
			if err := q.refundPart(ctx, p); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func (q *Quota) refundPart(ctx context.Context, p ChargePart) error {
	if p.Weight <= 0 || len(p.Counters) == 0 {
		return nil
	}
	if p.Memory {
		return q.fallback.Refund(ctx, p.Counters, p.Weight, p.ExpireAt)
	}
	return q.store.Refund(ctx, p.Counters, p.Weight, p.ExpireAt)
}

// consume charges the primary store, or the memory fallback when it fails.
func (q *Quota) consume(ctx context.Context, counters []ratelimit.Counter, weight int, expireAt time.Time) (ratelimit.Usage, bool, error) {
	u, err := q.store.Consume(ctx, counters, weight, expireAt)
	if err == nil {
		return u, false, nil
	}
	q.logger().WarnContext(ctx, "runner quota: store failed; counting in memory", slog.Any("error", err))
	u, err = q.fallback.Consume(ctx, counters, weight, expireAt)
	return u, true, err
}

// fill charges as much of want as fits under one counter. blk is set when
// less than want fit.
func (q *Quota) fill(ctx context.Context, c ratelimit.Counter, want int, expireAt time.Time) (int, *ChargePart, *Block, error) {
	try, used := want, c.Limit
	for range 4 { // a concurrent charge may shrink what is left between tries
		if try <= 0 {
			break
		}
		u, mem, err := q.consume(ctx, []ratelimit.Counter{c}, try, expireAt)
		if err != nil {
			return 0, nil, nil, err
		}
		if u.Blocked < 0 {
			var blk *Block
			if try < want {
				blk = &Block{Scope: c.Scope, CounterID: c.ID, Limit: c.Limit, Used: u.Counts[0]}
			}
			return try, &ChargePart{Counters: []ratelimit.Counter{c}, Weight: try, ExpireAt: expireAt, Memory: mem}, blk, nil
		}
		used = u.Counts[0]
		try = min(try-1, c.Limit-used)
	}
	return 0, nil, &Block{Scope: c.Scope, CounterID: c.ID, Limit: c.Limit, Used: used}, nil
}

func (q *Quota) id(day, rest string) string {
	env := q.cfg.Env
	if env == "" {
		env = "local"
	}
	return fmt.Sprintf("%s_%s_%s", env, day, rest)
}

func appendPart(c Charge, p *ChargePart) Charge {
	if p != nil && p.Weight > 0 {
		c.Parts = append(c.Parts, *p)
	}
	return c
}

func blockOr(b *Block) Block {
	if b != nil {
		return *b
	}
	return Block{}
}

// docSafe keeps a Firestore document id legal ("/" is the only banned rune).
func docSafe(s string) string {
	return strings.ReplaceAll(s, "/", "_")
}
