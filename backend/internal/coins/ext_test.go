package coins

import (
	"context"
	"errors"
	"testing"
)

// OpExt (internal/extapi) holds the caller's Max, writes spend_ext with the
// USD and gives the rest back.
func TestOpExt(t *testing.T) {
	ctx, c := context.Background(), &clock{t: t0}
	st := NewMemory()
	s := newService(st, ModeEnforce, c)
	s.Config.PerUSD = 100
	fund(t, st, "u", 100, c.now())
	if _, err := s.Reserve(ctx, ReserveRequest{UID: "u", Op: OpExt, RequestID: "r0"}); !errors.Is(err, ErrUnknownOp) {
		t.Fatalf("ext without Max: %v", err)
	}
	h, err := s.Reserve(ctx, ReserveRequest{UID: "u", Op: OpExt, Kind: "fakePaid", RequestID: "r1", Max: 30})
	if err != nil || h.Amount != 30 {
		t.Fatalf("reserve %+v %v", h, err)
	}
	ch, err := s.Commit(ctx, h, Usage{USD: 0.25, Priced: true})
	if err != nil || ch.Coins != 25 || ch.Balance != 75 {
		t.Fatalf("commit %+v %v", ch, err)
	}
	es, _, _ := st.Ledger(ctx, "u", 10, "")
	var ext []Entry
	for _, e := range es {
		if e.Kind == KindSpendAI {
			t.Fatal("spend_ai for an ext hold")
		}
		if e.Kind == KindSpendExt {
			ext = append(ext, e)
		}
	}
	if len(ext) != 1 || ext[0].Amount != -25 || ext[0].USD != 0.25 || ext[0].Op != OpExt {
		t.Fatalf("spend_ext %+v", ext)
	}
}
