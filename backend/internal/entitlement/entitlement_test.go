package entitlement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

const (
	bundle  = "jp.kz2601.metamon"
	product = "jp.kz2601.metamon.pro.monthly"
)

type env struct {
	signer  *appstoretest.Signer
	svc     *Service
	deleted map[string]bool
	now     time.Time
}

func newEnv(t *testing.T, st Store) *env {
	t.Helper()
	e := &env{signer: appstoretest.New(t, appstoretest.Options{}), deleted: map[string]bool{}, now: time.Now()}
	e.svc = &Service{
		Verifier: &appstore.Verifier{Roots: e.signer.Roots, Policy: appstore.Policy{
			BundleIDs: []string{bundle}, ProductIDs: []string{product}, Environments: DefaultEnvironments,
		}},
		Store:   st,
		Deleted: func(_ context.Context, uid string) (bool, error) { return e.deleted[uid], nil },
		Now:     func() time.Time { return e.now },
	}
	return e
}

func (e *env) jws(t *testing.T, tx appstoretest.Transaction) string { return e.signer.Sign(t, tx) }

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

func TestSubmitBindsAndGrantsPro(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e := newEnv(t, st)
		ctx := context.Background()
		alice := uid(t, "alice")
		otid := uid(t, "otid")
		got, err := e.svc.SubmitApple(ctx, alice, e.jws(t, appstoretest.Monthly(bundle, product, otid, e.now)))
		if err != nil {
			t.Fatal(err)
		}
		if !got.IsPro || got.Pro.OriginalTransactionID != otid || got.Pro.Environment != "Sandbox" {
			t.Fatalf("got %+v", got)
		}
		// another device / launch of the same account
		if again, err := e.svc.Get(ctx, alice); err != nil || !again.IsPro {
			t.Fatalf("get: %+v %v", again, err)
		}
		if owner, _ := st.Owner(ctx, otid); owner != alice {
			t.Fatalf("owner %q", owner)
		}
	})
}

func TestSubmitRejectsOtherActiveAccount(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e := newEnv(t, st)
		ctx := context.Background()
		alice, bob, otid := uid(t, "alice"), uid(t, "bob"), uid(t, "otid")
		jws := e.jws(t, appstoretest.Monthly(bundle, product, otid, e.now))
		if _, err := e.svc.SubmitApple(ctx, alice, jws); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.SubmitApple(ctx, bob, jws); !errors.Is(err, ErrOwnedByOther) {
			t.Fatalf("want ErrOwnedByOther, got %v", err)
		}
		if st, _ := e.svc.Get(ctx, bob); st.IsPro || st.Pro != nil {
			t.Fatalf("bob must not be pro: %+v", st)
		}
		// once alice deletes her account, the same Apple ID may restore on bob
		e.deleted[alice] = true
		got, err := e.svc.SubmitApple(ctx, bob, jws)
		if err != nil || !got.IsPro {
			t.Fatalf("transfer: %+v %v", got, err)
		}
		if owner, _ := st.Owner(ctx, otid); owner != bob {
			t.Fatalf("owner %q", owner)
		}
	})
}

func TestSubmitOrderingAndRevocation(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e := newEnv(t, st)
		ctx := context.Background()
		alice, otid := uid(t, "alice"), uid(t, "otid")
		first := appstoretest.Monthly(bundle, product, otid, e.now.AddDate(0, -1, 0).Add(time.Hour)) // expires in 1h
		renewal := appstoretest.Monthly(bundle, product, otid, e.now)
		renewal.TransactionID = otid + "-2"

		if _, err := e.svc.SubmitApple(ctx, alice, e.jws(t, renewal)); err != nil {
			t.Fatal(err)
		}
		// an older transaction arriving late must not shorten the entitlement
		got, err := e.svc.SubmitApple(ctx, alice, e.jws(t, first))
		if err != nil || got.Pro.TransactionID != renewal.TransactionID {
			t.Fatalf("older replaced newer: %+v %v", got.Pro, err)
		}
		// refund of the current transaction
		refunded := renewal
		refunded.RevocationDate = e.now.UnixMilli()
		got, err = e.svc.SubmitApple(ctx, alice, e.jws(t, refunded))
		if err != nil || got.IsPro || !got.Pro.Revoked() {
			t.Fatalf("revocation: %+v %v", got, err)
		}
	})
}

func TestSubmitExpiredIsStoredButNotPro(t *testing.T) {
	e := newEnv(t, NewMemory())
	old := appstoretest.Monthly(bundle, product, "o1", e.now.AddDate(0, -3, 0))
	old.SignedDate = e.now.UnixMilli()
	got, err := e.svc.SubmitApple(context.Background(), "u", e.jws(t, old))
	if err != nil || got.IsPro || got.Pro == nil {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestEnvironmentFilter(t *testing.T) {
	e := newEnv(t, NewMemory())
	e.svc.Environments = []string{appstore.EnvProduction}
	got, err := e.svc.SubmitApple(context.Background(), "u", e.jws(t, appstoretest.Monthly(bundle, product, "o", e.now)))
	if err != nil || got.IsPro {
		t.Fatalf("sandbox must not count when only Production is accepted: %+v %v", got, err)
	}
}

func TestPurgeUser(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		e := newEnv(t, st)
		ctx := context.Background()
		alice, bob, otid := uid(t, "alice"), uid(t, "bob"), uid(t, "otid")
		if _, err := e.svc.SubmitApple(ctx, alice, e.jws(t, appstoretest.Monthly(bundle, product, otid, e.now))); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.SubmitApple(ctx, bob, e.jws(t, appstoretest.Monthly(bundle, product, otid+"b", e.now))); err != nil {
			t.Fatal(err)
		}
		n, err := st.PurgeUser(ctx, alice)
		if err != nil || n != 2 {
			t.Fatalf("purged %d %v", n, err)
		}
		if p, _ := st.Get(ctx, alice); p != nil {
			t.Fatalf("pro kept: %+v", p)
		}
		if owner, _ := st.Owner(ctx, otid); owner != "" {
			t.Fatalf("binding kept: %q", owner)
		}
		if p, _ := st.Get(ctx, bob); p == nil {
			t.Fatal("bob's entitlement must stay")
		}
	})
}

