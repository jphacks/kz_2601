package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"google.golang.org/api/idtoken"

	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
)

// newExt is the runner's external API door (#1493,
// docs/backend/features/external-apis.md). EXT_RUNNER_URL is the runner's
// origin; appconfig calls it with an OIDC ID token for that URL (Cloud Run:
// the runner's invoker is appconfig's SA). http://localhost is called
// without one (the emulator). Unset: the ext routes answer 503
// ext_unavailable.
func newExt(ctx context.Context, logger *slog.Logger) appconfig.ExtRunner {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("EXT_RUNNER_URL")), "/")
	if base == "" {
		logger.Info("ext: EXT_RUNNER_URL not set: external APIs are off")
		return nil
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "localhost")) {
		logger.Error("ext: EXT_RUNNER_URL must be https://… (or http://localhost)", slog.String("value", base))
		os.Exit(1)
	}
	hc := &http.Client{}
	if u.Scheme == "https" {
		if hc, err = idtoken.NewClient(ctx, base); err != nil {
			logger.Error("ext: runner ID token client", slog.Any("error", err))
			os.Exit(1)
		}
	}
	hc.Timeout = 20 * time.Second
	logger.Info("ext: runner", slog.String("url", base))
	return &extapi.Client{BaseURL: base, HTTP: hc}
}
