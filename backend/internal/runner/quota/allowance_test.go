package quota

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// #630 part 3: runs 3 / writes 4 / reads 10 / recipients 2 a month; only
// runs can be bought past the allowance (2 runs for 5 coins).
var testAllowance = AllowanceConfig{
	Limits: map[string]int64{coins.KindRuns: 3, coins.KindWrites: 4, coins.KindReads: 10, coins.KindRecipients: 2},
	Blocks: map[string]coins.Block{coins.KindRuns: {Size: 2, Coins: 5}},
}

// wallet is the coin side: a real coins.Service on memory.
type wallet struct {
	*coins.Service
	t *testing.T
}

func newWallet(t *testing.T, c *clock) wallet {
	return wallet{Service: &coins.Service{Config: coins.Config{Mode: coins.ModeEnforce, Overflow: testAllowance.Blocks}, Store: coins.NewMemory(), Now: c.now}, t: t}
}

// fund gives uid n coins and sets overflow.
func (w wallet) fund(uid string, n int64, enabled bool, monthlyCap int64) {
	w.t.Helper()
	ctx := context.Background()
	if n > 0 {
		if _, _, err := coins.CreditPurchase(ctx, w.Store, uid, coins.Purchase{TransactionID: "fund-" + uid, ProductID: "p", Coins: n}, coins.GrantPolicy{Anonymous: true}, w.Now()); err != nil {
			w.t.Fatal(err)
		}
	}
	if _, err := w.SetOverflow(ctx, uid, true, coins.OverflowSettings{Enabled: enabled, MonthlyCap: monthlyCap}); err != nil {
		w.t.Fatal(err)
	}
}

func (w wallet) get(uid string) coins.Wallet {
	w.t.Helper()
	got, err := coins.GetWallet(context.Background(), w.Store, uid, coins.GrantPolicy{Anonymous: true}, w.Now())
	if err != nil {
		w.t.Fatal(err)
	}
	return got
}

func (w wallet) overflowRows(uid string) int {
	w.t.Helper()
	es, _, err := w.Store.Ledger(context.Background(), uid, 100, "")
	if err != nil {
		w.t.Fatal(err)
	}
	n := 0
	for _, e := range es {
		if e.Kind == coins.KindSpendOverflow {
			n++
		}
	}
	return n
}

func allowanceQuota(st ratelimit.Store, env string, c *clock, mode coins.Mode, buyer OverflowBuyer) *Quota {
	if st == nil {
		st = ratelimit.NewMemoryClock(c.now)
	}
	a := testAllowance
	a.Mode = mode
	q := New(Config{Env: env, Now: c.now, Allowance: a}, st, everyonePro{})
	if buyer != nil {
		q.Buyer = buyer
	}
	return q
}

func jstClock() *clock { return &clock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, ratelimit.JST)} }

func outcomes(t *testing.T, q *Quota, n int) []string {
	t.Helper()
	var out []string
	for range n {
		out = append(out, begin(t, q, appA).Outcome)
	}
	return out
}

func equal(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

// COIN_BILLING_MODE=off: nothing monthly is counted, nothing stops.
func TestAllowanceModeOff(t *testing.T) {
	c := jstClock()
	w := newWallet(t, c)
	q := allowanceQuota(nil, "test", c, coins.ModeOff, w)
	for i := range 10 {
		d := begin(t, q, appA)
		if !d.Allowed || d.Month() != "" || d.Notices() != nil || len(d.Charge.Parts) != 0 {
			t.Fatalf("run %d: %+v", i, d)
		}
	}
	g, err := q.Writes(context.Background(), begin(t, q, appA), 50)
	if err != nil || g.Allowed != 50 {
		t.Fatalf("writes %+v %v", g, err)
	}
}

// Free creators (PRO_GATE_MODE=log) have no monthly allowance.
func TestAllowanceProOnly(t *testing.T) {
	c := jstClock()
	a := testAllowance
	a.Mode = coins.ModeEnforce
	q := New(Config{Env: "test", Now: c.now, Allowance: a, ProMode: "log"}, ratelimit.NewMemoryClock(c.now), proSet{})
	for range 5 {
		if d := begin(t, q, appA); !d.Allowed || d.Plan != PlanFree || d.Month() != "" {
			t.Fatalf("free: %+v", d)
		}
	}
}

// log mode counts and logs coins.would_overflow when a block would be
// bought, but never stops.
func TestAllowanceModeLog(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		c := jstClock()
		q := allowanceQuota(st, env, c, coins.ModeLog, nil)
		var buf bytes.Buffer
		q.Logger = slog.New(slog.NewJSONHandler(&buf, nil))
		for i := range 6 {
			if d := begin(t, q, appA); !d.Allowed || d.Month() != "202611" {
				t.Fatalf("run %d: %+v", i, d)
			}
		}
		// runs 4 (block 1) and 6 (block 2) cross a block
		if n := strings.Count(buf.String(), `"event":"coins.would_overflow"`); n != 2 {
			t.Fatalf("would_overflow lines: %d\n%s", n, buf.String())
		}
		if strings.Contains(buf.String(), "alice") {
			t.Fatalf("raw uid in the log: %s", buf.String())
		}
	})
}

