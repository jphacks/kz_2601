package coins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

// stores runs f on Memory and, with FIRESTORE_EMULATOR_HOST, on Firestore.
func stores(t *testing.T, f func(t *testing.T, st Store)) {
	t.Run("memory", func(t *testing.T) { f(t, NewMemory()) })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		c, err := fsenv.NewClient(context.Background(), fsenv.Target{Project: "demo-kz2601", Database: "(default)"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		f(t, NewFirestore(c))
	})
}

// uid makes ids unique per run (the emulator keeps data between runs).
func uid(t *testing.T, name string) string {
	return name + "-" + strings.ReplaceAll(t.Name(), "/", "_") + "-" + time.Now().Format("150405.000000000")
}

var t0 = time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func testConfig(mode Mode) Config {
	return Config{
		Mode: mode, PerUSD: 100, MinPerCall: 1, HoldTTL: 10 * time.Minute,
		Reserve:   map[string]int64{OpGenerate: 5, OpEdit: 4, OpChat: 2, OpText: 1},
		Estimates: map[string]int64{OpGenerate: 3},
		Packs:     []Pack{{ProductID: "pack.small", Coins: 50}},
	}
}

func newService(st Store, mode Mode, c *clock) *Service {
	return &Service{Config: testConfig(mode), Store: st, Now: c.now, Subjects: ownSubject}
}

// ownSubject makes every account its own person.
func ownSubject(_ context.Context, uid string) []string { return []string{"test:" + uid} }

// fund credits n purchased coins to uid directly.
func fund(t *testing.T, st Store, u string, n int64, now time.Time) {
	t.Helper()
	if _, _, err := CreditPurchase(context.Background(), st, u, Purchase{TransactionID: "fund-" + u + fmt.Sprint(n, now.UnixNano()), ProductID: "pack.small", Coins: n}, GrantPolicy{Anonymous: true}, now); err != nil {
		t.Fatal(err)
	}
}

func wallet(t *testing.T, st Store, u string, now time.Time) Wallet {
	t.Helper()
	w, err := GetWallet(context.Background(), st, u, GrantPolicy{Anonymous: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// consistent checks that the ledger adds up to the wallet, by source too.
func consistent(t *testing.T, st Store, u string, now time.Time) {
	t.Helper()
	es, _, err := st.Ledger(context.Background(), u, 1000, "")
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	var by Split
	for _, e := range es {
		if e.Amount != e.Delta.Sum() {
			t.Errorf("entry %s amount %d != delta %+v", e.ID, e.Amount, e.Delta)
		}
		sum += e.Amount
		by.Grant += e.Delta.Grant
		by.Initial += e.Delta.Initial
		by.Purchased += e.Delta.Purchased
	}
	w := wallet(t, st, u, now)
	if sum != w.Total() {
		t.Errorf("ledger sum %d != wallet total %d (%+v)", sum, w.Total(), w)
	}
	if w.Reserved == 0 && (by.Purchased != w.Purchased || by.Initial != w.Initial || by.Grant != w.Grant) {
		t.Errorf("ledger by source %+v != wallet %+v", by, w)
	}
	if w.Purchased < 0 || w.Grant < 0 || w.Initial < 0 || w.Reserved < 0 {
		t.Errorf("negative wallet %+v", w)
	}
}

func TestReserveCommitRelease(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 10, c.now())

		h, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "r1"})
		if err != nil || h.Amount != 5 || h.Status != HoldHeld {
			t.Fatalf("reserve: %+v %v", h, err)
		}
		if w := wallet(t, st, u, c.now()); w.Available() != 5 || w.Reserved != 5 {
			t.Fatalf("after reserve %+v", w)
		}
		// 0.021 USD × 100 = 2.1 → 3 coins
		ch, err := s.Commit(ctx, h, Usage{USD: 0.021, Priced: true})
		if err != nil || ch.Coins != 3 || ch.Balance != 7 {
			t.Fatalf("commit: %+v %v", ch, err)
		}
		// committing again changes nothing
		if ch, err := s.Commit(ctx, h, Usage{USD: 0.021, Priced: true}); err != nil || ch.Coins != 3 {
			t.Fatalf("commit again: %+v %v", ch, err)
		}
		h2, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpChat, RequestID: "r2"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Release(ctx, h2); err != nil || h2.Status != HoldReleased {
			t.Fatalf("release: %+v %v", h2, err)
		}
		// a released hold is never charged
		if ch, err := s.Commit(ctx, h2, Usage{USD: 1, Priced: true}); err != nil || ch.Coins != 0 {
			t.Fatalf("commit after release: %+v %v", ch, err)
		}
		w := wallet(t, st, u, c.now())
		if w.Available() != 7 || w.Reserved != 0 || len(w.Holds) != 0 {
			t.Fatalf("end %+v", w)
		}
		consistent(t, st, u, c.now())
	})
}

