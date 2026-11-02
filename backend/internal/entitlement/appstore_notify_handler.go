package entitlement

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// NotificationsPath is the URL registered in App Store Connect (Production
// and Sandbox server URL). docs/infra/app-store-notifications.md
const NotificationsPath = "/v1/appstore/notifications"

// maxNotificationBytes bounds the body (signedPayload embeds two JWS, ~10 KB).
const maxNotificationBytes = 256 << 10

// RegisterNotifications mounts POST NotificationsPath on a public mux: Apple
// sends no credentials, the signed payload is the authentication.
//
//	200 {"outcome": "applied|duplicate|unbound|ignored"}   Apple stops retrying
//	400 / 422                                              malformed / not signed by Apple
//	500                                                    transient; Apple retries
func RegisterNotifications(mux *http.ServeMux, n *Notifications) {
	mux.HandleFunc("POST "+NotificationsPath, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SignedPayload string `json:"signedPayload"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxNotificationBytes)).Decode(&req); err != nil || req.SignedPayload == "" {
			httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "signedPayload is required")
			return
		}
		outcome, err := n.Handle(r.Context(), req.SignedPayload)
		var verr *appstore.Error
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"outcome": outcome})
		case errors.As(err, &verr):
			slog.WarnContext(r.Context(), "appstore notification: rejected", slog.String("reason", verr.Reason), slog.Any("error", verr.Err))
			httpx.ErrorWithDetails(w, http.StatusUnprocessableEntity, httpx.CodeInvalidTransaction,
				"the signed payload could not be verified", map[string]any{"reason": verr.Reason})
		default:
			slog.ErrorContext(r.Context(), "appstore notification: failed", slog.Any("error", err))
			httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		}
	})
}
