package main

import (
	"log/slog"
	"os"

	"github.com/jphacks/kz_2601/backend/internal/plan"
)

// planLimits reads the per-plan limits (#317): PLAN_* and PLAN_LIMITS_MODE.
// A bad value stops the start (a typo must not lift a limit silently).
func planLimits(logger *slog.Logger) plan.Table {
	t, err := plan.FromEnv(os.Getenv)
	if err != nil {
		logger.Error("plan limits", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("plan limits", slog.String("mode", string(t.Mode)), slog.Any("free", t.Free), slog.Any("pro", t.Pro))
	return t
}
