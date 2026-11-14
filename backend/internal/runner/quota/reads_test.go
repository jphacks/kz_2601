package quota

import (
	"context"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// #340 reads: the app's daily reads are all or nothing; retries count nothing;
// the per-run cap follows the plan.
func TestReadsAllOrNothing(t *testing.T) {
	stores(t, func(t *testing.T, st ratelimit.Store, env string) {
		lim := DefaultLimits
		lim.AppReadsPro = 10
		q, _ := newQuota(st, env, lim, everyonePro{})
		ctx := context.Background()
		run := Run{AppID: "reads-app", CreatorUID: "c1", Attempt: 1}
		dec, err := q.Begin(ctx, run)
		if err != nil || !dec.Allowed {
			t.Fatalf("begin: %+v %v", dec, err)
		}
		if q.RunReadCap(dec.Plan) != DefaultLimits.RunReadsPro {
			t.Fatalf("run cap %d", q.RunReadCap(dec.Plan))
		}
		ch, blk, err := q.Reads(ctx, dec, 8)
		if err != nil || blk != nil || ch.Empty() {
			t.Fatalf("8 reads: %+v %+v %v", ch, blk, err)
		}
		_, blk, err = q.Reads(ctx, dec, 3)
		if err != nil || blk == nil || blk.Scope != ScopeAppReads || blk.Limit != 10 {
			t.Fatalf("3 more (over 10): %+v %v", blk, err)
		}
		// nothing was charged by the blocked call: 2 still fit
		if _, blk, _ := q.Reads(ctx, dec, 2); blk != nil {
			t.Fatalf("2 more: %+v", blk)
		}
		// a retry counts nothing
		retry, _ := q.Begin(ctx, Run{AppID: "reads-app", CreatorUID: "c1", Attempt: 2})
		if ch, blk, _ := q.Reads(ctx, retry, 100); blk != nil || !ch.Empty() {
			t.Fatalf("retry: %+v %+v", ch, blk)
		}
	})
}

func TestReadsCost(t *testing.T) {
	base := EstimateCostUSD(Usage{Outcome: OutcomeOK})
	if got := EstimateCostUSD(Usage{Outcome: OutcomeOK, Reads: 1000}) - base; got < 0.00039 || got > 0.00041 {
		t.Fatalf("1,000 reads cost %v", got)
	}
}