// Overflow off: the run past the allowance stops with skipped_allowance and
// counts nothing; the creator gets a stopped 100% notice.
func TestAllowanceExhaustedStops(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		c := jstClock()
		w := newWallet(t, c)
		w.fund("alice", 100, false, 100)
		q := allowanceQuota(st, env, c, coins.ModeEnforce, w)
		got := outcomes(t, q, 5)
		if want := []string{OutcomeOK, OutcomeOK, OutcomeOK, OutcomeSkippedAllowance, OutcomeSkippedAllowance}; !equal(got, want) {
			t.Fatalf("outcomes %v, want %v", got, want)
		}
		d := begin(t, q, appA)
		if d.Allowed || d.Block == nil || d.Block.Scope != "allowance_runs" || d.Block.Stop != OutcomeSkippedAllowance || !d.Charge.Empty() {
			t.Fatalf("stopped run %+v", d)
		}
		if ns := d.Notices(); len(ns) != 1 || ns[0] != (Notice{Kind: coins.KindRuns, Level: WarnFull, Stopped: true}) {
			t.Fatalf("notices %+v", ns)
		}
		if w.get("alice").Purchased != 100 || w.overflowRows("alice") != 0 {
			t.Fatal("coins were spent with overflow off")
		}
		// another creator is not affected
		if d := begin(t, q, appB); !d.Allowed {
			t.Fatalf("carol: %+v", d)
		}
	})
}

// Overflow on: past the allowance one block is bought and the runs go on;
// the next block is bought when that one is used.
func TestAllowanceOverflowBuysBlocks(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		c := jstClock()
		w := newWallet(t, c)
		w.fund("alice", 100, true, 100)
		q := allowanceQuota(st, env, c, coins.ModeEnforce, w)
		for i := range 7 { // 3 allowance + 2 blocks of 2
			if d := begin(t, q, appA); !d.Allowed {
				t.Fatalf("run %d: %+v", i, d)
			}
		}
		wl := w.get("alice")
		if wl.Purchased != 90 || wl.Overflow.Spent != 10 || wl.Overflow.Blocks[coins.KindRuns] != 2 || w.overflowRows("alice") != 2 {
			t.Fatalf("wallet %+v", wl)
		}
		// a fresh instance (empty cache) reads the blocks and buys nothing more
		q2 := allowanceQuota(st, env, c, coins.ModeEnforce, w)
		if st == nil { // memory: the instances must share the counters
			q2 = q
		}
		d := begin(t, q2, appA)
		if !d.Allowed || w.overflowRows("alice") != 3 {
			t.Fatalf("8th run buys the third block: %+v rows %d", d, w.overflowRows("alice"))
		}
	})
}

// The cap: a block that would pass the owner's monthly cap is not bought.
func TestAllowanceOverflowCap(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		c := jstClock()
		w := newWallet(t, c)
		w.fund("alice", 100, true, 6) // one block (5) fits, two do not
		q := allowanceQuota(st, env, c, coins.ModeEnforce, w)
		got := outcomes(t, q, 6)
		if want := []string{OutcomeOK, OutcomeOK, OutcomeOK, OutcomeOK, OutcomeOK, OutcomeSkippedOverflowCap}; !equal(got, want) {
			t.Fatalf("outcomes %v, want %v", got, want)
		}
		d := begin(t, q, appA)
		if d.Block == nil || d.Block.Stop != OutcomeSkippedOverflowCap {
			t.Fatalf("stopped %+v", d)
		}
		ns := d.Notices()
		if len(ns) != 2 || ns[0] != (Notice{Kind: coins.KindRuns, Level: WarnFull, Stopped: true}) || ns[1] != (Notice{Kind: NoticeCap, Level: WarnFull, Stopped: true}) {
			t.Fatalf("notices %+v", ns)
		}
		if wl := w.get("alice"); wl.Overflow.Spent != 5 || wl.Purchased != 95 {
			t.Fatalf("wallet %+v", wl)
		}
	})
}

func TestAllowanceOverflowCoinsShort(t *testing.T) {
	c := jstClock()
	w := newWallet(t, c)
	w.fund("alice", 4, true, 100)
	q := allowanceQuota(nil, "test", c, coins.ModeEnforce, w)
	got := outcomes(t, q, 4)
	if want := []string{OutcomeOK, OutcomeOK, OutcomeOK, OutcomeSkippedCoins}; !equal(got, want) {
		t.Fatalf("outcomes %v, want %v", got, want)
	}
}

