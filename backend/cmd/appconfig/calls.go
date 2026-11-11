package main

import (
	"log/slog"
	"os"

	"github.com/jphacks/kz_2601/backend/internal/calls"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// newCalls is the voice / video calls (#447) behind CALLS_ENABLED (default
// off: no routes). The LiveKit keys come from env (Secret Manager); the
// minutes are charged to the caller's own coins through cs.
func newCalls(st store.Store, cs *coins.Service, logger *slog.Logger) *calls.Service {
	cfg, err := calls.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("calls config", slog.Any("error", err))
		os.Exit(1)
	}
	var cst calls.Store = calls.NewMemory()
	if fs, ok := st.(*store.Firestore); ok {
		cst = calls.NewFirestore(fs.Client())
	}
	logger.Info("calls", slog.Bool("enabled", cfg.Enabled), slog.Bool("charged", cfg.PerMinute > 0), slog.Int("blockMinutes", cfg.BlockMinutes))
	return &calls.Service{Config: cfg, Store: cst, Coins: cs, Log: logger}
}
