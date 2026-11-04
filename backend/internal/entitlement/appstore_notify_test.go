package entitlement

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
)

// notifyEnv: alice has submitted subscription otid from the device (bound).
type notifyEnv struct {
	*env
	n     *Notifications
	st    NotifyStore
	alice string
	otid  string
	tx    appstoretest.Transaction
	seq   int
}

func newNotifyEnv(t *testing.T, st Store) *notifyEnv {
	t.Helper()
	e := newEnv(t, st)
	ns := st.(NotifyStore)
	ne := &notifyEnv{env: e, st: ns, alice: uid(t, "alice"), otid: uid(t, "otid")}
	ne.n = &Notifications{Verifier: e.svc.Verifier, Store: ns, Now: func() time.Time { return e.now }}
	ne.tx = appstoretest.Monthly(bundle, product, ne.otid, e.now.Add(-time.Minute))
	ne.tx.AppAccountToken = AccountToken(ne.alice)
	if _, err := e.svc.SubmitApple(context.Background(), ne.alice, e.jws(t, ne.tx)); err != nil {
		t.Fatal(err)
	}
	return ne
}

// uuid is unique per call (the emulator keeps records between runs).
func (ne *notifyEnv) uuid(t *testing.T) string {
	ne.seq++
	return uid(t, "n") + "-" + string(rune('a'+ne.seq))
}

func (ne *notifyEnv) send(t *testing.T, uuid, typ, subtype string, tx appstoretest.Transaction, r *appstoretest.RenewalInfo, at time.Time) string {
	t.Helper()
	out, err := ne.n.Handle(context.Background(), ne.signer.Notification(t, uuid, typ, subtype, tx, r, at))
	if err != nil {
		t.Fatalf("%s: %v", typ, err)
	}
	return out
}

func (ne *notifyEnv) pro(t *testing.T) *Pro {
	t.Helper()
	p, err := ne.st.Get(context.Background(), ne.alice)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// renewed is the next period of tx.
func renewed(tx appstoretest.Transaction, n int, from time.Time) appstoretest.Transaction {
	tx.TransactionID = tx.OriginalTransactionID + "-" + string(rune('1'+n))
	tx.PurchaseDate = from.UnixMilli()
	tx.ExpiresDate = from.AddDate(0, 1, 0).UnixMilli()
	return tx
}

func TestNotificationRenewExtendsPro(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ne := newNotifyEnv(t, st)
		next := renewed(ne.tx, 1, time.UnixMilli(ne.tx.ExpiresDate))
		r := appstoretest.Renewal(next)
		if out := ne.send(t, ne.uuid(t), appstore.NotifyDidRenew, "", next, &r, ne.now); out != OutcomeApplied {
			t.Fatalf("outcome %q", out)
		}
		p := ne.pro(t)
		if p.TransactionID != next.TransactionID || !p.ExpiresAt.Equal(time.UnixMilli(next.ExpiresDate).UTC()) ||
			p.WillRenew == nil || !*p.WillRenew || !p.Active(ne.now, nil) {
			t.Fatalf("pro %+v", p)
		}
	})
}

func TestNotificationRefundRevokesAndReversalRestores(t *testing.T) {
	for _, typ := range []string{appstore.NotifyRefund, appstore.NotifyRevoke} {
		t.Run(typ, func(t *testing.T) {
			stores(t, func(t *testing.T, st Store) {
				ne := newNotifyEnv(t, st)
				refunded := ne.tx
				refunded.RevocationDate = ne.now.UnixMilli()
				ne.send(t, ne.uuid(t), typ, "", refunded, nil, ne.now)
				if p := ne.pro(t); !p.Revoked() || p.Active(ne.now, nil) {
					t.Fatalf("not revoked: %+v", p)
				}
				// the device sends its cached (older) signing of the same transaction
				st2, err := ne.svc.SubmitApple(context.Background(), ne.alice, ne.jws(t, ne.tx))
				if err != nil || st2.IsPro {
					t.Fatalf("stale device transaction restored Pro: %+v %v", st2, err)
				}
				if typ == appstore.NotifyRefund {
					ne.send(t, ne.uuid(t), appstore.NotifyRefundReversed, "", ne.tx, nil, ne.now.Add(time.Hour))
					if p := ne.pro(t); p.Revoked() || !p.Active(ne.now, nil) {
						t.Fatalf("reversal not applied: %+v", p)
					}
				}
			})
		})
	}
}

