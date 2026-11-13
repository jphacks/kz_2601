package appconfig

import (
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Client config (#302, macOS #981, Windows/Linux #2131): the oldest iOS / Android / macOS / Windows / Linux build the backend still
// serves and the newest one in the store. Read from env at startup; an unset or
// invalid value means "no enforcement" (the field is omitted and the client
// does nothing).
const (
	EnvMinIOSBuild    = "MIN_IOS_BUILD"     // below this: blocking "please update" screen
	EnvLatestIOSBuild = "LATEST_IOS_BUILD"  // below this: dismissible notice
	EnvIOSAppStoreURL = "IOS_APP_STORE_URL" // https://apps.apple.com/app/id<id> (optional)

	EnvMinAndroidBuild    = "MIN_ANDROID_BUILD"    // versionCode; same meaning as MIN_IOS_BUILD
	EnvLatestAndroidBuild = "LATEST_ANDROID_BUILD" // versionCode; same meaning as LATEST_IOS_BUILD
	EnvAndroidStoreURL    = "ANDROID_STORE_URL"    // https://play.google.com/store/apps/details?id=<pkg> (optional)

	EnvMinMacOSBuild    = "MIN_MACOS_BUILD"    // CFBundleVersion of the Mac app; same meaning as MIN_IOS_BUILD
	EnvLatestMacOSBuild = "LATEST_MACOS_BUILD" // same meaning as LATEST_IOS_BUILD
	EnvMacAppStoreURL   = "MAC_APP_STORE_URL"  // https://apps.apple.com/app/id<id> or macappstore:// (optional)

	EnvMinWindowsBuild    = "MIN_WINDOWS_BUILD"    // build number of the Windows app; same meaning as MIN_IOS_BUILD
	EnvLatestWindowsBuild = "LATEST_WINDOWS_BUILD" // same meaning as LATEST_IOS_BUILD
	EnvMinLinuxBuild      = "MIN_LINUX_BUILD"      // build number of the Linux app; same meaning as MIN_IOS_BUILD
	EnvLatestLinuxBuild   = "LATEST_LINUX_BUILD"   // same meaning as LATEST_IOS_BUILD
)

// ClientConfigValues is the body of GET /v1/client-config.
type ClientConfigValues struct {
	MinIOSBuild    int    `json:"minIOSBuild,omitempty"`
	LatestIOSBuild int    `json:"latestIOSBuild,omitempty"`
	IOSAppStoreURL string `json:"iosAppStoreUrl,omitempty"`

	MinAndroidBuild    int    `json:"minAndroidBuild,omitempty"`
	LatestAndroidBuild int    `json:"latestAndroidBuild,omitempty"`
	AndroidStoreURL    string `json:"androidStoreUrl,omitempty"`

	MinMacOSBuild    int    `json:"minMacOSBuild,omitempty"`
	LatestMacOSBuild int    `json:"latestMacOSBuild,omitempty"`
	MacAppStoreURL   string `json:"macAppStoreUrl,omitempty"`

	MinWindowsBuild    int `json:"minWindowsBuild,omitempty"`
	LatestWindowsBuild int `json:"latestWindowsBuild,omitempty"`

	MinLinuxBuild    int `json:"minLinuxBuild,omitempty"`
	LatestLinuxBuild int `json:"latestLinuxBuild,omitempty"`

	Notice *Notice `json:"notice,omitempty"` // #333 (notice.go)
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
	// storeURL accepts only the given schemes, with a host.
	storeURL := func(key string, schemes ...string) string {
		raw := strings.TrimSpace(getenv(key))
		if raw == "" {
			return ""
		}
		if u, err := url.Parse(raw); err == nil && u.Host != "" && slices.Contains(schemes, u.Scheme) {
			return raw
		}
		slog.Warn("appconfig: ignoring invalid store URL", slog.String("env", key), slog.String("value", raw))
		return ""
	}
	v := ClientConfigValues{
		MinIOSBuild:        build(EnvMinIOSBuild),
		LatestIOSBuild:     build(EnvLatestIOSBuild),
		IOSAppStoreURL:     storeURL(EnvIOSAppStoreURL, "https", "itms-apps"),
		MinAndroidBuild:    build(EnvMinAndroidBuild),
		LatestAndroidBuild: build(EnvLatestAndroidBuild),
		AndroidStoreURL:    storeURL(EnvAndroidStoreURL, "https", "market"),
		MinMacOSBuild:      build(EnvMinMacOSBuild),
		LatestMacOSBuild:   build(EnvLatestMacOSBuild),
		MacAppStoreURL:     storeURL(EnvMacAppStoreURL, "https", "macappstore"),
		MinWindowsBuild:    build(EnvMinWindowsBuild),
		LatestWindowsBuild: build(EnvLatestWindowsBuild),
		MinLinuxBuild:      build(EnvMinLinuxBuild),
		LatestLinuxBuild:   build(EnvLatestLinuxBuild),
		Notice:             NoticeFromEnv(getenv),
	}
	// A latest below min would only confuse: min wins.
	if v.LatestIOSBuild != 0 && v.LatestIOSBuild < v.MinIOSBuild {
		v.LatestIOSBuild = v.MinIOSBuild
	}
	if v.LatestAndroidBuild != 0 && v.LatestAndroidBuild < v.MinAndroidBuild {
		v.LatestAndroidBuild = v.MinAndroidBuild
	}
	if v.LatestMacOSBuild != 0 && v.LatestMacOSBuild < v.MinMacOSBuild {
		v.LatestMacOSBuild = v.MinMacOSBuild
	}
	if v.LatestWindowsBuild != 0 && v.LatestWindowsBuild < v.MinWindowsBuild {
		v.LatestWindowsBuild = v.MinWindowsBuild
	}
	if v.LatestLinuxBuild != 0 && v.LatestLinuxBuild < v.MinLinuxBuild {
		v.LatestLinuxBuild = v.MinLinuxBuild
	}
	return v
}

// ClientConfig serves GET /v1/client-config. It is public (clients check it
// before sign-in) and holds no user data.
func ClientConfig(v ClientConfigValues) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		httpx.WriteJSON(w, http.StatusOK, v)
	})
}
