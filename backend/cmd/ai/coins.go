package main

import (
	"context"
	"log"
	"os"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
)

// newCoins is the coin charge of the AI calls (#630) behind
// COIN_BILLING_MODE (default off: nil, nothing is read or written). The
// wallets are the ones appconfig serves (GET /v1/me/coins); the ai only
// holds, charges and releases. A run without Firestore keeps them in memory.
func newCoins(client *firestore.Client) *coins.Service {
	cfg, err := coins.ConfigFromEnv(os.Getenv)
	if err == nil {
		err = cfg.CheckReserves() // the ai holds: a 0 hold would never charge
	}
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	log.Printf("ai: coin billing mode=%s", cfg.Mode)
	if cfg.Mode == coins.ModeOff {
		return nil
	}
	var st coins.Store = coins.NewMemory()
	var ents *entitlement.Firestore
	if client != nil {
		st = coins.NewFirestore(client)
		ents = entitlement.NewFirestore(client)
	}
	policy, err := entitlement.ConfigFromEnv(os.Getenv)
	if err != nil {
		log.Fatalf("ai: %v", err)
	}
	return &coins.Service{Config: cfg, Store: st, ProPeriodEnd: proPeriodEnd(ents, policy.Environments)}
}

// proPeriodEnd is the active Pro subscription's expiresAt (zero when not
// Pro): the monthly grant follows it. nil without Firestore (nobody is Pro).
func proPeriodEnd(ents *entitlement.Firestore, envs []string) func(ctx context.Context, uid string) (time.Time, error) {
	if ents == nil {
		return nil
	}
	return func(ctx context.Context, uid string) (time.Time, error) {
		p, err := ents.Get(ctx, uid)
		if err != nil || !p.Active(time.Now(), envs) {
			return time.Time{}, err
		}
		return p.ExpiresAt, nil
	}
}