func TestNotificationGracePeriod(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ne := newNotifyEnv(t, st)
		// the period ended an hour ago and the renewal failed: Apple grants 6 days
		lapsed := ne.tx
		lapsed.ExpiresDate = ne.now.Add(-time.Hour).UnixMilli()
		r := appstoretest.Renewal(lapsed)
		r.GracePeriodExpiresDate = ne.now.Add(6 * 24 * time.Hour).UnixMilli()
		ne.send(t, ne.uuid(t), appstore.NotifyDidFailToRenew, appstore.SubtypeGracePeriod, lapsed, &r, ne.now)
		p := ne.pro(t)
		if !p.Active(ne.now, nil) || !p.GraceExpiresAt.Equal(time.UnixMilli(r.GracePeriodExpiresDate).UTC()) {
			t.Fatalf("grace not granted: %+v", p)
		}
		// the device re-sends the transaction, signed later (no renewal info): grace is kept
		resent := lapsed
		resent.SignedDate = ne.now.Add(time.Minute).UnixMilli()
		if st2, err := ne.svc.SubmitApple(context.Background(), ne.alice, ne.jws(t, resent)); err != nil || !st2.IsPro {
			t.Fatalf("grace lost on device submit: %+v %v", st2, err)
		}
		// grace over
		r.GracePeriodExpiresDate = ne.now.Add(-time.Minute).UnixMilli()
		ne.send(t, ne.uuid(t), appstore.NotifyGracePeriodExpired, "", lapsed, &r, ne.now.Add(time.Hour))
		if p := ne.pro(t); p.Active(ne.now, nil) {
			t.Fatalf("still pro after GRACE_PERIOD_EXPIRED: %+v", p)
		}
	})
}

func TestNotificationExpiredAndRenewalStatus(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ne := newNotifyEnv(t, st)
		r := appstoretest.Renewal(ne.tx)
		r.AutoRenewStatus = 0
		ne.send(t, ne.uuid(t), appstore.NotifyDidChangeRenewalStatus, "AUTO_RENEW_DISABLED", ne.tx, &r, ne.now)
		if p := ne.pro(t); p.WillRenew == nil || *p.WillRenew || !p.Active(ne.now, nil) {
			t.Fatalf("renewal status: %+v", p)
		}
		// the period ran out (the clock is moved past it)
		expired := ne.tx
		ne.now = time.UnixMilli(ne.tx.ExpiresDate).Add(time.Minute)
		ne.send(t, ne.uuid(t), appstore.NotifyExpired, "VOLUNTARY", expired, &r, ne.now)
		if p := ne.pro(t); p.Active(ne.now, nil) || p.WillRenew == nil || *p.WillRenew {
			t.Fatalf("pro after EXPIRED %+v", p)
		}
	})
}

func TestNotificationDuplicateAndOutOfOrder(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ne := newNotifyEnv(t, st)
		ctx := context.Background()
		next := renewed(ne.tx, 1, time.UnixMilli(ne.tx.ExpiresDate))
		id := ne.uuid(t)
		payload := ne.signer.Notification(t, id, appstore.NotifyDidRenew, "", next, nil, ne.now)
		if out, err := ne.n.Handle(ctx, payload); err != nil || out != OutcomeApplied {
			t.Fatalf("first: %q %v", out, err)
		}
		before := ne.pro(t)
		if out, err := ne.n.Handle(ctx, payload); err != nil || out != OutcomeDuplicate {
			t.Fatalf("second: %q %v", out, err)
		}
		if after := ne.pro(t); !sameState(before, after) {
			t.Fatalf("duplicate changed state: %+v -> %+v", before, after)
		}
		// a refund of the previous period signed before the renewal arrives late
		old := ne.tx
		old.RevocationDate = ne.now.Add(-time.Hour).UnixMilli()
		ne.send(t, ne.uuid(t), appstore.NotifyRefund, "", old, nil, ne.now.Add(-time.Hour))
		// and the renewal of the same period signed earlier than the stored one
		r := appstoretest.Renewal(next)
		r.AutoRenewStatus = 0
		ne.send(t, ne.uuid(t), appstore.NotifyDidChangeRenewalStatus, "", next, &r, ne.now.Add(-2*time.Hour))
		if p := ne.pro(t); !sameState(before, p) {
			t.Fatalf("out-of-order notification rolled back: %+v -> %+v", before, p)
		}
	})
}

