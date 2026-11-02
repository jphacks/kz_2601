package entitlement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
)

type fakeStatusAPI struct {
	byTx  map[string][]appstore.SubscriptionStatus
	calls int
	fail  error
}

func (f *fakeStatusAPI) SubscriptionStatuses(_ context.Context, env, txid string) ([]appstore.SubscriptionStatus, error) {
	f.calls++
	if f.fail != nil {
		return nil, f.fail
	}
	s, ok := f.byTx[env+"/"+txid]
	if !ok {
		return nil, appstore.ErrNotFound
	}
	return s, nil
}

// Reconciliation picks up a refund whose notification never arrived.
func TestReconcileAppliesMissedRefund(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ne := newNotifyEnv(t, st)
		ctx := context.Background()
		refunded := ne.tx
		refunded.RevocationDate = ne.now.UnixMilli()
		refunded.SignedDate = ne.now.UnixMilli()
		api := &fakeStatusAPI{byTx: map[string][]appstore.SubscriptionStatus{
			"Sandbox/" + ne.tx.TransactionID: {
				{OriginalTransactionID: "someone-else", SignedTransactionInfo: "ignored"},
				{OriginalTransactionID: ne.otid, Status: 1, SignedTransactionInfo: ne.jws(t, refunded)},
			},
		}}
		r := &Reconciler{Verifier: ne.svc.Verifier, Store: ne.st, API: api, Now: func() time.Time { return ne.now }}

		// the emulator may hold bindings of other runs: only alice's matters
		dry, err := r.Run(ctx, 1<<20, true)
		if err != nil || dry.Updated < 1 {
			t.Fatalf("dry run: %+v %v", dry, err)
		}
		if p := ne.pro(t); p.Revoked() {
			t.Fatal("dry run wrote")
		}
		res, err := r.Run(ctx, 1<<20, false)
		if err != nil || res.Updated < 1 {
			t.Fatalf("run: %+v %v", res, err)
		}
		if p := ne.pro(t); !p.Revoked() || p.Active(ne.now, nil) {
			t.Fatalf("refund not applied: %+v", p)
		}
		// nothing new the second time
		if again, err := r.Run(ctx, 1<<20, false); err != nil || again.Updated != 0 {
			t.Fatalf("second run: %+v %v", again, err)
		}
	})
}

func TestReconcileSkipsLongExpiredAndCountsFailures(t *testing.T) {
	ne := newNotifyEnv(t, NewMemory())
	ctx := context.Background()
	api := &fakeStatusAPI{fail: errors.New("apple down")}
	r := &Reconciler{Verifier: ne.svc.Verifier, Store: ne.st, API: api, Now: func() time.Time { return ne.now }}
	res, err := r.Run(ctx, 10, false)
	if err != nil || res.Failed != 1 || res.Checked != 1 {
		t.Fatalf("failure: %+v %v", res, err)
	}
	// 2 months after the period ended: outside the window, not called
	r.Now = func() time.Time { return ne.now.AddDate(0, 3, 0) }
	api.calls = 0
	if res, err := r.Run(ctx, 10, false); err != nil || api.calls != 0 || res.Checked != 0 {
		t.Fatalf("expired long ago: %+v calls=%d %v", res, api.calls, err)
	}
	// not found in the environment: not a failure
	r.Now = func() time.Time { return ne.now }
	api.fail = nil
	if res, err := r.Run(ctx, 10, false); err != nil || res.Failed != 0 || res.Checked != 1 {
		t.Fatalf("not found: %+v %v", res, err)
	}
	// limit 0 calls nothing
	api.calls = 0
	if _, err := r.Run(ctx, 0, false); err != nil || api.calls != 0 {
		t.Fatalf("limit: calls=%d %v", api.calls, err)
	}
}

func TestBindingsPaging(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	for _, id := range []string{"c", "a", "b"} {
		if _, err := m.Apply(ctx, "u-"+id, Pro{OriginalTransactionID: id, TransactionID: id + "1"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := m.Bindings(ctx, "", 2)
	if len(got) != 2 || got[0].OriginalTransactionID != "a" || got[1].UID != "u-b" {
		t.Fatalf("page 1 %+v", got)
	}
	got, _ = m.Bindings(ctx, "b", 2)
	if len(got) != 1 || got[0].OriginalTransactionID != "c" {
		t.Fatalf("page 2 %+v", got)
	}
}