func TestCommitCaps(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 20, c.now())
		cases := []struct {
			usage Usage
			want  int64
		}{
			{Usage{USD: 10, Priced: true}, 5},     // over the hold: the hold
			{Usage{USD: 0.001, Priced: false}, 5}, // unpriced model: the hold
			{Usage{USD: 0, Priced: true}, 1},      // the minimum
		}
		for i, tc := range cases {
			h, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: fmt.Sprint("cap", i)})
			if err != nil {
				t.Fatal(err)
			}
			if ch, err := s.Commit(ctx, h, tc.usage); err != nil || ch.Coins != tc.want {
				t.Errorf("%d: %+v %v, want %d", i, ch, err, tc.want)
			}
		}
		consistent(t, st, u, c.now())
	})
}

func TestReserveInsufficientWritesNothing(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 3, c.now())
		_, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "r"})
		var ins *InsufficientError
		if !errors.As(err, &ins) || ins.Required != 5 || ins.Balance != 3 {
			t.Fatalf("err %v", err)
		}
		if w := wallet(t, st, u, c.now()); w.Available() != 3 || w.Reserved != 0 {
			t.Fatalf("wallet %+v", w)
		}
	})
}

func TestReserveIdempotentPerRequestID(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u, other := uid(t, "u"), uid(t, "o")
		fund(t, st, u, 10, c.now())
		fund(t, st, other, 10, c.now())
		h1, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "same"})
		if err != nil {
			t.Fatal(err)
		}
		h2, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "same"})
		if err != nil || !h2.Existing || h2.ID != h1.ID {
			t.Fatalf("retry: %+v %v", h2, err)
		}
		if w := wallet(t, st, u, c.now()); w.Reserved != 5 {
			t.Fatalf("taken twice: %+v", w)
		}
		// the same request id of another user is another hold
		h3, err := s.Reserve(ctx, ReserveRequest{UID: other, Op: OpGenerate, RequestID: "same"})
		if err != nil || h3.Existing || h3.ID == h1.ID {
			t.Fatalf("other user: %+v %v", h3, err)
		}
		// a retry after commit returns the committed hold
		if _, err := s.Commit(ctx, h1, Usage{USD: 0.01, Priced: true}); err != nil {
			t.Fatal(err)
		}
		h4, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "same"})
		if err != nil || !h4.Existing || h4.Status != HoldCommitted || h4.Charged != 1 {
			t.Fatalf("after commit: %+v %v", h4, err)
		}
		if w := wallet(t, st, u, c.now()); w.Available() != 9 || w.Reserved != 0 {
			t.Fatalf("wallet %+v", w)
		}
	})
}

func TestParallelReservesNeverOverdraw(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 12, c.now())
		const n = 12
		var wg sync.WaitGroup
		var mu sync.Mutex
		var held int64
		var holds []*Hold
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpEdit, RequestID: fmt.Sprint("p", i)}) // 4 each
				if err != nil {
					var ins *InsufficientError
					if !errors.As(err, &ins) {
						t.Logf("reserve %d: %v", i, err) // contention on the emulator: nothing taken
					}
					return
				}
				mu.Lock()
				held += h.Amount
				holds = append(holds, h)
				mu.Unlock()
			}()
		}
		wg.Wait()
		if held > 12 || len(holds) == 0 {
			t.Fatalf("held %d in %d holds from 12 coins", held, len(holds))
		}
		w := wallet(t, st, u, c.now())
		if w.Reserved != held || w.Available() != 12-held {
			t.Fatalf("wallet %+v, held %d", w, held)
		}
		// settle them in parallel too
		for i, h := range holds {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var err error
				if i%2 == 0 {
					_, err = s.Commit(ctx, h, Usage{USD: 0.02, Priced: true})
				} else {
					err = s.Release(ctx, h)
				}
				if err != nil {
					t.Errorf("settle: %v", err)
				}
			}()
		}
		wg.Wait()
		consistent(t, st, u, c.now())
	})
}

