package entitlement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
)

// Service verifies and stores App Store transactions.
type Service struct {
	Verifier *appstore.Verifier
	Store    Store
	// Deleted reports a soft-deleted / purged account (store.Accounts.UserDeleted).
	// nil = never (bindings never move).
	Deleted func(ctx context.Context, uid string) (bool, error)
	// Environments accepted for "is Pro" (nil = Verifier.Environments).
	Environments []string
	Now          func() time.Time
	Log          *slog.Logger
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func (s *Service) envs() []string {
	if s.Environments != nil {
		return s.Environments
	}
	return s.Verifier.Environments
}

// Status is the API view of a uid's entitlements (GET /v1/me/entitlements).
type Status struct {
	IsPro bool `json:"isPro"`
	Pro   *Pro `json:"pro"`
}

func (s *Service) status(p *Pro) Status {
	return Status{IsPro: p.Active(s.now(), s.envs()), Pro: p}
}

// Get returns uid's current status.
func (s *Service) Get(ctx context.Context, uid string) (Status, error) {
	p, err := s.Store.Get(ctx, uid)
	if err != nil {
		return Status{}, err
	}
	return s.status(p), nil
}

// SubmitApple verifies signedJWS and stores it for uid.
//
// Binding policy (one App Store subscription = one uid):
//   - unbound: bound to uid
//   - bound to uid: updated
//   - bound to a deleted account (DELETE /v1/me or purged): moved to uid
//     (the same Apple ID signed up again)
//   - bound to another active uid: ErrOwnedByOther (409); logged with uid
//     hashes. Support can release it by deleting appstore_transactions/{id}.
//
// Expired / revoked transactions are stored too (the server learns about
// refunds) but do not replace a newer one (merge).
func (s *Service) SubmitApple(ctx context.Context, uid, signedJWS string) (Status, error) {
	tx, err := s.Verifier.Verify(signedJWS)
	if err != nil {
		return Status{}, err
	}
	owner, err := s.Store.Owner(ctx, tx.OriginalTransactionID)
	if err != nil {
		return Status{}, fmt.Errorf("owner: %w", err)
	}
	transferFrom := ""
	if owner != "" && owner != uid && s.Deleted != nil {
		gone, err := s.Deleted(ctx, owner)
		if err != nil {
			return Status{}, fmt.Errorf("owner status: %w", err)
		}
		if gone {
			transferFrom = owner
		}
	}
	attrs := []any{slog.String("account", hash(uid)), slog.String("environment", tx.Environment),
		slog.String("productId", tx.ProductID), slog.Bool("revoked", tx.Revoked()),
		slog.Time("expiresAt", tx.ExpiresDate)}
	if tx.AppAccountToken != "" && tx.AppAccountToken != AccountToken(uid) {
		// bought while signed in as another uid (or before a sign-in switch); informational
		attrs = append(attrs, slog.Bool("appAccountTokenMismatch", true))
	}
	p, err := s.Store.Apply(ctx, uid, FromTransaction(tx, s.now()), transferFrom)
	if errors.Is(err, ErrOwnedByOther) {
		s.log().WarnContext(ctx, "entitlement: transaction bound to another account",
			append(attrs, slog.String("owner", hash(owner)))...)
		return Status{}, err
	}
	if err != nil {
		return Status{}, fmt.Errorf("apply: %w", err)
	}
	if transferFrom != "" {
		attrs = append(attrs, slog.String("transferredFrom", hash(transferFrom)))
	}
	st := s.status(p)
	s.log().InfoContext(ctx, "entitlement: apple transaction applied", append(attrs, slog.Bool("isPro", st.IsPro))...)
	return st, nil
}

// hash keeps uids out of logs while letting lines be correlated.
func hash(uid string) string {
	sum := sha256.Sum256([]byte("entitlement:" + uid))
	return hex.EncodeToString(sum[:6])
}