func TestHandler(t *testing.T) {
	e := newEnv(t, NewMemory())
	mux := http.NewServeMux()
	Register(mux, e.svc)
	h := auth.Middleware(auth.DevVerifier{}, auth.ModeDev)(mux)
	do := func(uid, method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		r := httptest.NewRequest(method, path, &buf)
		if uid != "" {
			r.Header.Set("Authorization", "Bearer "+uid)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	good := e.jws(t, appstoretest.Monthly(bundle, product, "o", e.now))

	if w := do("", "GET", "/v1/me/entitlements", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	if w := do("u1", "GET", "/v1/me/entitlements", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"isPro":false`) {
		t.Fatalf("empty: %d %s", w.Code, w.Body)
	}
	if w := do("u1", "POST", "/v1/me/entitlements/apple", map[string]string{}); w.Code != 400 {
		t.Fatalf("missing: %d", w.Code)
	}
	tampered := good[:len(good)-4] + "AAAA"
	if w := do("u1", "POST", "/v1/me/entitlements/apple", map[string]string{"signedTransaction": tampered}); w.Code != 422 ||
		!strings.Contains(w.Body.String(), `"reason":"signature"`) || !strings.Contains(w.Body.String(), httpx.CodeInvalidTransaction) {
		t.Fatalf("tampered: %d %s", w.Code, w.Body)
	}
	w := do("u1", "POST", "/v1/me/entitlements/apple", map[string]string{"signedTransaction": good})
	if w.Code != 200 {
		t.Fatalf("ok: %d %s", w.Code, w.Body)
	}
	var st Status
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || !st.IsPro || st.Pro.ProductID != product {
		t.Fatalf("body %s", w.Body)
	}
	if w := do("u2", "POST", "/v1/me/entitlements/apple", map[string]string{"signedTransaction": good}); w.Code != 409 ||
		!strings.Contains(w.Body.String(), httpx.CodeTransactionOwnedByOther) {
		t.Fatalf("other: %d %s", w.Code, w.Body)
	}
}

func TestProChecker(t *testing.T) {
	m := NewMemory()
	now := time.Now()
	c := NewProChecker(m, nil)
	c.Now = func() time.Time { return now }
	ctx := context.Background()
	if c.IsPro(ctx, "u") {
		t.Fatal("empty is not pro")
	}
	if _, err := m.Apply(ctx, "u", Pro{ProductID: product, OriginalTransactionID: "o", TransactionID: "t", Environment: "Sandbox", ExpiresAt: now.Add(time.Hour)}, ""); err != nil {
		t.Fatal(err)
	}
	if c.IsPro(ctx, "u") {
		t.Fatal("negative result is cached for NegTTL")
	}
	now = now.Add(16 * time.Second)
	if !c.IsPro(ctx, "u") {
		t.Fatal("pro after NegTTL")
	}
	// cache never outlives expiresAt
	now = now.Add(time.Hour)
	if c.IsPro(ctx, "u") {
		t.Fatal("expired")
	}
}

func TestMerge(t *testing.T) {
	now := time.Now()
	cur := &Pro{OriginalTransactionID: "o", TransactionID: "t2", ExpiresAt: now.Add(48 * time.Hour)}
	if _, w := merge(cur, Pro{OriginalTransactionID: "o", TransactionID: "t1", ExpiresAt: now.Add(time.Hour)}, now); w {
		t.Fatal("older must not replace")
	}
	if _, w := merge(cur, Pro{OriginalTransactionID: "o", TransactionID: "t3", ExpiresAt: now.Add(72 * time.Hour)}, now); !w {
		t.Fatal("renewal must replace")
	}
	revoked := &Pro{OriginalTransactionID: "y", TransactionID: "ty", ExpiresAt: now.Add(300 * 24 * time.Hour), RevokedAt: now}
	if _, w := merge(revoked, Pro{OriginalTransactionID: "m", TransactionID: "tm", ExpiresAt: now.Add(24 * time.Hour)}, now); !w {
		t.Fatal("an active subscription replaces a refunded one")
	}
}

func TestAccountToken(t *testing.T) {
	a := AccountToken("firebase-uid-123")
	if len(a) != 36 || a[14] != '8' || !strings.ContainsAny(a[19:20], "89ab") {
		t.Fatalf("not a UUIDv8: %s", a)
	}
	if a != AccountToken("firebase-uid-123") || a == AccountToken("other") {
		t.Fatal("must be deterministic and uid-specific")
	}
	// shared vector with ios ProAccountTokenTests
	if a != "578b5ddb-6720-8e0a-a511-88fa2b02abdb" {
		t.Fatalf("vector changed: %s", a)
	}
}

func TestConfigFromEnv(t *testing.T) {
	p, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || p.BundleIDs[0] != bundle || p.ProductIDs[0] != product || len(p.Environments) != 2 {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := ConfigFromEnv(func(k string) string {
		if k == "APPSTORE_ENVIRONMENTS" {
			return "Xcode"
		}
		return ""
	}); err == nil {
		t.Fatal("Xcode must be refused")
	}
}