func TestHoldExpiryReturnsCoins(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 10, c.now())
		h, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "lost"})
		if err != nil {
			t.Fatal(err)
		}
		c.add(5 * time.Minute)
		if v, _ := s.Wallet(ctx, u, false); v.Available != 5 {
			t.Fatalf("before the ttl: %+v", v)
		}
		c.add(6 * time.Minute)
		v, err := s.Wallet(ctx, u, false)
		if err != nil || v.Available != 10 || v.Reserved != 0 {
			t.Fatalf("after the ttl: %+v %v", v, err)
		}
		// a late commit does not charge
		if ch, err := s.Commit(ctx, h, Usage{USD: 0.01, Priced: true}); err != nil || ch.Coins != 0 {
			t.Fatalf("late commit %+v %v", ch, err)
		}
		consistent(t, st, u, c.now())
	})
}

func TestSweepReturnsExpiredHolds(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0.Add(-24 * time.Hour)} // older than other tests' holds on a shared emulator
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 10, c.now())
		if _, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "a"}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpChat, RequestID: "b"}); err != nil {
			t.Fatal(err)
		}
		c.add(11 * time.Minute)
		for range 20 { // other runs' holds may be older on the emulator
			n, err := s.Sweep(ctx, 50)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
		}
		w := wallet(t, st, u, c.now())
		if w.Available() != 10 || w.Reserved != 0 || len(w.Holds) != 0 {
			t.Fatalf("after sweep %+v", w)
		}
		consistent(t, st, u, c.now())
	})
}

func TestMonthlyGrantExpires(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		end := t0.Add(30 * 24 * time.Hour)
		s := newService(st, ModeEnforce, c)
		s.Config.ProMonthly = 20
		s.ProPeriodEnd = func(context.Context, string) (time.Time, error) {
			if c.now().Before(end) {
				return end, nil
			}
			return time.Time{}, nil // not renewed
		}
		u := uid(t, "u")
		fund(t, st, u, 3, c.now())
		v, err := s.Wallet(ctx, u, false)
		if err != nil || v.Grant != 20 || !v.GrantPeriodEnd.Equal(end) || v.Purchased != 3 {
			t.Fatalf("granted %+v %v", v, err)
		}
		// the same period is granted once
		if v, _ := s.Wallet(ctx, u, false); v.Grant != 20 {
			t.Fatalf("twice %+v", v)
		}
		// spend order: grant first
		h, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "g", Max: 22})
		if err != nil || h.Parts.Grant != 20 || h.Parts.Purchased != 2 {
			t.Fatalf("parts %+v %v", h, err)
		}
		c.t = end.Add(-time.Minute)
		if err := s.Release(ctx, h); err != nil {
			t.Fatal(err)
		}
		if v, _ := s.Wallet(ctx, u, false); v.Grant != 20 || v.Purchased != 3 {
			t.Fatalf("released before the end %+v", v)
		}
		// a hold over the period end loses its grant part, keeps the bought part
		h, err = s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "g2", Max: 22})
		if err != nil {
			t.Fatal(err)
		}
		c.t = end.Add(time.Second)
		if _, err := s.Commit(ctx, h, Usage{USD: 0.001, Priced: true}); err != nil { // 1 coin from the grant part
			t.Fatal(err)
		}
		v, _ = s.Wallet(ctx, u, false)
		if v.Grant != 0 || v.Purchased != 3 || v.Reserved != 0 {
			t.Fatalf("after the period %+v", v)
		}
		// renewed: a new period grants again; leftovers never carry over
		end2 := end.Add(30 * 24 * time.Hour)
		s.ProPeriodEnd = func(context.Context, string) (time.Time, error) { return end2, nil }
		if v, _ := s.Wallet(ctx, u, false); v.Grant != 20 || !v.GrantPeriodEnd.Equal(end2) {
			t.Fatalf("renewed %+v", v)
		}
		consistent(t, st, u, c.now())
	})
}

func TestInitialGrantOnceForSignedIn(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		s.Config.InitialGrant = 7
		u := uid(t, "u")
		if v, _ := s.Wallet(ctx, u, true); v.Initial != 0 {
			t.Fatalf("anonymous got %+v", v)
		}
		if v, _ := s.Wallet(ctx, u, false); v.Initial != 7 {
			t.Fatalf("signed in got %+v", v)
		}
		if v, _ := s.Wallet(ctx, u, false); v.Initial != 7 {
			t.Fatalf("twice %+v", v)
		}
		fund(t, st, u, 5, c.now())
		h, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "i", Max: 9})
		if err != nil || h.Parts.Initial != 7 || h.Parts.Purchased != 2 {
			t.Fatalf("spend order %+v %v", h, err)
		}
		if _, err := s.Commit(ctx, h, Usage{USD: 0.08, Priced: true}); err != nil { // 8: 7 initial + 1 bought
			t.Fatal(err)
		}
		if v, _ := s.Wallet(ctx, u, false); v.Initial != 0 || v.Purchased != 4 {
			t.Fatalf("after %+v", v)
		}
		consistent(t, st, u, c.now())
	})
}

