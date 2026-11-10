package main

import (
	"context"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
)

// overflowBuyer buys the creators' monthly allowance blocks with coins
// (#630 part 3) from the same wallets as appconfig. Only COIN_BILLING_MODE=
// enforce buys; the monthly grant follows the Pro billing period, as on
// appconfig.
func overflowBuyer(cfg coins.Config, c *firestore.Client, envs []string, logger *slog.Logger) *coins.Service {
	ents := entitlement.NewFirestore(c)
	return &coins.Service{
		Config: cfg,
		Store:  coins.NewFirestore(c),
		ProPeriodEnd: func(ctx context.Context, uid string) (time.Time, error) {
			p, err := ents.Get(ctx, uid)
			if err != nil || !p.Active(time.Now(), envs) {
				return time.Time{}, err
			}
			return p.ExpiresAt, nil
		},
		Log: logger,
	}
}
