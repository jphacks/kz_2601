// Command appstore-reconcile re-reads Pro subscriptions from the App Store
// Server API and stores changes that App Store Server Notifications v2 did not
// deliver (#315, entitlement.Reconciler). Meant for a Cloud Run Job run by
// Cloud Scheduler a few times a day (docs/infra/app-store-notifications.md).
// Exit status 1 when any subscription failed (the next run retries it).
//
// Environment:
//
//	GOOGLE_CLOUD_PROJECT          required (Firestore project)
//	FIRESTORE_DATABASE            database ID (default "(default)"; internal/fsenv)
//	APP_ENV / ALLOW_SHARED_PROD_DB  stg guard (internal/fsenv)
//	FIRESTORE_EMULATOR_HOST       local emulator (make run-appstore-reconcile)
//	APPSTORE_API_KEY_ID           In-App Purchase key id (App Store Connect)
//	APPSTORE_API_ISSUER_ID        issuer id (App Store Connect)
//	APPSTORE_API_PRIVATE_KEY      the key's .p8 (PEM); from Secret Manager
//	APPSTORE_BUNDLE_IDS / APPSTORE_PRODUCT_IDS / APPSTORE_ENVIRONMENTS
//	                              same policy as appconfig (entitlement.ConfigFromEnv)
//
// stg and prod share one Firestore database: schedule this job in one
// environment only.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "only log what would change")
	limit := flag.Int("limit", 1000, "max App Store Server API calls per run")
	window := flag.Duration("window", 35*24*time.Hour, "skip Pro that ended longer ago than this")
	flag.Parse()

	logger := httpx.NewLogger()
	slog.SetDefault(logger)
	policy, err := entitlement.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("appstore-reconcile: config", slog.Any("error", err))
		os.Exit(2)
	}
	api, err := serverAPI(os.Getenv, policy)
	if err != nil {
		logger.Error("appstore-reconcile: server api", slog.Any("error", err))
		os.Exit(2)
	}
	target := fsenv.FromEnv(os.Getenv)
	if target.Project == "" {
		logger.Error("appstore-reconcile: GOOGLE_CLOUD_PROJECT is required")
		os.Exit(2)
	}
	if _, err := fsenv.Check(target, os.Getenv); err != nil {
		logger.Error("appstore-reconcile: refusing to start", slog.Any("error", err))
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		logger.Error("appstore-reconcile: firestore init failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	r := &entitlement.Reconciler{
		Verifier: &appstore.Verifier{Policy: policy},
		Store:    entitlement.NewFirestore(st.Client()),
		API:      api,
		Window:   *window,
		Log:      logger,
	}
	if run(ctx, r, *limit, *dryRun, logger) {
		os.Exit(1)
	}
}

// run reports whether anything failed.
func run(ctx context.Context, r *entitlement.Reconciler, limit int, dryRun bool, logger *slog.Logger) bool {
	res, err := r.Run(ctx, limit, dryRun)
	if err != nil {
		logger.Error("appstore-reconcile: failed", slog.Any("error", err))
		return true
	}
	logger.Info("appstore-reconcile: done", slog.Int("bindings", res.Bindings), slog.Int("checked", res.Checked),
		slog.Int("updated", res.Updated), slog.Int("failed", res.Failed), slog.Bool("dryRun", dryRun))
	return res.Failed > 0
}

// serverAPI reads the In-App Purchase key from the environment.
func serverAPI(getenv func(string) string, policy appstore.Policy) (*appstore.ServerAPI, error) {
	key, err := appstore.ParsePrivateKey([]byte(getenv("APPSTORE_API_PRIVATE_KEY")))
	if err != nil {
		return nil, err
	}
	api := &appstore.ServerAPI{KeyID: getenv("APPSTORE_API_KEY_ID"), IssuerID: getenv("APPSTORE_API_ISSUER_ID"), Key: key}
	if api.KeyID == "" || api.IssuerID == "" {
		return nil, errors.New("APPSTORE_API_KEY_ID and APPSTORE_API_ISSUER_ID are required")
	}
	api.BundleID = policy.BundleIDs[0]
	return api, nil
}
