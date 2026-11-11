package runner_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/status"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// recordingBuyer is a coins.Service on memory that records who paid.
type recordingBuyer struct {
	*coins.Service
	mu   sync.Mutex
	uids []string
}

func newBuyer(block coins.Block) *recordingBuyer {
	cfg := coins.Config{Mode: coins.ModeEnforce, Overflow: map[string]coins.Block{coins.KindRuns: block}}
	return &recordingBuyer{Service: &coins.Service{Config: cfg, Store: coins.NewMemory()}}
}

func (b *recordingBuyer) BuyOverflow(ctx context.Context, r coins.OverflowRequest) (coins.OverflowResult, error) {
	b.mu.Lock()
	b.uids = append(b.uids, r.UID)
	b.mu.Unlock()
	return b.Service.BuyOverflow(ctx, r)
}

func (b *recordingBuyer) fund(t *testing.T, uid string, n int64, enabled bool, monthlyCap int64) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := coins.CreditPurchase(ctx, b.Store, uid, coins.Purchase{TransactionID: "fund-" + uid, ProductID: "p", Coins: n}, coins.GrantPolicy{Anonymous: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SetOverflow(ctx, uid, true, coins.OverflowSettings{Enabled: enabled, MonthlyCap: monthlyCap}); err != nil {
		t.Fatal(err)
	}
}

func allowanceOpts(runs, writes int64, buyer *recordingBuyer) options {
	return options{
		allowance: quota.AllowanceConfig{Mode: coins.ModeEnforce,
			Limits: map[string]int64{coins.KindRuns: runs, coins.KindWrites: writes},
			Blocks: buyer.Config.Overflow},
		buyer: buyer,
	}
}

func (h *harness) ruleResult(uid, app string) string {
	h.t.Helper()
	code, out := h.do(uid, http.MethodGet, app+"/automations", "", nil)
	if code != http.StatusOK {
		h.t.Fatalf("automations: %d %v", code, out)
	}
	return out["rules"].([]any)[0].(map[string]any)["result"].(string)
}

func lastOutcome(h *harness, id string) string {
	evs := h.events(id)
	var last store.OutboxEvent
	for _, ev := range evs {
		if ev.CreatedAt.After(last.CreatedAt) || last.EventID == "" {
			last = ev
		}
	}
	return last.Outcome
}

// #630 part 3: overflow off, the creator's monthly runs stop at the
// allowance; the creator is told at 80% and when it stops, once each.
func TestRunnerAllowanceStops(t *testing.T) {
	b := newBuyer(coins.Block{Size: 1, Coins: 2})
	stores(t, allowanceOpts(2, 0, b), func(t *testing.T, h *harness) {
		id, uid := "RUN-ALW"+h.sfx, "uid-alw"+h.sfx
		h.tokens[uid] = []string{"tok-alw"}
		b.fund(t, uid, 100, false, 100)
		app, _, _ := h.privateTodo(id, uid)
		h.tick()
		for range 3 {
			h.post(uid, app+"/entities/task/records", `{"data":{"title":"次"}}`)
			h.tick()
		}
		if o := lastOutcome(h, id); o != quota.OutcomeSkippedAllowance {
			t.Fatalf("outcome %q", o)
		}
		if r := h.ruleResult(uid, app); r != status.ResultAllowanceExhausted {
			t.Fatalf("result %q", r)
		}
		ws := h.warnPushes()
		if len(ws) != 2 || !strings.Contains(ws[0].Body, "80%") || !strings.Contains(ws[1].Body, "来月まで止まります") {
			t.Fatalf("pushes %+v", ws)
		}
		if out := h.logs.String(); strings.Count(out, `"event":"runner.allowance_warn"`) != 2 || strings.Contains(out, uid) {
			t.Fatalf("logs %s", out)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 2 {
			t.Fatalf("logs written: %d (2 runs)", n)
		}
	})
}

// Overflow on: a block is bought past the allowance and the run goes on;
// at the owner's cap the runs stop with overflow_cap_reached.
func TestRunnerAllowanceOverflowThenCap(t *testing.T) {
	b := newBuyer(coins.Block{Size: 1, Coins: 2})
	stores(t, allowanceOpts(1, 0, b), func(t *testing.T, h *harness) {
		id, uid := "RUN-OVF"+h.sfx, "uid-ovf"+h.sfx
		h.tokens[uid] = []string{"tok-ovf"}
		b.fund(t, uid, 10, true, 3)
		app, _, _ := h.privateTodo(id, uid)
		h.tick() // run 1: the allowance
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"二"}}`)
		h.tick() // run 2: one block (2 coins)
		if o := lastOutcome(h, id); o != quota.OutcomeOK {
			t.Fatalf("run 2: %q", o)
		}
		h.post(uid, app+"/entities/task/records", `{"data":{"title":"三"}}`)
		h.tick() // run 3: a second block passes the cap (4 > 3)
		if o := lastOutcome(h, id); o != quota.OutcomeSkippedOverflowCap {
			t.Fatalf("run 3: %q", o)
		}
		if r := h.ruleResult(uid, app); r != status.ResultOverflowCapReached {
			t.Fatalf("result %q", r)
		}
		w, err := coins.GetWallet(t.Context(), b.Store, uid, coins.GrantPolicy{Anonymous: true}, time.Now())
		if err != nil || w.Purchased != 8 || w.Overflow.Spent != 2 {
			t.Fatalf("wallet %+v %v", w, err)
		}
		var bodies []string
		for _, p := range h.warnPushes() {
			bodies = append(bodies, p.Body)
		}
		all := strings.Join(bodies, "|")
		if len(bodies) != 3 || !strings.Contains(all, "80%") || !strings.Contains(all, "コインで払っています") || !strings.Contains(all, "今月の上限に達しました") {
			t.Fatalf("pushes %v", bodies)
		}
	})
}

// The writes allowance cuts the run's writes (all of them) but the run is
// ok: the creator sees allowance_exhausted.
func TestRunnerAllowanceCutsWrites(t *testing.T) {
	b := newBuyer(coins.Block{})
	stores(t, allowanceOpts(100, 1, b), func(t *testing.T, h *harness) {
		id, uid := "RUN-ALWW"+h.sfx, "uid-alww"+h.sfx
		h.tokens[uid] = []string{"tok-alww"}
		app, _, _ := h.privateTodo(id, uid)
		h.tick() // on_task writes a log and increments the project: 2 > 1
		if o := lastOutcome(h, id); o != quota.OutcomeOK {
			t.Fatalf("outcome %q", o)
		}
		if n := len(h.records(store.PrivateRecords(uid, id, "log"))); n != 0 {
			t.Fatalf("logs written: %d", n)
		}
		if r := h.ruleResult(uid, app); r != status.ResultAllowanceExhausted {
			t.Fatalf("result %q", r)
		}
	})
}

// Shared apps: a member's run is counted and paid by the creator.
func TestRunnerAllowanceSharedChargesCreator(t *testing.T) {
	b := newBuyer(coins.Block{Size: 1, Coins: 1})
	stores(t, allowanceOpts(1, 0, b), func(t *testing.T, h *harness) {
		id := "RUN-ALWS" + h.sfx
		owner, mom := "uid-alws-owner"+h.sfx, "uid-alws-mom"+h.sfx
		b.mu.Lock()
		b.uids = nil // the buyer is shared by both subtests
		b.mu.Unlock()
		b.fund(t, owner, 10, true, 10)
		b.fund(t, mom, 10, true, 10)
		app := h.circle(id, owner, circleRules(t), mom)
		event := h.post(owner, app+"/entities/events/records", `{"data":{"title":"練習","status":"予定"}}`)
		h.post(mom, app+"/entities/rsvps/records", `{"data":{"event":"`+event+`","answer":"yes"}}`)
		h.post(mom, app+"/entities/rsvps/records", `{"data":{"event":"`+event+`","answer":"no"}}`)
		h.tick()
		b.mu.Lock()
		uids := append([]string(nil), b.uids...)
		b.mu.Unlock()
		if len(uids) == 0 {
			t.Fatal("no block was needed")
		}
		for _, u := range uids {
			if u != owner {
				t.Fatalf("paid by %q, want the creator", u)
			}
		}
		w, err := coins.GetWallet(t.Context(), b.Store, mom, coins.GrantPolicy{Anonymous: true}, time.Now())
		if err != nil || w.Purchased != 10 {
			t.Fatalf("the member paid: %+v %v", w, err)
		}
	})
}