// The initial grant is once per person (sign-in subject), not per account:
// a second account of the same Apple ID, also after the first is deleted,
// gets none; another person still gets theirs.
func TestInitialGrantOncePerPerson(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		s.Config.InitialGrant = 7
		person, other := "apple.com:"+uid(t, "sub"), "apple.com:"+uid(t, "sub2")
		first, second, third, stranger := uid(t, "a"), uid(t, "b"), uid(t, "c"), uid(t, "d")
		subs := map[string][]string{first: {person}, second: {person}, third: {other, person}, stranger: {other}}
		s.Subjects = func(_ context.Context, u string) []string { return subs[u] }

		if v, err := s.Wallet(ctx, first, false); err != nil || v.Initial != 7 {
			t.Fatalf("first %+v %v", v, err)
		}
		if v, _ := s.Wallet(ctx, first, false); v.Initial != 7 {
			t.Fatalf("first again %+v", v)
		}
		if _, err := st.PurgeUser(ctx, first); err != nil {
			t.Fatal(err)
		}
		if v, _ := s.Wallet(ctx, second, false); v.Initial != 0 || v.Total != 0 {
			t.Fatalf("same person, new account %+v", v)
		}
		// one claimed subject is enough to refuse, and the new one is not claimed
		if v, _ := s.Wallet(ctx, third, false); v.Initial != 0 {
			t.Fatalf("linked subjects %+v", v)
		}
		if v, _ := s.Wallet(ctx, stranger, false); v.Initial != 7 {
			t.Fatalf("another person %+v", v)
		}
		consistent(t, st, second, c.now())
	})
}

// Without the payer's subjects (the payer is not the verified caller) the
// initial grant waits instead of being given or refused.
func TestInitialGrantWaitsForSubjects(t *testing.T) {
	ctx, c := context.Background(), &clock{t: t0}
	st := NewMemory()
	s := newService(st, ModeEnforce, c)
	s.Config.InitialGrant = 7
	s.Subjects = nil // from the request: none in a bare context
	u := uid(t, "u")
	if v, _ := s.Wallet(ctx, u, false); v.Initial != 0 {
		t.Fatalf("no caller %+v", v)
	}
	if v, _ := s.Wallet(auth.ContextWithIdentity(ctx, auth.Identity{UID: "someone-else", Subjects: []string{"apple.com:x"}}), u, false); v.Initial != 0 {
		t.Fatalf("other caller %+v", v)
	}
	if v, _ := s.Wallet(auth.ContextWithIdentity(ctx, auth.Identity{UID: u, Subjects: []string{"apple.com:y"}}), u, false); v.Initial != 7 {
		t.Fatalf("caller %+v", v)
	}
}

func TestCreditPurchaseOncePerTransaction(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx := context.Background()
		u, other := uid(t, "u"), uid(t, "o")
		p := Purchase{TransactionID: uid(t, "tx"), ProductID: "pack.small", Coins: 50, Environment: "Sandbox"}
		w, ok, err := CreditPurchase(ctx, st, u, p, GrantPolicy{}, t0)
		if err != nil || !ok || w.Purchased != 50 {
			t.Fatalf("first %+v %v %v", w, ok, err)
		}
		w, ok, err = CreditPurchase(ctx, st, u, p, GrantPolicy{}, t0.Add(time.Minute))
		if err != nil || ok || w.Purchased != 50 {
			t.Fatalf("again %+v %v %v", w, ok, err)
		}
		if _, _, err := CreditPurchase(ctx, st, other, p, GrantPolicy{}, t0); !errors.Is(err, ErrOwnedByOther) {
			t.Fatalf("other account: %v", err)
		}
		if w := wallet(t, st, other, t0); w.Total() != 0 {
			t.Fatalf("other credited %+v", w)
		}
		consistent(t, st, u, t0)
	})
}

