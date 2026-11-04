package appstore_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
)

func TestVerifyNotificationAndRenewalInfo(t *testing.T) {
	s := appstoretest.New(t, appstoretest.Options{})
	v := &appstore.Verifier{Roots: s.Roots, Policy: appstore.Policy{
		BundleIDs: []string{"b"}, ProductIDs: []string{"p"}, Environments: []string{appstore.EnvSandbox},
	}}
	now := time.Now().Truncate(time.Millisecond)
	tx := appstoretest.Monthly("b", "p", "otid", now)
	r := appstoretest.Renewal(tx)
	r.GracePeriodExpiresDate = now.Add(time.Hour).UnixMilli()
	payload := s.Notification(t, "uuid-1", appstore.NotifyDidFailToRenew, appstore.SubtypeGracePeriod, tx, &r, now)

	n, err := v.VerifyNotification(payload)
	if err != nil {
		t.Fatal(err)
	}
	if n.Type != appstore.NotifyDidFailToRenew || n.Subtype != appstore.SubtypeGracePeriod || n.UUID != "uuid-1" ||
		!n.SignedDate.Equal(now) || n.Environment != appstore.EnvSandbox {
		t.Fatalf("notification %+v", n)
	}
	got, err := v.Verify(n.SignedTransactionInfo)
	if err != nil || got.OriginalTransactionID != "otid" {
		t.Fatalf("tx %+v %v", got, err)
	}
	ri, err := v.VerifyRenewalInfo(n.SignedRenewalInfo)
	if err != nil || !ri.AutoRenew || !ri.GracePeriodExpiresDate.Equal(now.Add(time.Hour)) || ri.OriginalTransactionID != "otid" {
		t.Fatalf("renewal %+v %v", ri, err)
	}

	// production notifications are refused when only Sandbox is allowed
	prod := tx
	prod.Environment = appstore.EnvProduction
	_, err = v.VerifyNotification(s.Notification(t, "uuid-2", appstore.NotifyDidRenew, "", prod, nil, now))
	var verr *appstore.Error
	if !errors.As(err, &verr) || verr.Reason != appstore.ReasonEnvironment {
		t.Fatalf("want environment error, got %v", err)
	}
	// a chain from another CA
	other := appstoretest.New(t, appstoretest.Options{})
	if _, err := v.VerifyNotification(other.Notification(t, "uuid-3", appstore.NotifyRefund, "", tx, nil, now)); !errors.As(err, &verr) || verr.Reason != appstore.ReasonChain {
		t.Fatalf("want chain error, got %v", err)
	}
	if _, err := v.VerifyRenewalInfo(other.Sign(t, r)); !errors.As(err, &verr) || verr.Reason != appstore.ReasonChain {
		t.Fatalf("want chain error, got %v", err)
	}
}
