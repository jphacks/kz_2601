package extapi

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/coins"
)

// countingPaid replaces fakePaid's echo with one that counts its calls.
func countingPaid(e *env, n *atomic.Int64, cost float64) {
	e.svc.ops[FakePaid] = map[string]Operation{"echo": OperationFunc(func(context.Context, Input) (Output, error) {
		n.Add(1)
		return Output{Item: &Row{Title: "x"}, Usage: Usage{USD: cost, Priced: true}}, nil
	})}
}

// A paid call sent again with the same request id is not run again on the
// first hold: 409 duplicate_request, the second `runs` goes back, the coins
// are charged once.
func TestPaidSameRequestIDRunsOnce(t *testing.T) {
	e := newEnv(t, opts{})
	e.fund(user, 100)
	var n atomic.Int64
	countingPaid(e, &n, 0.10)
	r := e.req(FakePaid, "echo", map[string]string{"text": "a"})
	if res, err := e.call(r); err != nil || res.Charge.Coins != 10 {
		t.Fatalf("first: %+v %v", res, err)
	}
	res, err := e.call(r)
	if Reason(err) != ReasonDuplicateRequest || StatusOf(Reason(err)) != 409 {
		t.Fatalf("second: %+v %v", res, err)
	}
	if n.Load() != 1 {
		t.Fatalf("the op ran %d times", n.Load())
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d", e.runs())
	}
	w := e.wallet(user)
	if w.Available() != 90 || w.Reserved != 0 || len(spends(e.ledger(user), coins.KindSpendExt)) != 1 {
		t.Fatalf("wallet %+v ledger %+v", w, e.ledger(user))
	}
}

// ctxBiller fails a settlement on a cancelled context, as Firestore does
// (the memory store does not look at the context).
type ctxBiller struct{ *coins.Service }

func (b ctxBiller) Commit(ctx context.Context, h *coins.Hold, u coins.Usage) (coins.Charge, error) {
	if err := ctx.Err(); err != nil {
		return coins.Charge{}, err
	}
	return b.Service.Commit(ctx, h, u)
}

func (b ctxBiller) Release(ctx context.Context, h *coins.Hold) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.Service.Release(ctx, h)
}

// The caller going away after the provider answered (appconfig's wait
// ended) still settles the cost: the hold is committed, not left to expire.
func TestPaidCommitAfterCallerGone(t *testing.T) {
	e := newEnv(t, opts{})
	e.fund(user, 100)
	e.svc.Coins = ctxBiller{e.coins}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.svc.ops[FakePaid] = map[string]Operation{"echo": OperationFunc(func(context.Context, Input) (Output, error) {
		cancel() // the answer came, then the request was cancelled
		return Output{Item: &Row{Title: "x"}, Usage: Usage{USD: 0.10, Priced: true}}, nil
	})}
	res, err := e.svc.Call(ctx, e.req(FakePaid, "echo", map[string]string{"text": "a"}))
	if err != nil || res.Charge.Coins != 10 {
		t.Fatalf("%+v %v", res, err)
	}
	w := e.wallet(user)
	if w.Available() != 90 || w.Reserved != 0 || len(w.Holds) != 0 || len(spends(e.ledger(user), coins.KindSpendExt)) != 1 {
		t.Fatalf("wallet %+v", w)
	}
}

// A request id first used by a call that failed (its hold given back) does
// not make a later call with it free.
func TestPaidSameRequestIDAfterFailure(t *testing.T) {
	e := newEnv(t, opts{})
	e.fund(user, 100)
	r := e.req(FakePaid, "echo", map[string]string{"text": "a", "fail": "5xx"})
	if _, err := e.call(r); Reason(err) != ReasonUpstream {
		t.Fatalf("first: %v", err)
	}
	var n atomic.Int64
	countingPaid(e, &n, 0.10)
	r.Args = map[string]string{"text": "a"}
	if _, err := e.call(r); Reason(err) != ReasonDuplicateRequest {
		t.Fatalf("second: %v", err)
	}
	if n.Load() != 0 || e.runs() != 0 || e.wallet(user).Available() != 100 {
		t.Fatalf("ran %d runs %d wallet %+v", n.Load(), e.runs(), e.wallet(user))
	}
}
