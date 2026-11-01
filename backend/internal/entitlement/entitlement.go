// Package entitlement keeps the server-side record of Tailor It Pro (#314).
//
// iOS sends the StoreKit 2 signed transaction (JWS) after a purchase / restore
// and on launch; Service verifies it locally (internal/appstore) and stores
//
//	users/{uid}/entitlements/pro             Pro (productId, originalTransactionId, expiresAt, environment, ...)
//	appstore_transactions/{originalTxId}     {uid, productId, environment, boundAt, updatedAt}
//
// The second document binds one App Store subscription to one uid (see
// Service.SubmitApple for the policy). The ai rate limiter reads the first one
// through ProChecker (#449).
//
// Seam for #315 (App Store Server Notifications v2 / Server API): a notification
// carries the same JWSTransaction; look up the uid by originalTransactionId
// (Store.Owner) and call Store.Apply with the verified transaction.
package entitlement

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
)

// Pro is users/{uid}/entitlements/pro.
type Pro struct {
	ProductID             string    `json:"productId" firestore:"productId"`
	OriginalTransactionID string    `json:"originalTransactionId" firestore:"originalTransactionId"`
	TransactionID         string    `json:"transactionId" firestore:"transactionId"`
	Environment           string    `json:"environment" firestore:"environment"`
	ExpiresAt             time.Time `json:"expiresAt" firestore:"expiresAt"`
	RevokedAt             time.Time `json:"revokedAt,omitzero" firestore:"revokedAt,omitempty"`
	UpdatedAt             time.Time `json:"updatedAt" firestore:"updatedAt"`
}

// Revoked reports a refunded / revoked transaction.
func (p *Pro) Revoked() bool { return !p.RevokedAt.IsZero() }

// Active reports whether p grants Pro at now. envs restricts the accepted
// App Store environments (empty = any).
func (p *Pro) Active(now time.Time, envs []string) bool {
	if p == nil || p.Revoked() || !p.ExpiresAt.After(now) {
		return false
	}
	return len(envs) == 0 || slices.Contains(envs, p.Environment)
}

// FromTransaction maps a verified transaction.
func FromTransaction(t *appstore.Transaction, now time.Time) Pro {
	return Pro{
		ProductID:             t.ProductID,
		OriginalTransactionID: t.OriginalTransactionID,
		TransactionID:         t.TransactionID,
		Environment:           t.Environment,
		ExpiresAt:             t.ExpiresDate,
		RevokedAt:             t.RevocationDate,
		UpdatedAt:             now.UTC(),
	}
}

// merge decides what to store when in arrives while cur is stored (cur may be
// nil). Transactions can arrive out of order (an old one from another device,
// a cached launch-time one), so an older transaction never replaces a newer one:
//
//   - nothing stored, or the same transaction again (revocation may have changed)
//   - a later expiry (renewal, upgrade, new subscription)
//   - cur no longer grants Pro but in does
//   - same expiry and subscription, now revoked
func merge(cur *Pro, in Pro, now time.Time) (Pro, bool) {
	switch {
	case cur == nil,
		in.TransactionID == cur.TransactionID,
		in.ExpiresAt.After(cur.ExpiresAt),
		!cur.Active(now, nil) && in.Active(now, nil),
		in.ExpiresAt.Equal(cur.ExpiresAt) && in.OriginalTransactionID == cur.OriginalTransactionID && in.Revoked():
		return in, true
	}
	return *cur, false
}

// Errors.
var (
	// ErrOwnedByOther: the subscription is bound to another active uid.
	ErrOwnedByOther = errors.New("transaction is bound to another account")
)

// Store persists entitlements and transaction bindings.
type Store interface {
	// Get returns uid's Pro document, or nil when there is none.
	Get(ctx context.Context, uid string) (*Pro, error)
	// Owner returns the uid originalTransactionID is bound to ("" when unbound).
	Owner(ctx context.Context, originalTransactionID string) (string, error)
	// Apply binds p.OriginalTransactionID to uid and stores p when merge
	// accepts it, atomically. The binding may move away from transferFrom
	// (a deleted account); any other owner yields ErrOwnedByOther.
	// It returns what is stored afterwards.
	Apply(ctx context.Context, uid string, p Pro, transferFrom string) (*Pro, error)
	// PurgeUser deletes uid's entitlement documents and bindings (cmd/purge).
	PurgeUser(ctx context.Context, uid string) (int, error)
}

// owns is the binding rule shared by the stores.
func owns(owner, uid, transferFrom string) bool {
	return owner == "" || owner == uid || (transferFrom != "" && owner == transferFrom)
}

// AccountToken is the StoreKit appAccountToken iOS sets for uid: a UUIDv8
// from SHA-256 (Firebase uids are not UUIDs). ios: ProAccountToken.swift.
func AccountToken(uid string) string {
	b := sha256.Sum256([]byte("tailorit:appAccountToken:" + uid))
	b[6] = b[6]&0x0f | 0x80 // version 8
	b[8] = b[8]&0x3f | 0x80 // RFC 9562 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Default App Store policy (overridable by env, ConfigFromEnv).
var (
	DefaultBundleIDs    = []string{"jp.kz2601.metamon"}
	DefaultProductIDs   = []string{"jp.kz2601.metamon.pro.monthly"}
	DefaultEnvironments = []string{appstore.EnvSandbox, appstore.EnvProduction}
)

// ConfigFromEnv reads the App Store policy:
//
//	APPSTORE_BUNDLE_IDS    comma list (default jp.kz2601.metamon)
//	APPSTORE_PRODUCT_IDS   comma list (default jp.kz2601.metamon.pro.monthly; ios ProProducts.swift)
//	APPSTORE_ENVIRONMENTS  Sandbox,Production (default both: App Review and
//	                       TestFlight buy in Sandbox against the production server)
func ConfigFromEnv(getenv func(string) string) (appstore.Policy, error) {
	p := appstore.Policy{
		BundleIDs:    list(getenv("APPSTORE_BUNDLE_IDS"), DefaultBundleIDs),
		ProductIDs:   list(getenv("APPSTORE_PRODUCT_IDS"), DefaultProductIDs),
		Environments: list(getenv("APPSTORE_ENVIRONMENTS"), DefaultEnvironments),
	}
	for _, e := range p.Environments {
		if e != appstore.EnvSandbox && e != appstore.EnvProduction {
			return p, fmt.Errorf("APPSTORE_ENVIRONMENTS: unknown environment %q", e)
		}
	}
	return p, nil
}

func list(raw string, def []string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return slices.Clone(def)
	}
	return out
}
