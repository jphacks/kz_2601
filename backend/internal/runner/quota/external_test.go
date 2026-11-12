package quota

import (
	"context"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

func TestExtPro(t *testing.T) {
	ctx := context.Background()
	t.Run("decision decides", func(t *testing.T) {
		q := New(Config{}, nil, nil) // nobody is Pro: the Decision is not checked again
		cases := []struct {
			name string
			dec  Decision
			want bool
		}{
			{"allowed pro", Decision{Allowed: true, Plan: PlanPro}, true},
			{"allowed free", Decision{Allowed: true, Plan: PlanFree}, false},
			{"blocked pro", Decision{Allowed: false, Plan: PlanPro}, false},
			{"none", Decision{Allowed: false, Plan: PlanNone}, false},
		}
		for _, c := range cases {
			if got := q.ExtPro(ctx, appA, &c.dec); got != c.want {
				t.Errorf("%s: ExtPro = %v, want %v", c.name, got, c.want)
			}
		}
	})
	t.Run("no decision checks the creator", func(t *testing.T) {
		pro := proSet{"alice": true}
		cases := []struct {
			name string
			pro  ProChecker
			r    Run
			want bool
		}{
			{"pro", pro, Run{AppID: "a", CreatorUID: "alice"}, true},
			{"not pro", pro, Run{AppID: "a", CreatorUID: "carol"}, false},
			{"anonymous", everyonePro{}, Run{AppID: "a", CreatorUID: "alice", CreatorAnonymous: true}, false},
			{"no creator", everyonePro{}, Run{AppID: "a"}, false},
			{"no checker", nil, Run{AppID: "a", CreatorUID: "alice"}, false},
		}
		for _, c := range cases {
			q := New(Config{}, nil, c.pro)
			if got := q.ExtPro(ctx, c.r, nil); got != c.want {
				t.Errorf("%s: ExtPro = %v, want %v", c.name, got, c.want)
			}
		}
	})
}

// extQuota counts the monthly runs (3 a month, nothing to buy) so a call is visible.
func extQuota(t *testing.T, pro ProChecker) *Quota {
	t.Helper()
	c := jstClock()
	a := testAllowance
	a.Mode = coins.ModeEnforce
	return New(Config{Env: "test", Now: c.now, Allowance: a}, ratelimit.NewMemoryClock(c.now), pro)
}

func TestExtRun(t *testing.T) {
	ctx := context.Background()
	t.Run("not allowed decision blocks and counts nothing", func(t *testing.T) {
		q := extQuota(t, everyonePro{})
		want := &Block{Scope: ScopeAppExec, Limit: 5, Used: 5}
		for _, dec := range []Decision{
			{Allowed: false, Plan: PlanPro, Block: want},
			{Allowed: false, Plan: PlanPro},
		} {
			ch, b, err := q.ExtRun(ctx, appA, &dec)
			if err != nil || b == nil || !ch.Empty() {
				t.Fatalf("ExtRun(%+v) = %+v %+v %v", dec, ch, b, err)
			}
			if dec.Block != nil && *b != *want {
				t.Fatalf("block = %+v, want %+v", *b, *want)
			}
			if dec.Block == nil && *b != (Block{}) {
				t.Fatalf("block without one in the decision = %+v", *b)
			}
		}
	})
	t.Run("allowed but not pro needs pro", func(t *testing.T) {
		q := extQuota(t, everyonePro{})
		dec := Decision{Allowed: true, Plan: PlanFree}
		ch, b, err := q.ExtRun(ctx, appA, &dec)
		if err != nil || b == nil || b.Scope != ScopeExtNeedsPro || !ch.Empty() {
			t.Fatalf("ExtRun = %+v %+v %v", ch, b, err)
		}
	})
	t.Run("pressed by a person without pro", func(t *testing.T) {
		q := extQuota(t, proSet{})
		ch, b, err := q.ExtRun(ctx, appA, nil)
		if err != nil || b == nil || b.Scope != ScopeExtNeedsPro || !ch.Empty() {
			t.Fatalf("ExtRun = %+v %+v %v", ch, b, err)
		}
		r := appA
		r.CreatorAnonymous = true
		if _, b, _ := extQuota(t, everyonePro{}).ExtRun(ctx, r, nil); b == nil || b.Scope != ScopeExtNeedsPro {
			t.Fatalf("anonymous creator: block = %+v", b)
		}
	})
	t.Run("pro counts one run of the month", func(t *testing.T) {
		q := extQuota(t, everyonePro{})
		r := appA
		r.Attempt = 3 // a pressed call is always a first attempt
		var charges []Charge
		for i := range 3 {
			ch, b, err := q.ExtRun(ctx, r, nil)
			if err != nil || b != nil {
				t.Fatalf("call %d: %+v %v", i, b, err)
			}
			if len(ch.Parts) != 1 || ch.Parts[0].Weight != 1 || ch.Parts[0].Counters[0].Scope != coins.AllowanceScope(coins.KindRuns) {
				t.Fatalf("call %d: charge = %+v", i, ch)
			}
			charges = append(charges, ch)
		}
		_, b, err := q.ExtRun(ctx, r, nil)
		if err != nil || b == nil || b.Stop != OutcomeSkippedAllowance || b.Used != 3 {
			t.Fatalf("4th call: %+v %v", b, err)
		}
		// Refund gives the call back: one more fits.
		if err := q.Refund(ctx, charges[0]); err != nil {
			t.Fatal(err)
		}
		if _, b, err := q.ExtRun(ctx, r, nil); err != nil || b != nil {
			t.Fatalf("after refund: %+v %v", b, err)
		}
		if _, b, _ := q.ExtRun(ctx, r, nil); b == nil {
			t.Fatal("after refund only one call fits")
		}
	})
	t.Run("server rule run shares the month with its decision", func(t *testing.T) {
		q := extQuota(t, everyonePro{})
		dec := begin(t, q, appA) // the run itself: 1 of 3
		if !dec.Allowed || dec.Plan != PlanPro {
			t.Fatalf("begin = %+v", dec)
		}
		for i := range 2 {
			if _, b, err := q.ExtRun(ctx, appA, &dec); err != nil || b != nil {
				t.Fatalf("call %d: %+v %v", i, b, err)
			}
		}
		if _, b, _ := q.ExtRun(ctx, appA, &dec); b == nil || b.Stop != OutcomeSkippedAllowance {
			t.Fatalf("over the month: %+v", b)
		}
	})
}
