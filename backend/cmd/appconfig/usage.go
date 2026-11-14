package main

import (
	"log/slog"
	"os"

	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// usage lets GET /v1/me/usage read today's AI units from the counters the ai
// service writes (same Firestore, RATE_LIMIT_COLLECTION, APP_ENV). The limits
// are ai's RATE_LIMIT_*: set them on both services (docs/infra/runtime-config.md).
// A bad value stops the start, as on ai. RATE_LIMIT_STORE=memory or the
// in-memory store: the units are per ai instance, so "ai" is not answered.
func usage(st store.Store, logger *slog.Logger) appconfig.Option {
	cfg, err := ratelimit.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("usage: rate limits", slog.Any("error", err))
		os.Exit(1)
	}
	c := appconfig.UsageConfig{Rate: cfg}
	if fs, ok := st.(*store.Firestore); ok && os.Getenv("RATE_LIMIT_STORE") != "memory" {
		c.Counters = ratelimit.NewFirestore(fs.Client(), os.Getenv("RATE_LIMIT_COLLECTION"))
	}
	logger.Info("usage", slog.Bool("aiUnits", c.Counters != nil), slog.Any("limits", cfg.Limits))
	return appconfig.WithUsage(c)
}