// The allowance is per JST month: the 1st (JST) starts again.
func TestAllowanceMonthBoundary(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		c := &clock{t: time.Date(2026, 11, 30, 23, 0, 0, 0, ratelimit.JST)}
		w := newWallet(t, c)
		w.fund("alice", 0, false, 0)
		q := allowanceQuota(st, env, c, coins.ModeEnforce, w)
		if got := outcomes(t, q, 4); got[3] != OutcomeSkippedAllowance {
			t.Fatalf("november %v", got)
		}
		c.t = time.Date(2026, 11, 30, 15, 30, 0, 0, time.UTC) // 2026-12-01 00:30 JST
		d := begin(t, q, appA)
		if !d.Allowed || d.Month() != "202612" {
			t.Fatalf("december %+v", d)
		}
	})
}

// Two runner instances past the allowance at once buy each block once.
func TestAllowanceConcurrentInstances(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		c := jstClock()
		w := newWallet(t, c)
		w.fund("alice", 100, true, 100)
		if st == nil {
			st = ratelimit.NewMemoryClock(c.now)
		}
		qs := []*Quota{allowanceQuota(st, env, c, coins.ModeEnforce, w), allowanceQuota(st, env, c, coins.ModeEnforce, w)}
		var wg sync.WaitGroup
		var mu sync.Mutex
		allowed := 0
		for i := range 7 { // exactly the allowance + 2 blocks
			wg.Go(func() {
				d, err := qs[i%2].Begin(context.Background(), appA)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				if d.Allowed {
					allowed++
				}
				mu.Unlock()
			})
		}
		wg.Wait()
		wl := w.get("alice")
		if allowed != 7 || wl.Overflow.Blocks[coins.KindRuns] != 2 || w.overflowRows("alice") != 2 || wl.Purchased != 90 {
			t.Fatalf("allowed %d, wallet %+v, rows %d", allowed, wl, w.overflowRows("alice"))
		}
	})
}

// Writes / reads / recipients past their allowance: all of the run's or none.
func TestAllowanceWritesReadsRecipients(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		ctx := context.Background()
		c := jstClock()
		w := newWallet(t, c)
		w.fund("alice", 100, true, 100)
		q := allowanceQuota(st, env, c, coins.ModeEnforce, w)

		d := begin(t, q, appA)
		g, err := q.Writes(ctx, d, 4)
		if err != nil || g.Allowed != 4 {
			t.Fatalf("writes %+v %v", g, err)
		}
		if ns := d.Notices(); len(ns) != 1 || ns[0] != (Notice{Kind: coins.KindWrites, Level: WarnNear}) {
			t.Fatalf("80%% of writes: %+v", ns)
		}
		g, err = q.Writes(ctx, d, 1) // 5 > 4 and writes have no block
		if err != nil || g.Allowed != 0 || g.Dropped != 1 || g.Block == nil || g.Block.Stop != OutcomeSkippedAllowance || !g.Charge.Empty() {
			t.Fatalf("over %+v %v", g, err)
		}

		ch, blk, err := q.Reads(ctx, d, 10)
		if err != nil || blk != nil || ch.Empty() {
			t.Fatalf("reads %+v %+v %v", ch, blk, err)
		}
		if _, blk, err = q.Reads(ctx, d, 1); err != nil || blk == nil || blk.Stop != OutcomeSkippedAllowance {
			t.Fatalf("reads over %+v %v", blk, err)
		}

		rg, err := q.Recipients(ctx, d, []string{"x", "y", "z"})
		if err != nil || len(rg.Allowed) != 0 || len(rg.Dropped) != 3 || rg.Dropped[0].Block.Stop != OutcomeSkippedAllowance {
			t.Fatalf("recipients over %+v %v", rg, err)
		}
		if rg, err = q.Recipients(ctx, d, []string{"x", "y"}); err != nil || len(rg.Allowed) != 2 {
			t.Fatalf("recipients %+v %v", rg, err)
		}
	})
}

// Each notice is claimed once per creator, kind, level and JST month, across
// instances; the next month claims again.
func TestClaimNoticeOncePerMonth(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		ctx := context.Background()
		c := jstClock()
		if st == nil {
			st = ratelimit.NewMemoryClock(c.now)
		}
		q1 := allowanceQuota(st, env, c, coins.ModeLog, nil)
		q2 := allowanceQuota(st, env, c, coins.ModeLog, nil)
		near := Notice{Kind: coins.KindRuns, Level: WarnNear}
		claim := func(q *Quota, n Notice) bool {
			t.Helper()
			ok, err := q.ClaimNotice(ctx, begin(t, q, appA), n)
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}
		if !claim(q1, near) || claim(q1, near) || claim(q2, near) {
			t.Fatal("near claimed more than once")
		}
		if !claim(q2, Notice{Kind: coins.KindRuns, Level: WarnFull}) || !claim(q1, Notice{Kind: NoticeCap, Level: WarnNear}) {
			t.Fatal("other levels / kinds are separate")
		}
		if claim(q1, Notice{Kind: coins.KindRuns, Level: 50}) {
			t.Fatal("unknown level claimed")
		}
		c.t = c.t.AddDate(0, 1, 0)
		if !claim(q1, near) {
			t.Fatal("next month not claimed")
		}
	})
}
