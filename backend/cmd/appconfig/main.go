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

	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/auth"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	verifier := newVerifier(ctx, mode, project, logger)
	st, closeStore := newStore(ctx, project, logger)
	defer closeStore()

	handler := newHandler(auth.Middleware(verifier, mode), logger, appconfig.New(st), appconfig.SharePage(st))
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

// newStore returns Firestore when GOOGLE_CLOUD_PROJECT is set (the emulator
// is used automatically when FIRESTORE_EMULATOR_HOST is set), otherwise an
// in-memory store (STORE=memory forces it). The second value closes the store.
func newStore(ctx context.Context, project string, logger *slog.Logger) (store.Store, func()) {
	if os.Getenv("STORE") == "memory" || project == "" {
		logger.Warn("using in-memory store (data is lost on restart)")
		return store.NewMemory(), func() {}
	}
	fs, err := store.NewFirestore(ctx, project)
	if err != nil {
		logger.Error("firestore init failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("using Firestore", slog.String("project", project), slog.String("emulator", os.Getenv("FIRESTORE_EMULATOR_HOST")))
	return fs, func() { _ = fs.Close() }
}

// newHandler mounts health and the share-link page (public) and the v1 API
// (authenticated). Unknown /v1 paths and wrong methods are answered by the
// API mux before auth so they stay 404/405 envelopes rather than 401.
func newHandler(authMiddleware func(http.Handler) http.Handler, logger *slog.Logger, api *http.ServeMux, sharePage http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /healthz", health)
	if sharePage != nil {
		mux.Handle("GET /s/{token}", sharePage) // web fallback of a share link (no DSL exposed)
	}
	authed := authMiddleware(api)
	mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := api.Handler(r); pattern == "" {
			api.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	}))
	return httpx.Chain(mux, httpx.WithRequestID, httpx.Log(logger), httpx.Recover(logger), httpx.ErrorEnvelope)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
