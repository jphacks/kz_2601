package entitlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/googleplay"
)

// PlatformGooglePlay is Pro.Platform of a Play purchase.
const PlatformGooglePlay = "googlePlay"

// playKeyPrefix marks a Play binding key (Pro.OriginalTransactionID).
const playKeyPrefix = "gp:"

// Errors of SubmitGoogle.
var (
	// ErrPlayNotConfigured: no Play Developer API credentials (503).
	ErrPlayNotConfigured = errors.New("google play is not configured")
	// ErrPlayUnavailable: the Play Developer API failed (503; the client retries).
	ErrPlayUnavailable = errors.New("google play is unavailable")
	// ErrAccountMismatch: the purchase was made for another account (409).
	ErrAccountMismatch = errors.New("purchase was made for another account")
)

// GooglePlay is the Play side of Service.
type GooglePlay struct {
	API    googleplay.API
	Policy googleplay.Policy
}

// DefaultPlayPackageName is the Play package (overridable by env, GooglePlayConfigFromEnv).
const DefaultPlayPackageName = "jp.kz2601.metamon"

// GooglePlayConfigFromEnv reads the Play policy:
//
//	GOOGLE_PLAY_PACKAGE_NAME  the app's package (default jp.kz2601.metamon)
//	GOOGLE_PLAY_PRODUCT_IDS   comma list (default jp.kz2601.metamon.pro.monthly; android ProProducts)
//
// The credentials (GOOGLE_PLAY_SERVICE_ACCOUNT_JSON) are read by cmd/appconfig.
func GooglePlayConfigFromEnv(getenv func(string) string) googleplay.Policy {
	pkg := strings.TrimSpace(getenv("GOOGLE_PLAY_PACKAGE_NAME"))
	if pkg == "" {
		pkg = DefaultPlayPackageName
	}
	return googleplay.Policy{PackageName: pkg, ProductIDs: list(getenv("GOOGLE_PLAY_PRODUCT_IDS"), DefaultProductIDs)}
}

// PlayKey is the binding key of a purchase token: tokens are never stored.
func PlayKey(purchaseToken string) string {
	sum := sha256.Sum256([]byte("googleplay:" + purchaseToken))
	return playKeyPrefix + hex.EncodeToString(sum[:])
}

// FromPlay maps a Play subscription (item = its Pro line item). A state that
// does not entitle (on hold, paused, expired) ends Pro now even when the
// expiry is later. SignedAt is the read time: the API answer is the latest
// state of the purchase.
func FromPlay(sub *googleplay.Subscription, item googleplay.LineItem, key string, now time.Time) Pro {
	env := appstore.EnvProduction
	if sub.Test {
		env = appstore.EnvSandbox
	}
	expires := item.ExpiryTime
	if !sub.Entitles() && expires.After(now) {
		expires = now
	}
	txID := item.OrderID
	if txID == "" {
		txID = key
	}
	willRenew := item.AutoRenew && (sub.State == googleplay.StateActive || sub.State == googleplay.StateInGracePeriod)
	return Pro{
		ProductID:             item.ProductID,
		OriginalTransactionID: key,
		TransactionID:         txID,
		Environment:           env,
		ExpiresAt:             expires,
		WillRenew:             &willRenew,
		UpdatedAt:             now.UTC(),
		SignedAt:              now.UTC(),
		Platform:              PlatformGooglePlay,
		renewalKnown:          true,
	}
}

