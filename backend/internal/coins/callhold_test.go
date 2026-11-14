package coins

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
)

// readHold reads uid's hold id back from the store.
func readHold(t *testing.T, st Store, uid, id string) Hold {
	t.Helper()
	var h Hold
	if err := st.Update(context.Background(), Keys{UID: uid, HoldID: id}, func(tx *Tx) error {
		if tx.Hold != nil {
			h = *tx.Hold
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return h
}

// callHold reserves a call block of n coins for u.
func callHold(t *testing.T, s *Service, u, req string, n int64) *Hold {
	t.Helper()
	h, err := s.Reserve(context.Background(), ReserveRequest{UID: u, Op: OpCall, RequestID: req, Max: n, TTL: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestCommitCoinsCapsAtTheHold(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		charge, want int64
	}{
		"within":   {charge: 4, want: 4},
		"above":    {charge: 99, want: 10},
		"negative": {charge: -5, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			c := &clock{t: t0}
			st := NewMemory()
			s := newService(st, ModeEnforce, c)
			fund(t, st, "u", 30, c.now())
			h := callHold(t, s, "u", "b0", 10)
			got, err := s.CommitCoins(ctx, h, tc.charge)
			if err != nil {
				t.Fatal(err)
			}
			if got.Coins != tc.want || got.Balance != 30-tc.want {
				t.Fatalf("charge = %+v, want %d coins, balance %d", got, tc.want, 30-tc.want)
			}
			if h.Status != HoldCommitted || h.Charged != tc.want {
				t.Fatalf("hold = %+v", h)
			}
			// a webhook sent again commits nothing more
			again, err := s.CommitCoins(ctx, h, tc.charge)
			if err != nil || again.Balance != 30-tc.want {
				t.Fatalf("again = %+v %v", again, err)
			}
			consistent(t, st, "u", c.now())
		})
	}
}

func TestCommitCoinsNilDryRunAndExpired(t *testing.T) {
	ctx := context.Background()
	c := &clock{t: t0}
	st := NewMemory()
	s := newService(st, ModeEnforce, c)
	if got, err := s.CommitCoins(ctx, nil, 5); err != nil || got != (Charge{}) {
		t.Fatalf("nil hold: %+v %v", got, err)
	}

	dry := &Hold{UID: "u", Op: OpCall, Amount: 10, DryRun: true}
	if got, err := s.CommitCoins(ctx, dry, 5); err != nil || got != (Charge{}) {
		t.Fatalf("log mode must charge nothing: %+v %v", got, err)
	}

	fund(t, st, "u", 30, c.now())
	h := callHold(t, s, "u", "late", 10)
	c.add(10 * time.Minute) // past the 5-minute TTL
	if _, err := s.Wallet(ctx, "u", false); err != nil {
		t.Fatal(err) // returns the expired hold
	}
	got, err := s.CommitCoins(ctx, h, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got.Coins != 0 || got.Balance != 30 {
		t.Fatalf("an expired hold must not be charged: %+v", got)
	}
	if hs := readHold(t, st, "u", h.ID); hs.Status != HoldExpired || hs.Charged != 0 {
		t.Fatalf("stored hold %+v", hs)
	}
}

func TestExtendKeepsTheHold(t *testing.T) {
	ctx := context.Background()
	c := &clock{t: t0}
	st := NewMemory()
	s := newService(st, ModeEnforce, c)
	fund(t, st, "u", 30, c.now())
	h := callHold(t, s, "u", "b0", 10)
	first := readHold(t, st, "u", h.ID).ExpireAt

	if err := s.Extend(ctx, h, first.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := readHold(t, st, "u", h.ID).ExpireAt; !got.Equal(first) {
		t.Fatalf("an earlier expireAt must not shorten the hold: %s -> %s", first, got)
	}

	later := first.Add(20 * time.Minute)
	if err := s.Extend(ctx, h, later); err != nil {
		t.Fatal(err)
	}
	if got := readHold(t, st, "u", h.ID).ExpireAt; !got.Equal(storedTime(later)) {
		t.Fatalf("expireAt = %s, want %s", got, later)
	}
	if w := wallet(t, st, "u", c.now()); !w.Holds[h.ID].Equal(storedTime(later)) {
		t.Fatalf("wallet holds index = %v", w.Holds)
	}

	// past the old end the hold is still held and can be charged
	c.add(10 * time.Minute)
	v, err := s.Wallet(ctx, "u", false)
	if err != nil || v.Reserved != 10 {
		t.Fatalf("extended hold returned early: %+v %v", v, err)
	}
	got, err := s.CommitCoins(ctx, h, 7)
	if err != nil || got.Coins != 7 || got.Balance != 23 {
		t.Fatalf("commit after extend: %+v %v", got, err)
	}

	// a closed hold is left as is
	if err := s.Extend(ctx, h, later.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if hs := readHold(t, st, "u", h.ID); hs.Status != HoldCommitted || !hs.ExpireAt.IsZero() && hs.ExpireAt.After(later) {
		t.Fatalf("closed hold changed: %+v", hs)
	}
	if w := wallet(t, st, "u", c.now()); len(w.Holds) != 0 {
		t.Fatalf("a closed hold must not come back into the index: %v", w.Holds)
	}
	consistent(t, st, "u", c.now())
}

func TestExtendNilDryRunAndUnknown(t *testing.T) {
	ctx := context.Background()
	c := &clock{t: t0}
	st := NewMemory()
	s := newService(st, ModeEnforce, c)
	if err := s.Extend(ctx, nil, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Extend(ctx, &Hold{ID: "x", UID: "u", DryRun: true}, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Extend(ctx, &Hold{ID: "missing", UID: "u"}, t0.Add(time.Hour)); err != nil {
		t.Fatalf("an unknown hold is left as is: %v", err)
	}
	if w := wallet(t, st, "u", c.now()); len(w.Holds) != 0 {
		t.Fatalf("an unknown hold must not be indexed: %v", w.Holds)
	}
}

const (
	testBundle = "jp.kz2601.test"
	testPack   = "pack.small"
)

func appleService(t *testing.T, mode Mode) (*Service, *appstoretest.Signer) {
	t.Helper()
	sg := appstoretest.New(t, appstoretest.Options{})
	c := &clock{t: time.Now()}
	s := newService(NewMemory(), mode, c)
	s.Verifier = &appstore.Verifier{Roots: sg.Roots, Policy: appstore.Policy{
		BundleIDs: []string{testBundle}, ProductIDs: []string{testPack, "pack.unpriced"}, Environments: []string{appstore.EnvSandbox},
	}}
	return s, sg
}

func packJWS(t *testing.T, sg *appstoretest.Signer, product, txid, token string) string {
	x := appstoretest.Consumable(testBundle, product, txid, time.Now())
	x.AppAccountToken = token
	return sg.Sign(t, x)
}

func TestCreditAppleRejects(t *testing.T) {
	ctx := context.Background()
	s, sg := appleService(t, ModeEnforce)
	good := packJWS(t, sg, testPack, "tx1", appstore.AccountToken("u"))

	off, _ := appleService(t, ModeOff)
	if _, err := off.CreditApple(ctx, "u", false, good); !errors.Is(err, ErrOff) {
		t.Errorf("mode off: %v", err)
	}
	if _, err := s.CreditApple(ctx, "u", true, good); !errors.Is(err, ErrAnonymous) {
		t.Errorf("anonymous: %v", err)
	}
	noVerifier := newService(NewMemory(), ModeEnforce, &clock{t: t0})
	if _, err := noVerifier.CreditApple(ctx, "u", false, good); err == nil {
		t.Error("no verifier must fail closed")
	}

	reason := func(err error) string {
		var ae *appstore.Error
		if !errors.As(err, &ae) {
			return "not an appstore error: " + errStr(err)
		}
		return ae.Reason
	}
	for name, tc := range map[string]struct {
		jws, want string
	}{
		"malformed":       {jws: strings.Repeat("x", 64), want: appstore.ReasonMalformed},
		"other account":   {jws: packJWS(t, sg, testPack, "tx2", appstore.AccountToken("someone-else")), want: appstore.ReasonAccount},
		"no token":        {jws: packJWS(t, sg, testPack, "tx3", ""), want: appstore.ReasonAccount},
		"not a coin pack": {jws: packJWS(t, sg, "pack.unpriced", "tx4", appstore.AccountToken("u")), want: appstore.ReasonProduct},
	} {
		if _, err := s.CreditApple(ctx, "u", false, tc.jws); reason(err) != tc.want {
			t.Errorf("%s: %v, want reason %s", name, err, tc.want)
		}
	}
	if v, err := s.Wallet(ctx, "u", false); err != nil || v.Purchased != 0 {
		t.Fatalf("a rejected credit must add nothing: %+v %v", v, err)
	}
}

func TestCreditAppleOncePerTransaction(t *testing.T) {
	ctx := context.Background()
	s, sg := appleService(t, ModeEnforce)
	// iOS may send the UUID upper-cased
	jws := packJWS(t, sg, testPack, "tx1", strings.ToUpper(appstore.AccountToken("u")))
	got, err := s.CreditApple(ctx, "u", false, jws)
	if err != nil || !got.Credited || got.Coins != 50 || got.ProductID != testPack || got.Wallet.Purchased != 50 {
		t.Fatalf("credit = %+v %v", got, err)
	}
	again, err := s.CreditApple(ctx, "u", false, jws)
	if err != nil || again.Credited || again.Wallet.Purchased != 50 {
		t.Fatalf("retry must not credit twice: %+v %v", again, err)
	}
}

func errStr(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
