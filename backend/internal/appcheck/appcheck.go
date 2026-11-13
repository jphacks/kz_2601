// Package appcheck verifies Firebase App Check tokens (#318).
//
// iOS and Android send the token in X-Firebase-AppCheck. APP_CHECK_MODE decides what a
// missing or bad token does:
//
//	off      (default) nothing is verified; the header is ignored
//	log      verify and log the result; never reject (rollout: watch the logs)
//	enforce  reject with 401 app_check_failed unless the token verifies
//
// Old clients without the header keep working in off / log.
// Runbook: docs/infra/app-check.md.
package appcheck

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Header carries the App Check token (the name the Firebase SDKs use).
const Header = "X-Firebase-AppCheck"

const (
	ModeOff     = "off"
	ModeLog     = "log"
	ModeEnforce = "enforce"
)

// Result of checking one request (log field "appCheck", 401 details.reason).
const (
	ResultValid        = "valid"
	ResultMissing      = "missing"
	ResultInvalid      = "invalid"
	ResultWrongApp     = "wrong_app"    // verified, but not one of APP_CHECK_APP_IDS
	ResultUnconfigured = "unconfigured" // verifier failed to start (fails closed in enforce)
)

// Verifier checks a token and returns the Firebase app ID it was issued to.
type Verifier interface {
	Verify(ctx context.Context, token string) (appID string, err error)
}

// ParseMode normalises APP_CHECK_MODE ("" = off).
func ParseMode(raw string) (string, error) {
	switch m := strings.ToLower(strings.TrimSpace(raw)); m {
	case "", ModeOff:
		return ModeOff, nil
	case ModeLog, ModeEnforce:
		return m, nil
	default:
		return "", fmt.Errorf("APP_CHECK_MODE must be off|log|enforce, got %q", raw)
	}
}

// Config of Middleware.
type Config struct {
	Mode string
	// Verifier nil = not configured: log mode records "unconfigured",
	// enforce rejects everything (fail closed).
	Verifier Verifier
	// AppIDs, when non-empty, limits accepted tokens to these Firebase app IDs.
	AppIDs []string
	Logger *slog.Logger
}

// Middleware applies cfg.Mode to every request it wraps. Mount it only on the
// API (not /health or public pages).
func Middleware(cfg Config) func(http.Handler) http.Handler {
	if cfg.Mode == ModeOff || cfg.Mode == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			result, appID, err := check(r, cfg)
			attrs := []any{
				slog.String("appCheck", result),
				slog.String("mode", cfg.Mode),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			}
			if id := httpx.RequestID(r.Context()); id != "" {
				attrs = append(attrs, slog.String("requestId", id))
			}
			if appID != "" {
				attrs = append(attrs, slog.String("appId", appID))
			}
			if err != nil {
				attrs = append(attrs, slog.String("error", truncate(err.Error(), 200)))
			}
			attrs = append(attrs, httpx.ClientAttrs(r)...)

			if cfg.Mode == ModeEnforce && result != ResultValid {
				logger.WarnContext(r.Context(), "appcheck rejected", attrs...)
				httpx.ErrorWithDetails(w, http.StatusUnauthorized, httpx.CodeAppCheckFailed,
					"app check token is missing or invalid", map[string]any{"reason": result})
				return
			}
			level := slog.LevelInfo
			if cfg.Mode == ModeEnforce {
				level = slog.LevelDebug // valid in enforce: the request log is enough
			}
			logger.Log(r.Context(), level, "appcheck", attrs...)
			next.ServeHTTP(w, r)
		})
	}
}

func check(r *http.Request, cfg Config) (result, appID string, err error) {
	token := strings.TrimSpace(r.Header.Get(Header))
	if token == "" {
		return ResultMissing, "", nil
	}
	if cfg.Verifier == nil {
		return ResultUnconfigured, "", nil
	}
	appID, err = cfg.Verifier.Verify(r.Context(), token)
	if err != nil {
		return ResultInvalid, "", err
	}
	if len(cfg.AppIDs) > 0 && !slices.Contains(cfg.AppIDs, appID) {
		return ResultWrongApp, appID, nil
	}
	return ResultValid, appID, nil
}

// ParseAppIDs splits APP_CHECK_APP_IDS (comma separated, blanks dropped).
func ParseAppIDs(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// FromEnv builds the middleware from APP_CHECK_MODE / APP_CHECK_APP_IDS.
// newVerifier is called only when the mode is not off; its error leaves the
// verifier unconfigured (logged; enforce then rejects everything).
func FromEnv(getenv func(string) string, logger *slog.Logger, newVerifier func() (Verifier, error)) (func(http.Handler) http.Handler, string, error) {
	mode, err := ParseMode(getenv("APP_CHECK_MODE"))
	if err != nil {
		return nil, "", err
	}
	cfg := Config{Mode: mode, AppIDs: ParseAppIDs(getenv("APP_CHECK_APP_IDS")), Logger: logger}
	if mode != ModeOff {
		v, err := newVerifier()
		if err != nil || v == nil {
			if err == nil {
				err = errors.New("no verifier")
			}
			logger.Error("app check verifier init failed", slog.String("mode", mode), slog.Any("error", err))
		} else {
			cfg.Verifier = v
		}
	}
	return Middleware(cfg), mode, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
