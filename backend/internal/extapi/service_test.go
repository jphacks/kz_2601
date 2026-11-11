package extapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
)

const (
	creator = "creator-uid"
	user    = "user-uid"
	appID   = "app-1"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type proSet map[string]bool

func (p proSet) IsPro(_ context.Context, uid string) bool { return p[uid] }

// env is a Service on memory: quota (runs) and coins as in the runner.
type env struct {
	t     *testing.T
	c     *clock
	rl    *ratelimit.Memory
	q     *quota.Quota
	coins *coins.Service
	mem   *Memory
	svc   *Service
	logs  *bytes.Buffer
	reqN  atomic.Int64
}

type opts struct {
	mode      coins.Mode
	runs      int64 // ALLOWANCE_PRO_RUNS (0 = counted, not enforced)
	pro       proSet
	cfg       func(*Config)
	runsBlock coins.Block
}

func newEnv(t *testing.T, o opts) *env {
	t.Helper()
	if o.mode == "" {
		o.mode = coins.ModeEnforce
	}
	if o.pro == nil {
		o.pro = proSet{creator: true}
	}
	c := &clock{t: time.Date(2026, 11, 11, 12, 0, 0, 0, ratelimit.JST)}
	rl := ratelimit.NewMemoryClock(c.now)
	blocks := map[string]coins.Block{}
	if o.runsBlock.On() {
		blocks[coins.KindRuns] = o.runsBlock
	}
	q := quota.New(quota.Config{Env: "test", Now: c.now, Allowance: quota.AllowanceConfig{Mode: o.mode,
		Limits: map[string]int64{coins.KindRuns: o.runs}, Blocks: blocks}}, rl, o.pro)
	cs := &coins.Service{Config: coins.Config{Mode: o.mode, PerUSD: 100, MinPerCall: 1, HoldTTL: 10 * time.Minute, Overflow: blocks},
		Store: coins.NewMemory(), Now: c.now}
	q.Buyer = cs
	var logs bytes.Buffer
	lg := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	q.Logger, cs.Log = lg, lg
	mem := NewMemory()
	sealer, err := NewLocalSealer(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Enabled:      map[string]bool{FakeFree: true, FakePaid: true, FakeOAuth: true},
		Reserve:      map[string]int64{FakePaid: 50},
		CostUSD:      map[string]float64{},
		Timeout:      200 * time.Millisecond,
		CallbackBase: "https://runner.example",
		AppReturnURL: "tailorit://connected",
	}
	if o.cfg != nil {
		o.cfg(&cfg)
	}
	conns := &Connections{Store: mem, Sealer: sealer, Now: c.now, Log: lg}
	svc := New(cfg, q, cs, conns, mem, lg)
	svc.Now = c.now
	return &env{t: t, c: c, rl: rl, q: q, coins: cs, mem: mem, svc: svc, logs: &logs}
}

func (e *env) req(provider, op string, args map[string]string) CallRequest {
	return CallRequest{Provider: provider, Op: op, Args: args, AppID: appID, CreatorUID: creator, UserUID: user,
		RequestID: "req-" + strconv.FormatInt(e.reqN.Add(1), 10)}
}

func (e *env) call(r CallRequest) (CallResult, error) {
	return e.svc.Call(context.Background(), r)
}

// runs is the creator's `runs` this month.
func (e *env) runs() int {
	e.t.Helper()
	id := coins.AllowanceCounterID("test", coins.MonthKey(e.c.now()), coins.KindRuns, creator)
	n, err := e.rl.Peek(context.Background(), []ratelimit.Counter{{ID: id}})
	if err != nil {
		e.t.Fatal(err)
	}
	return n[0]
}

func (e *env) fund(uid string, n int64) {
	e.t.Helper()
	if _, _, err := coins.CreditPurchase(context.Background(), e.coins.Store, uid,
		coins.Purchase{TransactionID: "fund-" + uid, ProductID: "p", Coins: n}, coins.GrantPolicy{Anonymous: true}, e.c.now()); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) wallet(uid string) coins.Wallet {
	e.t.Helper()
	w, err := coins.GetWallet(context.Background(), e.coins.Store, uid, coins.GrantPolicy{Anonymous: true}, e.c.now())
	if err != nil {
		e.t.Fatal(err)
	}
	return w
}

func (e *env) ledger(uid string) []coins.Entry {
	e.t.Helper()
	es, _, err := e.coins.Store.Ledger(context.Background(), uid, 100, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return es
}

func spends(es []coins.Entry, kind string) []coins.Entry {
	var out []coins.Entry
	for _, x := range es {
		if x.Kind == kind {
			out = append(out, x)
		}
	}
	return out
}

// A free provider: one call = one `runs` of the creator; no coins.
func TestFreeCountsOneRun(t *testing.T) {
	e := newEnv(t, opts{})
	e.fund(user, 100)
	for i := 1; i <= 3; i++ {
		res, err := e.call(e.req(FakeFree, "listItems", map[string]string{"q": "x"}))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Output.Rows) != 2 || res.Charge.Coins != 0 {
			t.Fatalf("result %+v", res)
		}
		if got := e.runs(); got != i {
			t.Fatalf("runs = %d, want %d", got, i)
		}
	}
	if w := e.wallet(user); w.Available() != 100 || len(e.ledger(user)) != 1 {
		t.Fatalf("free call touched coins: %+v %+v", w, e.ledger(user))
	}
}

// A paid provider: hold → commit cost × COIN_PER_USD → the rest back;
// spend_ext (not spend_ai) with the USD. It is one `runs` too (決定 3).
func TestPaidReserveCommitRelease(t *testing.T) {
	e := newEnv(t, opts{})
	e.fund(user, 100)
	res, err := e.call(e.req(FakePaid, "echo", map[string]string{"text": "hi", "costUsd": "0.12"}))
	if err != nil {
		t.Fatal(err)
	}
	// ceil(0.12 × 100) = 12 of the 50 held
	if res.Charge.Coins != 12 || res.Charge.Balance != 88 {
		t.Fatalf("charge %+v", res.Charge)
	}
	w := e.wallet(user)
	if w.Available() != 88 || w.Reserved != 0 || len(w.Holds) != 0 {
		t.Fatalf("wallet %+v", w)
	}
	es := e.ledger(user)
	ext := spends(es, coins.KindSpendExt)
	if len(ext) != 1 || ext[0].Amount != -12 || ext[0].Op != coins.OpExt || ext[0].USD != 0.12 || ext[0].AppID != appID {
		t.Fatalf("spend_ext rows %+v", ext)
	}
	if len(spends(es, coins.KindSpendAI)) != 0 {
		t.Fatal("a spend_ai row was written")
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d", e.runs())
	}
	// the creator pays the runs, the person who pressed pays the coins
	if e.wallet(creator).Total() != 0 {
		t.Fatal("creator's coins were used")
	}
}

// No measured cost: EXT_COST_USD_<PROVIDER>_<OP>, else the hold is charged.
func TestPaidUnpriced(t *testing.T) {
	e := newEnv(t, opts{cfg: func(c *Config) { c.CostUSD[FakePaid+".echo"] = 0.03 }})
	e.fund(user, 200)
	res, err := e.call(e.req(FakePaid, "echo", map[string]string{"text": "a"}))
	if err != nil || res.Charge.Coins != 3 {
		t.Fatalf("configured cost: %+v %v", res, err)
	}
	e2 := newEnv(t, opts{})
	e2.fund(user, 200)
	res, err = e2.call(e2.req(FakePaid, "echo", map[string]string{"text": "a"}))
	if err != nil || res.Charge.Coins != 50 {
		t.Fatalf("unpriced: hold charged? %+v %v", res, err)
	}
}

// A failed call gives the run and the coins back.
func TestFailureRefundsRunsAndCoins(t *testing.T) {
	for _, fail := range []string{"5xx", "timeout"} {
		t.Run(fail, func(t *testing.T) {
			e := newEnv(t, opts{})
			e.fund(user, 100)
			_, err := e.call(e.req(FakePaid, "echo", map[string]string{"text": "a", "costUsd": "0.1", "fail": fail}))
			want := ReasonUpstream
			if fail == "timeout" {
				want = ReasonUpstreamTimeout
			}
			if Reason(err) != want {
				t.Fatalf("reason %q (%v), want %q", Reason(err), err, want)
			}
			if e.runs() != 0 {
				t.Fatalf("runs = %d after a failure", e.runs())
			}
			w := e.wallet(user)
			if w.Available() != 100 || w.Reserved != 0 || len(spends(e.ledger(user), coins.KindSpendExt)) != 0 {
				t.Fatalf("coins not given back: %+v", w)
			}
		})
	}
	t.Run("free", func(t *testing.T) {
		e := newEnv(t, opts{})
		if _, err := e.call(e.req(FakeFree, "listItems", map[string]string{"fail": "5xx"})); err == nil || e.runs() != 0 {
			t.Fatalf("err %v runs %d", err, e.runs())
		}
	})
}

// A 4xx the provider charged is committed, not given back.
func TestBillableFailureCommits(t *testing.T) {
	e := newEnv(t, opts{cfg: func(c *Config) { c.CostUSD[FakePaid+".echo"] = 0.05 }})
	e.fund(user, 100)
	res, err := e.call(e.req(FakePaid, "echo", map[string]string{"fail": "4xx"}))
	if Reason(err) != ReasonUpstream || res.Charge.Coins != 5 || e.runs() != 1 {
		t.Fatalf("billable 4xx: %+v %v runs=%d", res, err, e.runs())
	}
}

// Pro only (決定 1): nothing counted for a creator without Pro.
func TestProOnly(t *testing.T) {
	e := newEnv(t, opts{pro: proSet{}})
	e.fund(user, 100)
	_, err := e.call(e.req(FakePaid, "echo", map[string]string{"costUsd": "0.1"}))
	if !errors.Is(err, ErrProRequired) || StatusOf(Reason(err)) != 402 {
		t.Fatalf("err %v", err)
	}
	if e.runs() != 0 || e.wallet(user).Available() != 100 {
		t.Fatal("counted without Pro")
	}
	// PRO_GATE_MODE does not matter: the user's Pro does not help either
	e2 := newEnv(t, opts{pro: proSet{user: true}})
	if _, err := e2.call(e2.req(FakeFree, "listItems", nil)); !errors.Is(err, ErrProRequired) {
		t.Fatalf("creator not Pro: %v", err)
	}
}

// Past the monthly runs: stop (overflow off), or buy a block (overflow on).
func TestAllowance(t *testing.T) {
	e := newEnv(t, opts{runs: 2})
	for range 2 {
		if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := e.call(e.req(FakeFree, "listItems", nil))
	if Reason(err) != ReasonAllowanceExhausted || e.runs() != 2 {
		t.Fatalf("over the allowance: %v runs=%d", err, e.runs())
	}

	e = newEnv(t, opts{runs: 1, runsBlock: coins.Block{Size: 5, Coins: 7}})
	e.fund(creator, 20)
	if _, err := e.coins.SetOverflow(context.Background(), creator, true, coins.OverflowSettings{Enabled: true, MonthlyCap: 100}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := e.call(e.req(FakeFree, "listItems", nil)); err != nil {
			t.Fatal(err)
		}
	}
	if e.runs() != 3 || len(spends(e.ledger(creator), coins.KindSpendOverflow)) != 1 {
		t.Fatalf("overflow: runs=%d ledger=%+v", e.runs(), e.ledger(creator))
	}
}

// The paid hold is the person's: too few coins is 402, and the run is given back.
func TestPaidInsufficient(t *testing.T) {
	e := newEnv(t, opts{})
	e.fund(user, 10)
	_, err := e.call(e.req(FakePaid, "echo", map[string]string{"costUsd": "0.01"}))
	if Reason(err) != ReasonCoinsInsufficient || e.runs() != 0 {
		t.Fatalf("err %v runs %d", err, e.runs())
	}
}

// COIN_BILLING_MODE=off counts nothing and takes nothing; log only logs.
func TestBillingModes(t *testing.T) {
	e := newEnv(t, opts{mode: coins.ModeOff, runs: 1})
	for range 3 {
		if res, err := e.call(e.req(FakePaid, "echo", map[string]string{"costUsd": "0.5"})); err != nil || res.Charge.Coins != 0 {
			t.Fatalf("off: %+v %v", res, err)
		}
	}
	if e.runs() != 0 {
		t.Fatalf("off counted runs %d", e.runs())
	}
	e = newEnv(t, opts{mode: coins.ModeLog})
	if _, err := e.call(e.req(FakePaid, "echo", map[string]string{"costUsd": "0.5"})); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(e.logs.Bytes(), []byte(coins.EventWouldSpend)) || e.wallet(user).Total() != 0 {
		t.Fatalf("log mode: %s", e.logs.String())
	}
}

// Kill switches and checks: EXT_DISABLED, off by default, fakes never in
// production, no hold configured under enforce, unknown args and URLs.
func TestUnavailableAndBadRequests(t *testing.T) {
	cases := []struct {
		name string
		cfg  func(*Config)
		req  func(e *env) CallRequest
		want string
	}{
		{"disabled", func(c *Config) { c.Disabled = true }, func(e *env) CallRequest { return e.req(FakeFree, "listItems", nil) }, ReasonUnavailable},
		{"not enabled", func(c *Config) { c.Enabled = map[string]bool{} }, func(e *env) CallRequest { return e.req(FakeFree, "listItems", nil) }, ReasonUnavailable},
		{"fake in production", func(c *Config) { c.Production = true }, func(e *env) CallRequest { return e.req(FakeFree, "listItems", nil) }, ReasonUnavailable},
		{"paid without a hold", func(c *Config) { c.Reserve = map[string]int64{} }, func(e *env) CallRequest { return e.req(FakePaid, "echo", nil) }, ReasonUnavailable},
		{"unknown provider", nil, func(e *env) CallRequest { return e.req("nope", "listItems", nil) }, ReasonBadRequest},
		{"unknown op", nil, func(e *env) CallRequest { return e.req(FakeFree, "nope", nil) }, ReasonBadRequest},
		{"unknown arg", nil, func(e *env) CallRequest { return e.req(FakeFree, "listItems", map[string]string{"url": "x"}) }, ReasonBadRequest},
		{"url arg", nil, func(e *env) CallRequest {
			return e.req(FakeFree, "listItems", map[string]string{"q": "https://169.254.169.254/"})
		}, ReasonBadRequest},
		{"no request id", nil, func(e *env) CallRequest { r := e.req(FakeFree, "listItems", nil); r.RequestID = ""; return r }, ReasonBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, opts{cfg: tc.cfg})
			e.fund(user, 100)
			_, err := e.call(tc.req(e))
			if Reason(err) != tc.want {
				t.Fatalf("reason %q (%v), want %q", Reason(err), err, tc.want)
			}
			if e.runs() != 0 || e.wallet(user).Available() != 100 {
				t.Fatal("counted")
			}
		})
	}
}

