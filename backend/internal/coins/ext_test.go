package coins

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	h, err := s.Reserve(ctx, ReserveRequest{UID: "u", Op: OpExt, Kind: "fakePaid", Provider: "fakePaid", RequestID: "r1", Max: 30})
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
	if len(ext) != 1 || ext[0].Amount != -25 || ext[0].USD != 0.25 || ext[0].Op != OpExt || ext[0].Provider != "fakePaid" {
		t.Fatalf("spend_ext %+v", ext)
	}
	if b, _ := json.Marshal(ext[0]); !strings.Contains(string(b), `"provider":"fakePaid"`) {
		t.Fatalf("ledger JSON: %s", b)
	}
}

// #2145: only an ext hold keeps a provider; a row written before it (no
// provider) still reads and answers without the field.
func TestSpendExtProvider(t *testing.T) {
	ctx, c := context.Background(), &clock{t: t0}
	st := NewMemory()
	s := newService(st, ModeEnforce, c)
	s.Config.PerUSD = 100
	fund(t, st, "u", 100, c.now())
	h, err := s.Reserve(ctx, ReserveRequest{UID: "u", Op: OpGenerate, Provider: "fakePaid", RequestID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if h.Provider != "" {
		t.Fatalf("a non-ext hold kept a provider: %+v", h)
	}
	if err := st.Update(ctx, Keys{UID: "u"}, func(tx *Tx) error {
		tx.Entries = []Entry{{ID: "spend_old", Kind: KindSpendExt, Amount: -5, Op: OpExt, At: c.now()}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	es, _, err := st.Ledger(ctx, "u", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if e.ID != "spend_old" {
			continue
		}
		if b, _ := json.Marshal(e); e.Provider != "" || strings.Contains(string(b), "provider") || !strings.Contains(string(b), `"kind":"spend_ext"`) {
			t.Fatalf("old row: %+v %s", e, b)
		}
		return
	}
	t.Fatalf("old row missing: %+v", es)
}
