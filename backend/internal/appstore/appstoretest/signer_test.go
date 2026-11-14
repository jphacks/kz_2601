package appstoretest_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
)

const (
	bundle2758   = "jp.kz2601.metamon"
	product2758  = "jp.kz2601.metamon.pro.monthly"
	coinPack2758 = "jp.kz2601.metamon.coins.100"
)

func verifier2758(s *appstoretest.Signer) *appstore.Verifier {
	return &appstore.Verifier{Roots: s.Roots, Policy: appstore.Policy{
		BundleIDs:    []string{bundle2758},
		ProductIDs:   []string{product2758, coinPack2758},
		Environments: []string{appstore.EnvSandbox, appstore.EnvProduction},
	}}
}

func reason2758(t *testing.T, err error) string {
	t.Helper()
	var e *appstore.Error
	if !errors.As(err, &e) {
		t.Fatalf("want *appstore.Error, got %v", err)
	}
	return e.Reason
}

func TestMonthlyVerifies2758(t *testing.T) {
	s := appstoretest.New(t, appstoretest.Options{})
	now := time.Now()
	got, err := verifier2758(s).Verify(s.Sign(t, appstoretest.Monthly(bundle2758, product2758, "2000000758", now)))
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalTransactionID != "2000000758" || got.ProductID != product2758 || got.BundleID != bundle2758 {
		t.Fatalf("got %+v", got)
	}
	if !got.ExpiresDate.After(now.AddDate(0, 0, 27)) {
		t.Fatalf("expires %s, want about one month after %s", got.ExpiresDate, now)
	}
}

func TestConsumableVerifies2758(t *testing.T) {
	s := appstoretest.New(t, appstoretest.Options{})
	got, err := verifier2758(s).VerifyConsumable(s.Sign(t, appstoretest.Consumable(bundle2758, coinPack2758, "3000000758", time.Now())))
	if err != nil {
		t.Fatal(err)
	}
	if got.TransactionID != "3000000758" || got.Type != appstore.TypeConsumable || !got.ExpiresDate.IsZero() {
		t.Fatalf("got %+v", got)
	}
	// A consumable is not a subscription.
	_, err = verifier2758(s).Verify(s.Sign(t, appstoretest.Consumable(bundle2758, coinPack2758, "3000000759", time.Now())))
	if r := reason2758(t, err); r != appstore.ReasonNotSubscription {
		t.Fatalf("reason %q", r)
	}
}

func TestRenewal2758(t *testing.T) {
	tx := appstoretest.Monthly(bundle2758, product2758, "2000000758", time.Now())
	r := appstoretest.Renewal(tx)
	if r.OriginalTransactionID != tx.OriginalTransactionID || r.ProductID != product2758 ||
		r.AutoRenewProductID != product2758 || r.AutoRenewStatus != 1 ||
		r.Environment != tx.Environment || r.SignedDate != tx.SignedDate || r.GracePeriodExpiresDate != 0 {
		t.Fatalf("renewal %+v", r)
	}
	// Signed output carries the fields in its payload.
	s := appstoretest.New(t, appstoretest.Options{})
	parts := strings.Split(s.Sign(t, r), ".")
	if len(parts) != 3 {
		t.Fatalf("segments %d", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var back appstoretest.RenewalInfo
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back != r {
		t.Fatalf("roundtrip %+v != %+v", back, r)
	}
}

func TestExpiredChainRejected2758(t *testing.T) {
	now := time.Now()
	s := appstoretest.New(t, appstoretest.Options{
		NotBefore: now.AddDate(-2, 0, 0),
		NotAfter:  now.AddDate(-1, 0, 0),
	})
	_, err := verifier2758(s).Verify(s.Sign(t, appstoretest.Monthly(bundle2758, product2758, "2000000758", now)))
	if err == nil {
		t.Fatal("expired leaf accepted")
	}
	if r := reason2758(t, err); r != appstore.ReasonChain {
		t.Fatalf("reason %q", r)
	}
}

func TestOtherCARejected2758(t *testing.T) {
	a := appstoretest.New(t, appstoretest.Options{})
	b := appstoretest.New(t, appstoretest.Options{})
	_, err := verifier2758(b).Verify(a.Sign(t, appstoretest.Monthly(bundle2758, product2758, "2000000758", time.Now())))
	if r := reason2758(t, err); r != appstore.ReasonChain {
		t.Fatalf("reason %q", r)
	}
}