func TestNotificationUnboundAndIgnored(t *testing.T) {
	stores(t, func(t *testing.T, st Store) {
		ne := newNotifyEnv(t, st)
		other := appstoretest.Monthly(bundle, product, uid(t, "otid-new"), ne.now)
		if out := ne.send(t, ne.uuid(t), appstore.NotifySubscribed, "INITIAL_BUY", other, nil, ne.now); out != OutcomeUnbound {
			t.Fatalf("outcome %q", out)
		}
		otherProduct := ne.tx
		otherProduct.ProductID = "jp.kz2601.metamon.other"
		if out := ne.send(t, ne.uuid(t), appstore.NotifyDidRenew, "", otherProduct, nil, ne.now); out != OutcomeIgnored {
			t.Fatalf("outcome %q", out)
		}
		test := ne.signer.Sign(t, appstoretest.NotificationPayload{NotificationType: appstore.NotifyTest,
			NotificationUUID: ne.uuid(t), Version: "2.0", SignedDate: ne.now.UnixMilli(),
			Data: appstoretest.NotificationData{BundleID: bundle, Environment: "Sandbox"}})
		if out, err := ne.n.Handle(context.Background(), test); err != nil || out != OutcomeIgnored {
			t.Fatalf("TEST: %q %v", out, err)
		}
	})
}

func TestNotificationRejectsForgedPayloads(t *testing.T) {
	ne := newNotifyEnv(t, NewMemory())
	forger := appstoretest.New(t, appstoretest.Options{}) // another CA
	refunded := ne.tx
	refunded.RevocationDate = ne.now.UnixMilli()
	cases := map[string]string{
		"other CA":     forger.Notification(t, ne.uuid(t), appstore.NotifyRefund, "", refunded, nil, ne.now),
		"other bundle": ne.signer.Sign(t, appstoretest.NotificationPayload{NotificationType: appstore.NotifyTest, NotificationUUID: "x", Version: "2.0", SignedDate: ne.now.UnixMilli(), Data: appstoretest.NotificationData{BundleID: "com.example", Environment: "Sandbox"}}),
		"no uuid":      ne.signer.Sign(t, appstoretest.NotificationPayload{NotificationType: appstore.NotifyTest, Version: "2.0", SignedDate: ne.now.UnixMilli(), Data: appstoretest.NotificationData{BundleID: bundle, Environment: "Sandbox"}}),
		"v1 payload":   ne.signer.Sign(t, appstoretest.NotificationPayload{NotificationType: appstore.NotifyTest, NotificationUUID: "y", Version: "1.0", SignedDate: ne.now.UnixMilli(), Data: appstoretest.NotificationData{BundleID: bundle, Environment: "Sandbox"}}),
		"inner forged": func() string {
			// genuine envelope around a transaction signed by another CA
			return ne.signer.Sign(t, appstoretest.NotificationPayload{NotificationType: appstore.NotifyRefund, NotificationUUID: ne.uuid(t), Version: "2.0",
				SignedDate: ne.now.UnixMilli(), Data: appstoretest.NotificationData{BundleID: bundle, Environment: "Sandbox", SignedTransactionInfo: forger.Sign(t, refunded)}})
		}(),
		"env mismatch": func() string {
			return ne.signer.Sign(t, appstoretest.NotificationPayload{NotificationType: appstore.NotifyRefund, NotificationUUID: ne.uuid(t), Version: "2.0",
				SignedDate: ne.now.UnixMilli(), Data: appstoretest.NotificationData{BundleID: bundle, Environment: "Production", SignedTransactionInfo: ne.signer.Sign(t, refunded)}})
		}(),
		"tampered": func() string {
			s := ne.signer.Notification(t, ne.uuid(t), appstore.NotifyRefund, "", refunded, nil, ne.now)
			parts := strings.Split(s, ".")
			return parts[0] + "." + parts[1] + "x." + parts[2]
		}(),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ne.n.Handle(context.Background(), payload)
			var verr *appstore.Error
			if !errors.As(err, &verr) {
				t.Fatalf("want *appstore.Error, got %v", err)
			}
		})
	}
	if p := ne.pro(t); p.Revoked() || !p.Active(ne.now, nil) {
		t.Fatalf("forged notification changed Pro: %+v", p)
	}
}

func TestNotificationsHTTP(t *testing.T) {
	ne := newNotifyEnv(t, NewMemory())
	mux := http.NewServeMux()
	RegisterNotifications(mux, ne.n)
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, NotificationsPath, strings.NewReader(body)))
		return w
	}
	next := renewed(ne.tx, 1, time.UnixMilli(ne.tx.ExpiresDate))
	ok := ne.signer.Notification(t, ne.uuid(t), appstore.NotifyDidRenew, "", next, nil, ne.now)
	if w := post(`{"signedPayload":"` + ok + `"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), OutcomeApplied) {
		t.Fatalf("ok: %d %s", w.Code, w.Body)
	}
	if w := post(`{"signedPayload":"` + ok + `"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), OutcomeDuplicate) {
		t.Fatalf("duplicate: %d %s", w.Code, w.Body)
	}
	if w := post(`{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("empty: %d", w.Code)
	}
	if w := post(`{"signedPayload":"a.b.c"}`); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "invalid_transaction") {
		t.Fatalf("bad: %d %s", w.Code, w.Body)
	}
}
