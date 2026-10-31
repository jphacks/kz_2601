package appconfig

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Client config (#302): the oldest iOS build the backend still serves and the
// newest one in the store. Read from env at startup; an unset or invalid
// value means "no enforcement" (the field is omitted and iOS does nothing).
const (
	EnvMinIOSBuild    = "MIN_IOS_BUILD"     // below this: blocking "please update" screen
	EnvLatestIOSBuild = "LATEST_IOS_BUILD"  // below this: dismissible notice
	EnvIOSAppStoreURL = "IOS_APP_STORE_URL" // https://apps.apple.com/app/id<id> (optional)
)

// ClientConfigValues is the body of GET /v1/client-config.
type ClientConfigValues struct {
	MinIOSBuild    int    `json:"minIOSBuild,omitempty"`
	LatestIOSBuild int    `json:"latestIOSBuild,omitempty"`
	IOSAppStoreURL string `json:"iosAppStoreUrl,omitempty"`
}

// ClientConfigFromEnv reads the env vars through getenv (os.Getenv).
func ClientConfigFromEnv(getenv func(string) string) ClientConfigValues {
	build := func(key string) int {
		raw := strings.TrimSpace(getenv(key))
		if raw == "" {
			return 0
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			slog.Warn("appconfig: ignoring invalid build number", slog.String("env", key), slog.String("value", raw))
			return 0
		}
		return n
	}
	v := ClientConfigValues{MinIOSBuild: build(EnvMinIOSBuild), LatestIOSBuild: build(EnvLatestIOSBuild)}
	if raw := strings.TrimSpace(getenv(EnvIOSAppStoreURL)); raw != "" {
		if u, err := url.Parse(raw); err == nil && (u.Scheme == "https" || u.Scheme == "itms-apps") && u.Host != "" {
			v.IOSAppStoreURL = raw
		} else {
			slog.Warn("appconfig: ignoring invalid App Store URL", slog.String("env", EnvIOSAppStoreURL), slog.String("value", raw))
		}
	}
	// A latest below min would only confuse: min wins.
	if v.LatestIOSBuild != 0 && v.LatestIOSBuild < v.MinIOSBuild {
		v.LatestIOSBuild = v.MinIOSBuild
	}
	return v
}

// ClientConfig serves GET /v1/client-config. It is public (iOS checks it
// before sign-in) and holds no user data.
func ClientConfig(v ClientConfigValues) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		httpx.WriteJSON(w, http.StatusOK, v)
	})
}
