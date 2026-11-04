package entitlement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
)

// StatusAPI is the App Store Server API call reconciliation needs
// (*appstore.ServerAPI; a fake in tests).
type StatusAPI interface {
	SubscriptionStatuses(ctx context.Context, environment, transactionID string) ([]appstore.SubscriptionStatus, error)
}

// Reconciler re-reads subscription states from the App Store Server API so a
// notification Apple could not deliver (outage, 5xx past its retries) still
// reaches the stored Pro (#315, cmd/appstore-reconcile). It goes through every
// binding whose uid's Pro is that subscription and ended less than Window ago
// (or has not ended), and merges the latest transaction like a notification.
type Reconciler struct {
	Verifier *appstore.Verifier
	Store    NotifyStore
	API      StatusAPI
	// Window: Pro that ended longer ago is not checked (default 35 days:
	// a refund or a late renewal after a short lapse is still picked up).
	Window time.Duration
	Now    func() time.Time
	Log    *slog.Logger
}

// ReconcileResult counts one run.
type ReconcileResult struct {
	Bindings int // bindings read
	Checked  int // Server API calls
	Updated  int // stored Pro changed (would change, with dryRun)
	Failed   int // API / verify / store errors (the next run retries)
}

const reconcilePage = 200

func (r *Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Reconciler) log() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

// Run checks at most limit subscriptions. With dryRun nothing is written.
// The error is only for failing to list bindings; per-subscription failures
// are counted in Failed.
func (r *Reconciler) Run(ctx context.Context, limit int, dryRun bool) (ReconcileResult, error) {
	var res ReconcileResult
	window := r.Window
	if window <= 0 {
		window = 35 * 24 * time.Hour
	}
	after := ""
	for res.Checked < limit {
		page, err := r.Store.Bindings(ctx, after, reconcilePage)
		if err != nil {
			return res, fmt.Errorf("bindings: %w", err)
		}
		for _, b := range page {
			if res.Checked >= limit {
				break
			}
			res.Bindings++
			checked, updated, err := r.one(ctx, b, window, dryRun)
			if checked {
				res.Checked++
			}
			if updated {
				res.Updated++
			}
			if err != nil {
				res.Failed++
				r.log().WarnContext(ctx, "appstore reconcile: failed", slog.String("account", hash(b.UID)), slog.Any("error", err))
			}
		}
		if len(page) < reconcilePage {
			break
		}
		after = page[len(page)-1].OriginalTransactionID
	}
	return res, nil
}

func (r *Reconciler) one(ctx context.Context, b Binding, window time.Duration, dryRun bool) (checked, updated bool, err error) {
	if b.UID == "" {
		return false, false, nil
	}
	cur, err := r.Store.Get(ctx, b.UID)
	if err != nil {
		return false, false, err
	}
	now := r.now()
	if cur == nil || cur.OriginalTransactionID != b.OriginalTransactionID || cur.Until().Before(now.Add(-window)) {
		return false, false, nil
	}
	statuses, err := r.API.SubscriptionStatuses(ctx, cur.Environment, cur.TransactionID)
	if errors.Is(err, appstore.ErrNotFound) {
		r.log().InfoContext(ctx, "appstore reconcile: not found", slog.String("account", hash(b.UID)), slog.String("environment", cur.Environment))
		return true, false, nil
	}
	if err != nil {
		return true, false, err
	}
	for _, st := range statuses {
		if st.OriginalTransactionID != b.OriginalTransactionID {
			continue
		}
		tx, renewal, err := verifyPair(r.Verifier, st.SignedTransactionInfo, st.SignedRenewalInfo)
		if err != nil {
			return true, false, err
		}
		in := FromTransaction(tx, now).WithRenewal(renewal)
		next, write := merge(cur, in, now)
		if !write || sameState(cur, &next) {
			continue
		}
		if !dryRun {
			stored, err := r.Store.Apply(ctx, b.UID, in, "")
			if err != nil {
				return true, false, err
			}
			next = *stored
		}
		r.log().InfoContext(ctx, "appstore reconcile: updated", slog.String("account", hash(b.UID)),
			slog.Bool("dryRun", dryRun), slog.Bool("revoked", next.Revoked()), slog.Time("proUntil", next.Until()))
		cur, updated = &next, true
	}
	return true, updated, nil
}

// sameState: a and b grant the same thing (UpdatedAt / SignedAt aside).
func sameState(a, b *Pro) bool {
	willRenew := func(p *Pro) int {
		switch {
		case p.WillRenew == nil:
			return -1
		case *p.WillRenew:
			return 1
		}
		return 0
	}
	return a.TransactionID == b.TransactionID && a.ExpiresAt.Equal(b.ExpiresAt) &&
		a.RevokedAt.Equal(b.RevokedAt) && a.GraceExpiresAt.Equal(b.GraceExpiresAt) &&
		willRenew(a) == willRenew(b)
}
