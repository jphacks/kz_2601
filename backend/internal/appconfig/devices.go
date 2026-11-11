package appconfig

// Push devices and recipients' consent (#269, #339). Only the caller's own
// devices and consents are reachable: there is no uid in the paths. Device
// tokens are never logged and never returned.

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// routes (#269)
func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		mux.HandleFunc("PUT /v1/me/devices/{installationId}", h.putDevice)
		mux.HandleFunc("DELETE /v1/me/devices/{installationId}", h.deleteDevice)
		mux.HandleFunc("PUT /v1/me/notify-consent/{appId}", h.putNotifyConsent)
	})
}

var (
	installationIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
	// APNs tokens are 32 bytes today; Apple says the length may grow.
	deviceTokenRe = regexp.MustCompile(`^(?:[0-9a-f]{2}){32,100}$`)
	// FCM registration tokens are opaque (about 150-200 characters today,
	// case-sensitive); only the characters FCM uses are accepted.
	fcmTokenRe = regexp.MustCompile(`^[A-Za-z0-9_:.-]{32,1000}$`)
)

const maxDeviceText = 32 // runes of appVersion / os

type deviceIn struct {
	Token string `json:"token"`
	// Platform: ios (default, APNs) / android (FCM, #918).
	Platform    string `json:"platform"`
	Environment string `json:"environment"`
	AppVersion  string `json:"appVersion"`
	OS          string `json:"os"`
}

func installationID(r *http.Request) (string, error) {
	v := r.PathValue("installationId")
	if !installationIDRe.MatchString(v) {
		return "", apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid installationId")
	}
	return v, nil
}

func badDevice(field, msg string) error {
	return apiErr(http.StatusBadRequest, httpx.CodeBadRequest, msg).With("field", field)
}

func deviceFrom(id string, in deviceIn) (store.Device, error) {
	d := store.Device{InstallationID: id, Platform: in.Platform, Token: strings.TrimSpace(in.Token),
		Environment: in.Environment, AppVersion: strings.TrimSpace(in.AppVersion), OS: strings.TrimSpace(in.OS)}
	switch d.Platform {
	case "", store.PlatformIOS:
		d.Platform = store.PlatformIOS
		d.Token = strings.ToLower(d.Token)
		if !deviceTokenRe.MatchString(d.Token) {
			return d, badDevice("token", "token must be an APNs device token (hex)")
		}
	case store.PlatformAndroid:
		// FCM tokens are case-sensitive: kept as sent.
		if !fcmTokenRe.MatchString(d.Token) {
			return d, badDevice("token", "token must be an FCM registration token")
		}
		if d.Environment == "" {
			d.Environment = store.DeviceProduction // not used to filter Android tokens
		}
	default:
		return d, badDevice("platform", "platform must be ios or android")
	}
	switch {
	case d.Environment != store.DeviceSandbox && d.Environment != store.DeviceProduction:
		return d, badDevice("environment", "environment must be sandbox or production")
	case tooLong(d.AppVersion, maxDeviceText):
		return d, badDevice("appVersion", fmt.Sprintf("appVersion must be at most %d characters", maxDeviceText))
	case tooLong(d.OS, maxDeviceText):
		return d, badDevice("os", fmt.Sprintf("os must be at most %d characters", maxDeviceText))
	}
	return d, nil
}

// putDevice: PUT /v1/me/devices/{installationId}. Creates or replaces the
// caller's device; the oldest of more than store.MaxDevicesPerUser go.
func (h *Handler) putDevice(w http.ResponseWriter, r *http.Request) {
	id, err := installationID(r)
	if err != nil {
		fail(w, err)
		return
	}
	var in deviceIn
	if !httpx.DecodeBody(w, r, &in) {
		return
	}
	d, err := deviceFrom(id, in)
	if err != nil {
		fail(w, err)
		return
	}
	out, err := h.store.PutDevice(r.Context(), uidOf(r), d)
	if err != nil {
		fail(w, fmt.Errorf("put device: %w", err))
		return
	}
	// never log the token
	slog.Info("appconfig: device registered", slog.String("platform", out.Platform), slog.String("environment", out.Environment))
	httpx.WriteJSON(w, http.StatusOK, out)
}

// deleteDevice: DELETE /v1/me/devices/{installationId} (sign-out, notifications
// turned off). Idempotent: always 204.
func (h *Handler) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := installationID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err := h.store.DeleteDevice(r.Context(), uidOf(r), id); err != nil {
		fail(w, fmt.Errorf("delete device: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type consentIn struct {
	Allowed *bool `json:"allowed"`
}

// putNotifyConsent: PUT /v1/me/notify-consent/{appId} {allowed}. Whether the
// caller receives the notifications others' actions send in appId (#339).
// Turning it on needs the caller's copy of the app (404) and, for an app
// with shared data, participation (403 not_member). Turning it off always
// works (even after the app is gone).
func (h *Handler) putNotifyConsent(w http.ResponseWriter, r *http.Request) {
	appID, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	var in consentIn
	if !httpx.DecodeBody(w, r, &in) {
		return
	}
	if in.Allowed == nil {
		fail(w, badDevice("allowed", "allowed is required"))
		return
	}
	ctx, uid := r.Context(), uidOf(r)
	if *in.Allowed {
		app, err := h.store.GetApp(ctx, uid, appID)
		if err != nil {
			fail(w, mapStoreErr(err, "app"))
			return
		}
		a, err := h.parsedDSL(uid, app)
		if err != nil {
			fail(w, err)
			return
		}
		if hasSharedScope(a) {
			role, err := h.memberRole(ctx, appID, uid)
			if err != nil {
				fail(w, err)
				return
			}
			if role == "" {
				fail(w, notMember())
				return
			}
		}
	}
	c, err := h.store.SetNotifyConsent(ctx, uid, appID, *in.Allowed)
	if err != nil {
		fail(w, fmt.Errorf("set notify consent: %w", err))
		return
	}
	slog.Info("appconfig: notify consent set", slog.String("appId", appID), slog.Bool("allowed", c.Allowed))
	httpx.WriteJSON(w, http.StatusOK, c)
}
