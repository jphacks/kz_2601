package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appcheck"
	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

const shutdownTimeout = 15 * time.Second

func main() {
	logger := httpx.NewLogger()
	slog.SetDefault(logger)

	port := envOr("PORT", "8080")
	mode := envOr("AUTH_MODE", auth.ModeFirebase)
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	target := fsenv.FromEnv(os.Getenv)
	guardTarget(target, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	verifier := newVerifier(ctx, mode, project, logger)
	st, closeStore := newStore(ctx, target, logger)
	defer closeStore()

	appCheck := newAppCheck(ctx, project, logger)
	// deleted accounts (DELETE /v1/me) get 410 on everything else: appconfig.RejectDeleted
	authed := func(h http.Handler) http.Handler {
		return appCheck(auth.Middleware(verifier, mode)(appconfig.RejectDeleted(st, h)))
	}
	api := appconfig.New(st)
	entitlement.Register(api, newEntitlements(st, logger)) // Pro (#314)
	handler := newHandler(authed, logger, api, appconfig.SharePage(st))
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Error("listen failed", slog.String("port", port), slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("appconfig listening",
		slog.String("addr", listener.Addr().String()),
		slog.String("authMode", mode),
		slog.String("project", project),
	)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve failed", slog.Any("error", err))
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown failed", slog.Any("error", err))
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// newVerifier picks the ID-token verifier for AUTH_MODE.
//   - firebase: Admin SDK (needs GOOGLE_CLOUD_PROJECT + ADC)
//   - dev: Bearer value is taken as the uid (X-Debug-Uid also works when
//     FIREBASE_AUTH_EMULATOR_HOST is set; see auth.Middleware)
func newVerifier(ctx context.Context, mode, project string, logger *slog.Logger) auth.TokenVerifier {
	if mode == auth.ModeDev {
		logger.Warn("AUTH_MODE=dev: bearer tokens are trusted as uids; never use in production")
		return auth.DevVerifier{}
	}
	if project == "" {
		logger.Error("GOOGLE_CLOUD_PROJECT is required for AUTH_MODE=firebase")
		return auth.NotConfigured{}
	}
	v, err := auth.NewFirebaseVerifier(ctx, project)
	if err != nil {
		logger.Error("firebase auth init failed; all requests will be 401", slog.Any("error", err))
		return auth.NotConfigured{}
	}
	return v
}

// newAppCheck returns the App Check middleware (#318) for the authenticated
// API only: /health, /s/{token} and /v1/client-config (needed to tell old
// builds to update) stay open. APP_CHECK_MODE=off (default) | log | enforce,
// APP_CHECK_APP_IDS optional. docs/infra/app-check.md
func newAppCheck(ctx context.Context, project string, logger *slog.Logger) func(http.Handler) http.Handler {
	mw, mode, err := appcheck.FromEnv(os.Getenv, logger, func() (appcheck.Verifier, error) {
		return appcheck.NewFirebaseVerifier(ctx, project)
	})
	if err != nil {
		logger.Error("refusing to start", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("app check", slog.String("mode", mode))
	return mw
}

// guardTarget exits when a stg process would use the production Firestore
// without ALLOW_SHARED_PROD_DB=1 (fsenv.Check, #290).
func guardTarget(t fsenv.Target, logger *slog.Logger) {
	shared, err := fsenv.Check(t, os.Getenv)
	if err != nil {
		logger.Error("refusing to start", slog.Any("error", err))
		os.Exit(1)
	}
	if shared {
		logger.Warn("stg shares the production Firestore (ALLOW_SHARED_PROD_DB=1)", slog.String("firestore", t.String()))
	}
}

// newStore returns Firestore when GOOGLE_CLOUD_PROJECT is set (database from
// FIRESTORE_DATABASE; the emulator is used automatically when
// FIRESTORE_EMULATOR_HOST is set), otherwise an in-memory store (STORE=memory
// forces it). The second value closes the store.
func newStore(ctx context.Context, t fsenv.Target, logger *slog.Logger) (store.Store, func()) {
	if os.Getenv("STORE") == "memory" || t.Project == "" {
		logger.Warn("using in-memory store (data is lost on restart)")
		return store.NewMemory(), func() {}
	}
	fs, err := store.NewFirestore(ctx, t.Project, t.Database)
	if err != nil {
		logger.Error("firestore init failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("using Firestore", slog.String("project", t.Project), slog.String("database", t.Database), slog.String("emulator", os.Getenv("FIRESTORE_EMULATOR_HOST")))
	return fs, func() { _ = fs.Close() }
}

// newEntitlements verifies StoreKit transactions against the pinned Apple root
// (policy: entitlement.ConfigFromEnv) and stores them next to st.
func newEntitlements(st store.Store, logger *slog.Logger) *entitlement.Service {
	policy, err := entitlement.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Error("appstore config", slog.Any("error", err))
		os.Exit(1)
	}
	var es entitlement.Store = entitlement.NewMemory()
	if fs, ok := st.(*store.Firestore); ok {
		es = entitlement.NewFirestore(fs.Client())
	}
	logger.Info("entitlements", slog.Any("bundleIds", policy.BundleIDs), slog.Any("productIds", policy.ProductIDs), slog.Any("environments", policy.Environments))
	return &entitlement.Service{
		Verifier: &appstore.Verifier{Policy: policy},
		Store:    es,
		Deleted:  st.UserDeleted,
		Log:      logger,
	}
}

// newHandler mounts health and the share-link page (public) and the v1 API
// (authenticated). Unknown /v1 paths and wrong methods are answered by the
// API mux before auth so they stay 404/405 envelopes rather than 401.
func newHandler(authMiddleware func(http.Handler) http.Handler, logger *slog.Logger, api *http.ServeMux, sharePage http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /healthz", health)
	if sharePage != nil {
		mux.Handle("GET /s/{token}", sharePage) // read-only web view of a share link (#362, no records)
	}
	// minimum / latest iOS build (#302): public, read once from env
	mux.Handle("GET /v1/client-config", appconfig.ClientConfig(appconfig.ClientConfigFromEnv(os.Getenv)))
	authed := authMiddleware(api)
	mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := api.Handler(r); pattern == "" {
			api.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	}))
	// /debug/error answers 5xx/panic on purpose on stg only (alert check, #295)
	debug := httpx.DebugErrors(os.Getenv("APP_ENV") == "stg")
	return httpx.Chain(mux, httpx.WithRequestID, httpx.Log(logger), httpx.Recover(logger), debug, httpx.ErrorEnvelope)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
