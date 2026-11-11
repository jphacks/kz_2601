package coins

import (
	"context"
	"slices"
	"testing"
)

func TestBackfillClaims(t *testing.T) {
	me, other := OwnerHash("me"), OwnerHash("other")
	cases := []struct {
		name    string
		granted bool
		keys    []string
		have    map[string]InitialClaim
		want    BackfillOutcome
		write   []string
	}{
		{"no subject", true, nil, nil, BackfillNoSubject, nil},
		{"not granted", false, []string{"k1"}, nil, BackfillNotGranted, nil},
		{"granted, none recorded", true, []string{"k1", "k2"}, nil, BackfillWrite, []string{"k1", "k2"}},
		{"granted, one recorded by me", true, []string{"k1", "k2"}, map[string]InitialClaim{"k1": {Owner: me}}, BackfillWrite, []string{"k2"}},
		{"all recorded by me", true, []string{"k1"}, map[string]InitialClaim{"k1": {Owner: me}}, BackfillRecorded, nil},
		{"one recorded by another account", true, []string{"k1", "k2"}, map[string]InitialClaim{"k2": {Owner: other}}, BackfillOtherOwner, nil},
	}
	for _, c := range cases {
		got, write := backfillClaims(c.granted, me, c.keys, c.have)
		if got != c.want || !slices.Equal(write, c.write) {
			t.Errorf("%s: %v %v, want %v %v", c.name, got, write, c.want, c.write)
		}
	}
}

// The backfill writes the claims the grant would have written today (same
// InitialKey / OwnerHash), only with apply, never touches the wallet, and a
// second run finds them. A later account of the same person then gets none.
func TestBackfillInitial(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		person, stranger := "apple.com:"+uid(t, "sub"), "google.com:"+uid(t, "gsub")
		before, fresh, newcomer, again := uid(t, "before"), uid(t, "fresh"), uid(t, "newcomer"), uid(t, "again")
		// the initial grant given before the claims existed
		if err := st.Update(ctx, Keys{UID: before}, func(tx *Tx) error {
			tx.Wallet.Initial, tx.Wallet.InitialGranted, tx.WalletDirty = 200, true, true
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		run := func(u string, subs []string, apply bool) (BackfillOutcome, int) {
			t.Helper()
			out, n, err := BackfillInitial(ctx, st, u, subs, apply, c.now())
			if err != nil {
				t.Fatal(err)
			}
			return out, n
		}
		if out, n := run(before, []string{person}, false); out != BackfillWrite || n != 1 {
			t.Fatalf("dry run: %v %d", out, n)
		}
		if out, n := run(before, []string{person}, false); out != BackfillWrite || n != 1 {
			t.Fatalf("a dry run writes nothing: %v %d", out, n)
		}
		if out, n := run(before, []string{person}, true); out != BackfillWrite || n != 1 {
			t.Fatalf("apply: %v %d", out, n)
		}
		if out, n := run(before, []string{person}, true); out != BackfillRecorded || n != 0 {
			t.Fatalf("second run: %v %d", out, n)
		}
		if out, _ := run(fresh, []string{stranger}, true); out != BackfillNotGranted {
			t.Fatalf("no wallet: %v", out)
		}
		if out, _ := run(fresh, nil, true); out != BackfillNoSubject {
			t.Fatalf("no subject: %v", out)
		}
		if w := wallet(t, st, before, c.now()); w.Initial != 200 || !w.InitialGranted {
			t.Fatalf("wallet untouched: %+v", w)
		}

		// the grant reads the claim the backfill wrote
		s := newService(st, ModeEnforce, c)
		s.Config.InitialGrant = 200
		subs := map[string][]string{newcomer: {person}, again: {stranger}}
		s.Subjects = func(_ context.Context, u string) []string { return subs[u] }
		if v, err := s.Wallet(ctx, newcomer, false); err != nil || v.Initial != 0 {
			t.Fatalf("same person, new account: %+v %v", v, err)
		}
		if v, err := s.Wallet(ctx, again, false); err != nil || v.Initial != 200 {
			t.Fatalf("another person: %+v %v", v, err)
		}
		// newcomer's wallet is now marked granted, but its subject is before's
		if out, _ := run(newcomer, []string{person}, true); out != BackfillOtherOwner {
			t.Fatalf("subject of another account: %v", out)
		}
	})
}
