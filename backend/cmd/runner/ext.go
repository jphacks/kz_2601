package main

import (
	"context"
	"log/slog"
	"strings"

	"cloud.google.com/go/firestore"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
)

// extFromEnv builds the external API door (#1493,
// docs/backend/features/external-apis.md). Every provider is off unless
// EXT_<PROVIDER>_ENABLED; tokens are sealed with Cloud KMS
// (EXT_TOKEN_KMS_KEY), or with a random in-memory key on the emulator.
// Without either, OAuth providers stay off.
func extFromEnv(ctx context.Context, getenv func(string) string, q *quota.Quota, biller *coins.Service, c *firestore.Client, logger *slog.Logger) (*extapi.Service, error) {
	cfg, err := extapi.ConfigFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	var sealer extapi.Sealer
	switch key := strings.TrimSpace(getenv("EXT_TOKEN_KMS_KEY")); {
	case key != "":
		if sealer, err = extapi.NewKMSSealer(ctx, key); err != nil {
			return nil, err
		}
	case strings.TrimSpace(getenv("FIRESTORE_EMULATOR_HOST")) != "":
		if sealer, err = extapi.NewLocalSealer(nil); err != nil {
			return nil, err
		}
	default:
		logger.Info("runner: EXT_TOKEN_KMS_KEY not set: OAuth providers are off")
	}
	store := extapi.NewFirestore(c)
	// without a sealer no token is read or written; cutting and expiry
	// still delete what is there
	conns := &extapi.Connections{Store: store, Sealer: sealer, UnusedTTL: cfg.UnusedTTL, Log: logger}
	svc := extapi.New(cfg, q, biller, conns, store, logger)
	// the call limits (#1715) share the rateLimits counters across instances
	svc.UseLimitStore(ratelimit.NewFirestore(c, getenv("RATE_LIMIT_COLLECTION")))
	return svc, nil
}