func TestLedgerPaging(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		for range 5 {
			fund(t, st, u, 1, c.now())
			c.add(time.Second)
		}
		var all []Entry
		cursor := ""
		for page := 0; ; page++ {
			es, next, err := s.Ledger(ctx, u, 2, cursor)
			if err != nil {
				t.Fatal(err)
			}
			all = append(all, es...)
			if next == "" {
				break
			}
			if page > 5 {
				t.Fatal("no end")
			}
			cursor = next
		}
		if len(all) != 5 {
			t.Fatalf("got %d entries", len(all))
		}
		for i := 1; i < len(all); i++ {
			if !before(all[i-1], all[i]) {
				t.Fatalf("order %v then %v", all[i-1].At, all[i].At)
			}
		}
		if _, _, err := s.Ledger(ctx, u, 2, "nope"); !errors.Is(err, ErrBadCursor) {
			t.Fatalf("bad cursor: %v", err)
		}
	})
}

func TestPurgeUser(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeEnforce, c)
		u := uid(t, "u")
		fund(t, st, u, 10, c.now())
		if _, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "x"}); err != nil {
			t.Fatal(err)
		}
		if n, err := st.PurgeUser(ctx, u); err != nil || n < 3 {
			t.Fatalf("purge %d %v", n, err)
		}
		if w := wallet(t, st, u, c.now()); w.Total() != 0 {
			t.Fatalf("left %+v", w)
		}
		if es, _, _ := st.Ledger(ctx, u, 10, ""); len(es) != 0 {
			t.Fatalf("ledger left %d", len(es))
		}
	})
}

// failStore fails the test on any use (mode off must not touch the store).
type failStore struct{ t *testing.T }

func (f failStore) Update(context.Context, Keys, func(*Tx) error) error {
	f.t.Error("Update in mode off")
	return nil
}
func (f failStore) Expired(context.Context, time.Time, int) ([]HoldRef, error) {
	f.t.Error("Expired in mode off")
	return nil, nil
}
func (f failStore) Ledger(context.Context, string, int, string) ([]Entry, string, error) {
	f.t.Error("Ledger in mode off")
	return nil, "", nil
}
func (f failStore) PurgeUser(context.Context, string) (int, error) { return 0, nil }

func TestModeOffTouchesNothing(t *testing.T) {
	ctx, c := context.Background(), &clock{t: t0}
	for _, s := range []*Service{newService(failStore{t}, ModeOff, c), {Store: failStore{t}}} {
		s.Config.InitialGrant, s.Config.ProMonthly = 5, 5
		s.ProPeriodEnd = func(context.Context, string) (time.Time, error) {
			t.Error("ProPeriodEnd in mode off")
			return time.Time{}, nil
		}
		if v, err := s.Wallet(ctx, "u", false); err != nil || v.Mode != ModeOff || v.Total != 0 {
			t.Fatalf("wallet %+v %v", v, err)
		}
		h, err := s.Reserve(ctx, ReserveRequest{UID: "u", Op: OpGenerate, RequestID: "r"})
		if err != nil || h != nil {
			t.Fatalf("reserve %+v %v", h, err)
		}
		if ch, err := s.Commit(ctx, h, Usage{USD: 1, Priced: true}); err != nil || ch != (Charge{}) {
			t.Fatalf("commit %+v %v", ch, err)
		}
		if err := s.Release(ctx, h); err != nil {
			t.Fatal(err)
		}
		if es, _, err := s.Ledger(ctx, "u", 10, ""); err != nil || len(es) != 0 {
			t.Fatalf("ledger %v %v", es, err)
		}
		if n, err := s.Sweep(ctx, 10); err != nil || n != 0 {
			t.Fatalf("sweep %d %v", n, err)
		}
		if _, err := s.CreditApple(ctx, "u", false, "x.y.z"); !errors.Is(err, ErrOff) {
			t.Fatalf("credit %v", err)
		}
	}
}

func TestModeLogHoldsNothing(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ctx, c := context.Background(), &clock{t: t0}
		s := newService(st, ModeLog, c)
		s.Config.InitialGrant = 2
		u := uid(t, "u")
		h, err := s.Reserve(ctx, ReserveRequest{UID: u, Op: OpGenerate, RequestID: "r"}) // short: logged only
		if err != nil || h == nil || !h.DryRun {
			t.Fatalf("reserve %+v %v", h, err)
		}
		if ch, err := s.Commit(ctx, h, Usage{USD: 0.02, Priced: true}); err != nil || ch.Coins != 0 {
			t.Fatalf("commit %+v %v", ch, err)
		}
		v, err := s.Wallet(ctx, u, false)
		if err != nil || v.Mode != ModeLog || v.Available != 2 || v.Reserved != 0 {
			t.Fatalf("wallet %+v %v", v, err)
		}
		if n, _ := s.Sweep(ctx, 10); n != 0 {
			t.Fatalf("sweep in log %d", n)
		}
	})
}
