package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/googleplay"
)

// googlePlayFromEnv reads the Google Play settings (Android Pro purchases).
// Without GOOGLE_PLAY_SERVICE_ACCOUNT_JSON it returns nil and
// POST /v1/me/entitlements/google answers 503 not_configured. The key comes
// from Secret Manager (--set-secrets); it is never logged.
//
//	GOOGLE_PLAY_SERVICE_ACCOUNT_JSON  service account key (JSON) with Play Console API access
//	GOOGLE_PLAY_PACKAGE_NAME          default jp.kz2601.metamon
//	GOOGLE_PLAY_PRODUCT_IDS           default jp.kz2601.metamon.pro.monthly
func googlePlayFromEnv(ctx context.Context, getenv func(string) string) (*entitlement.GooglePlay, error) {
	raw := strings.TrimSpace(getenv("GOOGLE_PLAY_SERVICE_ACCOUNT_JSON"))
	if raw == "" {
		return nil, nil
	}
	c, err := googleplay.New(ctx, []byte(raw))
	if err != nil {
		// googleplay's errors carry no key material
		return nil, fmt.Errorf("GOOGLE_PLAY_SERVICE_ACCOUNT_JSON: %w", err)
	}
	return &entitlement.GooglePlay{API: c, Policy: entitlement.GooglePlayConfigFromEnv(getenv)}, nil
}

// playNotifications mounts Real-time developer notifications (Pub/Sub push)
// on the public mux when Play is configured and both are set:
//
//	GOOGLE_PLAY_RTDN_AUDIENCE         the push subscription's OIDC audience
//	GOOGLE_PLAY_RTDN_SERVICE_ACCOUNT  the push subscription's service account email
func playNotifications(ents *entitlement.Service, getenv func(string) string, logger *slog.Logger) func(*http.ServeMux) {
	aud := strings.TrimSpace(getenv("GOOGLE_PLAY_RTDN_AUDIENCE"))
	email := strings.TrimSpace(getenv("GOOGLE_PLAY_RTDN_SERVICE_ACCOUNT"))
	if ents.GooglePlay == nil || aud == "" || email == "" {
		logger.Info("google play notifications: off (needs GOOGLE_PLAY_SERVICE_ACCOUNT_JSON, GOOGLE_PLAY_RTDN_AUDIENCE, GOOGLE_PLAY_RTDN_SERVICE_ACCOUNT)")
		return func(*http.ServeMux) {}
	}
	n := &entitlement.PlayNotifications{
		Service:  ents,
		Verifier: &entitlement.OIDCPushVerifier{Audience: aud, Email: email},
		Log:      logger,
	}
	logger.Info("google play notifications: on", slog.String("path", entitlement.PlayNotificationsPath))
	return func(mux *http.ServeMux) { entitlement.RegisterPlayNotifications(mux, n) }
}
