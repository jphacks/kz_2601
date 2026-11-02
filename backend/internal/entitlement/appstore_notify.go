package entitlement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
)

// App Store Server Notifications v2 (#315). Apple POSTs every subscription
// change (renewal, failed renewal and grace period, expiry, auto-renew
// switch, refund, revoke) as a signed payload carrying the same
// JWSTransaction as POST /v1/me/entitlements/apple, plus the JWSRenewalInfo.
// The uid comes from the transaction binding (Store.Owner): appAccountToken is
// a one-way hash of the uid, so it is only cross-checked in logs.
//
//	appstore_notifications/{notificationUUID}  NotificationRecord (dedupe)

// Outcomes of a notification (logged, stored and returned to Apple).
const (
	OutcomeApplied   = "applied"   // the transaction was merged into the uid's Pro
	OutcomeDuplicate = "duplicate" // notificationUUID already handled
	OutcomeUnbound   = "unbound"   // no uid has submitted this subscription yet
	OutcomeIgnored   = "ignored"   // nothing to store (TEST, another product, no transaction)
)

// NotificationRetention is how long dedupe records are kept (expireAt; a
// Firestore TTL policy on appstore_notifications.expireAt deletes them).
// Apple retries a notification for about 3 days.
const NotificationRetention = 30 * 24 * time.Hour

// NotificationRecord is appstore_notifications/{UUID}.
type NotificationRecord struct {
	UUID                  string
	Type                  string
	Subtype               string
	Environment           string
	OriginalTransactionID string
	Outcome               string
	SignedAt              time.Time
	ProcessedAt           time.Time
	ExpireAt              time.Time
}

// Binding is appstore_transactions/{OriginalTransactionID}.
type Binding struct {
	OriginalTransactionID string
	UID                   string
}

// NotifyStore is Store plus what notifications and reconciliation need.
type NotifyStore interface {
	Store
	// NotificationSeen reports whether uuid was recorded.
	NotificationSeen(ctx context.Context, uuid string) (bool, error)
	// RecordNotification stores rec; recording an existing UUID is not an error.
	RecordNotification(ctx context.Context, rec NotificationRecord) error
	// Bindings lists bindings ordered by OriginalTransactionID, after the given one.
	Bindings(ctx context.Context, after string, limit int) ([]Binding, error)
}

// Notifications handles App Store Server Notifications v2.
type Notifications struct {
	Verifier *appstore.Verifier
	Store    NotifyStore
	Now      func() time.Time
	Log      *slog.Logger
}

func (n *Notifications) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

func (n *Notifications) log() *slog.Logger {
	if n.Log != nil {
		return n.Log
	}
	return slog.Default()
}

// Handle verifies signedPayload and applies it once. A rejected payload is
// an *appstore.Error; other errors are transient (Apple retries on non-200).
//
// Order: Apple does not guarantee delivery order. The stored Pro only moves
// forward (merge): a notification whose transaction was signed before the
// stored one, or that expires earlier, does not roll the entitlement back.
func (n *Notifications) Handle(ctx context.Context, signedPayload string) (string, error) {
	note, err := n.Verifier.VerifyNotification(signedPayload)
	if err != nil {
		return "", err
	}
	attrs := []any{slog.String("notificationType", note.Type), slog.String("subtype", note.Subtype),
		slog.String("notificationUUID", note.UUID), slog.String("environment", note.Environment)}
	seen, err := n.Store.NotificationSeen(ctx, note.UUID)
	if err != nil {
		return "", fmt.Errorf("notification seen: %w", err)
	}
	if seen {
		n.log().InfoContext(ctx, "appstore notification: duplicate", attrs...)
		return OutcomeDuplicate, nil
	}

	rec := NotificationRecord{UUID: note.UUID, Type: note.Type, Subtype: note.Subtype,
		Environment: note.Environment, SignedAt: note.SignedDate}
	rec.Outcome, rec.OriginalTransactionID, err = n.apply(ctx, note, attrs)
	if err != nil {
		return "", err
	}
	rec.ProcessedAt = n.now().UTC()
	rec.ExpireAt = rec.ProcessedAt.Add(NotificationRetention)
	if err := n.Store.RecordNotification(ctx, rec); err != nil {
		// applying again is harmless (merge), so let Apple retry
		return "", fmt.Errorf("record notification: %w", err)
	}
	return rec.Outcome, nil
}

func (n *Notifications) apply(ctx context.Context, note *appstore.Notification, attrs []any) (outcome, otid string, err error) {
	if note.SignedTransactionInfo == "" {
		n.log().InfoContext(ctx, "appstore notification: nothing to apply", attrs...)
		return OutcomeIgnored, "", nil
	}
	tx, renewal, err := verifyPair(n.Verifier, note.SignedTransactionInfo, note.SignedRenewalInfo)
	var verr *appstore.Error
	if errors.As(err, &verr) && (verr.Reason == appstore.ReasonProduct || verr.Reason == appstore.ReasonNotSubscription) {
		n.log().InfoContext(ctx, "appstore notification: not a Pro subscription", append(attrs, slog.String("reason", verr.Reason))...)
		return OutcomeIgnored, "", nil
	}
	if err != nil {
		return "", "", err
	}
	if tx.Environment != note.Environment {
		return "", "", &appstore.Error{Reason: appstore.ReasonEnvironment,
			Err: fmt.Errorf("transaction environment %q in a %q notification", tx.Environment, note.Environment)}
	}
	otid = tx.OriginalTransactionID
	attrs = append(attrs, slog.String("productId", tx.ProductID), slog.Bool("revoked", tx.Revoked()),
		slog.Time("expiresAt", tx.ExpiresDate))
	owner, err := n.Store.Owner(ctx, otid)
	if err != nil {
		return "", "", fmt.Errorf("owner: %w", err)
	}
	if owner == "" {
		// iOS submits the transaction after the purchase; that binds it
		n.log().InfoContext(ctx, "appstore notification: subscription not bound to an account yet", attrs...)
		return OutcomeUnbound, otid, nil
	}
	attrs = append(attrs, slog.String("account", hash(owner)))
	if tx.AppAccountToken != "" && tx.AppAccountToken != AccountToken(owner) {
		attrs = append(attrs, slog.Bool("appAccountTokenMismatch", true))
	}
	p, err := n.Store.Apply(ctx, owner, FromTransaction(tx, n.now()).WithRenewal(renewal), "")
	if err != nil {
		return "", "", fmt.Errorf("apply: %w", err)
	}
	n.log().InfoContext(ctx, "appstore notification: applied", append(attrs,
		slog.Bool("isPro", p.Active(n.now(), n.Verifier.Environments)),
		slog.Time("proUntil", p.Until()), slog.Bool("storedTransaction", p.TransactionID == tx.TransactionID))...)
	return OutcomeApplied, otid, nil
}

// verifyPair verifies a transaction and its (optional) renewal info.
func verifyPair(v *appstore.Verifier, signedTx, signedRenewal string) (*appstore.Transaction, *appstore.RenewalInfo, error) {
	tx, err := v.Verify(signedTx)
	if err != nil {
		return nil, nil, err
	}
	if signedRenewal == "" {
		return tx, nil, nil
	}
	r, err := v.VerifyRenewalInfo(signedRenewal)
	if err != nil {
		return nil, nil, err
	}
	if r.OriginalTransactionID != tx.OriginalTransactionID {
		return nil, nil, &appstore.Error{Reason: appstore.ReasonMalformed, Err: errors.New("renewal info of another subscription")}
	}
	return tx, r, nil
}