// SubmitGoogle verifies a Play purchase with the Play Developer API and stores
// it for uid (POST /v1/me/entitlements/google).
//
//   - productId must be allowed and be a line item of the purchase
//   - obfuscatedExternalAccountId, when set, must be AccountToken(uid)
//     (ErrAccountMismatch)
//   - binding: one purchase = one uid, like SubmitApple; a purchase that
//     replaced another (linkedPurchaseToken) cannot be taken from the
//     replaced one's active owner (ErrOwnedByOther)
//   - expired purchases are stored too (isPro false), never replacing newer ones
//   - an unacknowledged purchase that grants Pro is acknowledged here
//     (best effort: the app acknowledges too)
func (s *Service) SubmitGoogle(ctx context.Context, uid, productID, token string) (Status, error) {
	gp := s.GooglePlay
	if gp == nil || gp.API == nil {
		return Status{}, ErrPlayNotConfigured
	}
	if !slices.Contains(gp.Policy.ProductIDs, productID) {
		return Status{}, googleplay.Reject("product_id", "product %q is not allowed", productID)
	}
	sub, err := gp.API.Subscription(ctx, gp.Policy.PackageName, token)
	if errors.Is(err, googleplay.ErrNotFound) {
		return Status{}, googleplay.Reject("not_found", "%v", err)
	}
	if err != nil {
		return Status{}, fmt.Errorf("%w: %w", ErrPlayUnavailable, err)
	}
	item, ok := sub.Item([]string{productID})
	if !ok {
		return Status{}, googleplay.Reject("product_id", "the purchase has no line item %q", productID)
	}
	if item.ExpiryTime.IsZero() {
		return Status{}, googleplay.Reject("pending", "the purchase is not paid yet (%s)", sub.State)
	}
	attrs := []any{slog.String("account", hash(uid)), slog.String("platform", PlatformGooglePlay),
		slog.String("productId", item.ProductID), slog.String("state", sub.State), slog.Bool("test", sub.Test),
		slog.Time("expiresAt", item.ExpiryTime)}
	if sub.ObfuscatedAccountID == "" {
		attrs = append(attrs, slog.Bool("noAccountId", true))
	} else if sub.ObfuscatedAccountID != AccountToken(uid) {
		s.log().WarnContext(ctx, "entitlement: play purchase made for another account", attrs...)
		return Status{}, ErrAccountMismatch
	}

	key := PlayKey(token)
	if sub.LinkedPurchaseToken != "" {
		if err := s.checkLinked(ctx, uid, PlayKey(sub.LinkedPurchaseToken)); err != nil {
			if errors.Is(err, ErrOwnedByOther) {
				s.log().WarnContext(ctx, "entitlement: linked play purchase bound to another account", attrs...)
			}
			return Status{}, err
		}
	}
	owner, err := s.Store.Owner(ctx, key)
	if err != nil {
		return Status{}, fmt.Errorf("owner: %w", err)
	}
	transferFrom, err := s.transferFrom(ctx, owner, uid)
	if err != nil {
		return Status{}, err
	}
	now := s.now()
	p, err := s.Store.Apply(ctx, uid, FromPlay(sub, item, key, now), transferFrom)
	if errors.Is(err, ErrOwnedByOther) {
		s.log().WarnContext(ctx, "entitlement: play purchase bound to another account",
			append(attrs, slog.String("owner", hash(owner)))...)
		return Status{}, err
	}
	if err != nil {
		return Status{}, fmt.Errorf("apply: %w", err)
	}
	if transferFrom != "" {
		attrs = append(attrs, slog.String("transferredFrom", hash(transferFrom)))
	}
	if sub.AcknowledgementState == googleplay.AckPending && sub.Entitles() && item.ExpiryTime.After(now) {
		ackErr := gp.API.Acknowledge(ctx, gp.Policy.PackageName, item.ProductID, token)
		attrs = append(attrs, slog.Bool("acknowledged", ackErr == nil))
		if ackErr != nil {
			attrs = append(attrs, slog.Any("ackError", ackErr))
		}
	}
	st := s.status(p)
	s.log().InfoContext(ctx, "entitlement: play purchase applied", append(attrs, slog.Bool("isPro", st.IsPro))...)
	return st, nil
}

// checkLinked fails with ErrOwnedByOther when the replaced purchase is bound
// to another active uid.
func (s *Service) checkLinked(ctx context.Context, uid, linkedKey string) error {
	owner, err := s.Store.Owner(ctx, linkedKey)
	if err != nil {
		return fmt.Errorf("linked owner: %w", err)
	}
	if owner == "" || owner == uid {
		return nil
	}
	from, err := s.transferFrom(ctx, owner, uid)
	if err != nil {
		return err
	}
	if from == "" {
		return ErrOwnedByOther
	}
	return nil
}
