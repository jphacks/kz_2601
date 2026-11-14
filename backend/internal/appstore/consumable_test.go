package appstore_test

import (
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
)

func TestVerifyConsumable(t *testing.T) {
	s := appstoretest.New(t, appstoretest.Options{})
	now := time.Now()
	v := verifier(s)
	pack := "jp.kz2601.metamon.coins.test"
	v.ProductIDs = []string{pack, product}

	tx, err := v.VerifyConsumable(s.Sign(t, appstoretest.Consumable(bundle, pack, "c1", now)))
	if err != nil || tx.TransactionID != "c1" || tx.ProductID != pack {
		t.Fatalf("valid consumable: %+v %v", tx, err)
	}
	// the subscription check still rejects a consumable
	if _, err := v.Verify(s.Sign(t, appstoretest.Consumable(bundle, pack, "c1", now))); reason(t, err) != appstore.ReasonNotSubscription {
		t.Fatalf("Verify(consumable) = %v", err)
	}
	revoked := appstoretest.Consumable(bundle, pack, "c2", now)
	revoked.RevocationDate = now.UnixMilli()
	xcode := appstoretest.Consumable(bundle, pack, "c3", now)
	xcode.Environment = "Xcode"
	cases := map[string]struct {
		tx   appstoretest.Transaction
		want string
	}{
		"subscription":  {appstoretest.Monthly(bundle, product, "o1", now), appstore.ReasonNotConsumable},
		"revoked":       {revoked, appstore.ReasonRevoked},
		"xcode":         {xcode, appstore.ReasonEnvironment},
		"other product": {appstoretest.Consumable(bundle, "other", "c4", now), appstore.ReasonProduct},
	}
	for name, c := range cases {
		if _, err := v.VerifyConsumable(s.Sign(t, c.tx)); reason(t, err) != c.want {
			t.Errorf("%s: got %v, want %s", name, err, c.want)
		}
	}
}
