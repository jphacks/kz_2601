package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// newCoins is the coin wallet (#630) behind COIN_BILLING_MODE (default off:
// nothing is read or written). Packs are verified with the same bundle ids
// and environments as Pro (Xcode is never accepted) but only the COIN_PACKS
// product ids; the monthly grant follows the Pro billing period.
func newCoins(st store.Store, ents *entitlement.Service, logger *slog.Logger) *coins.Service {
	cfg, err := coins.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("coin config", slog.Any("error", err))
		os.Exit(1)
	}
	var cs coins.Store = coins.NewMemory()
	var counters ratelimit.Peeker
	if fs, ok := st.(*store.Firestore); ok {
		cs = coins.NewFirestore(fs.Client())
		// the runner's monthly allowance counters (same RATE_LIMIT_COLLECTION and APP_ENV as the runner)
		counters = ratelimit.NewFirestore(fs.Client(), os.Getenv("RATE_LIMIT_COLLECTION"))
	}
	logger.Info("coin billing", slog.String("mode", string(cfg.Mode)), slog.Int("packs", len(cfg.Packs)))
	policy := ents.Verifier.Policy
	policy.ProductIDs = cfg.PackIDs()
	return &coins.Service{
		Config:       cfg,
		Store:        cs,
		Verifier:     &appstore.Verifier{Roots: ents.Verifier.Roots, Policy: policy},
		ProPeriodEnd: proPeriodEnd(ents),
		Log:          logger,
		Counters:     counters,
		Env:          os.Getenv("APP_ENV"),
	}
}

// proPeriodEnd is the active Pro subscription's expiresAt (zero when not Pro).
func proPeriodEnd(ents *entitlement.Service) func(ctx context.Context, uid string) (time.Time, error) {
	return func(ctx context.Context, uid string) (time.Time, error) {
		st, err := ents.Get(ctx, uid)
		if err != nil || !st.IsPro || st.Pro == nil {
			return time.Time{}, err
		}
		return st.Pro.ExpiresAt, nil
	}
}
