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

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

const shutdownTimeout = 15 * time.Second

func main() {
	logger := httpx.NewLogger()
	slog.SetDefault(logger)

	port := envOr("PORT", "8080")
	mode := envOr("AUTH_MODE", auth.ModeFirebase)

	verifier, err := newVerifier(context.Background(), mode, logger)
	if err != nil {
		logger.Error("auth verifier setup failed", slog.String("authMode", mode), slog.Any("error", err))
		os.Exit(1)
	}

	handler := newHandler(auth.Middleware(verifier, mode), logger)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, listenErr := net.Listen("tcp", ":"+port)
	if listenErr != nil {
		logger.Error("listen failed", slog.String("port", port), slog.Any("error", listenErr))
		os.Exit(1)
	}
	logger.Info("appconfig listening",
		slog.String("addr", listener.Addr().String()),
		slog.String("authMode", mode),
		slog.String("project", os.Getenv("GOOGLE_CLOUD_PROJECT")),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

func newVerifier(ctx context.Context, mode string, logger *slog.Logger) (auth.TokenVerifier, error) {
	if err := auth.CheckEmulatorHost(mode); err != nil {
		return nil, err
	}
	verifier, err := auth.NewFirebaseVerifier(ctx)
	if err == nil {
		return verifier, nil
	}
	if mode == auth.ModeFirebase {
		return nil, err
	}
	logger.Warn("firebase verifier unavailable, only the debug uid header will authenticate",
		slog.String("authMode", mode),
		slog.Any("error", err),
	)
	return auth.NotConfigured{}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newHandler(authMiddleware func(http.Handler) http.Handler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /healthz", health)
	mux.Handle("GET /v1/apps", authMiddleware(http.HandlerFunc(listApps)))
	return httpx.Chain(mux, httpx.WithRequestID, httpx.Log(logger), httpx.Recover(logger), httpx.ErrorEnvelope)
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func listApps(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": []any{}})
}