// A full instance answers ext_busy at once.
func TestBusy(t *testing.T) {
	e := newEnv(t, opts{cfg: func(c *Config) { c.MaxConcurrent = 1 }})
	e.svc.sem <- struct{}{}
	_, err := e.call(e.req(FakeFree, "listItems", nil))
	<-e.svc.sem
	if Reason(err) != ReasonBusy || StatusOf(ReasonBusy) != 503 {
		t.Fatalf("err %v", err)
	}
}

// A call inside a server rule run counts one more `runs` on the run's
// Decision and uses the creator's coins and token.
func TestCallInsideRun(t *testing.T) {
	e := newEnv(t, opts{})
	e.fund(creator, 100)
	dec, err := e.q.Begin(context.Background(), quota.Run{AppID: appID, CreatorUID: creator, Attempt: 1})
	if err != nil || !dec.Allowed {
		t.Fatalf("begin %+v %v", dec, err)
	}
	r := e.req(FakePaid, "echo", map[string]string{"costUsd": "0.02"})
	r.UserUID, r.Run = "", &dec
	res, err := e.call(r)
	if err != nil || res.Charge.Coins != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if e.runs() != 2 || e.wallet(creator).Available() != 98 {
		t.Fatalf("runs %d wallet %+v", e.runs(), e.wallet(creator))
	}
}
