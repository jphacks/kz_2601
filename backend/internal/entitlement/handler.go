package entitlement

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/googleplay"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// maxJWSBytes bounds the request (a JWSTransaction is ~3-4 KB).
const maxJWSBytes = 32 << 10

// Bounds of POST /v1/me/entitlements/google (a purchase token is ~150-200 chars).
const (
	maxPlayBytes     = 8 << 10
	maxPurchaseToken = 4096
)

// Register mounts the entitlement routes on mux. Authentication (and
// appconfig.RejectDeleted) is applied by the caller, like the other /v1 routes.
//
//	GET  /v1/me/entitlements
//	POST /v1/me/entitlements/apple   {"signedTransaction": "<JWS>"}
//	POST /v1/me/entitlements/google  {"productId": "...", "purchaseToken": "..."}
func Register(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("GET /v1/me/entitlements", func(w http.ResponseWriter, r *http.Request) {
		st, err := s.Get(r.Context(), auth.UID(r.Context()))
		if err != nil {
			slog.ErrorContext(r.Context(), "entitlement: get failed", slog.Any("error", err))
			httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("POST /v1/me/entitlements/apple", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxJWSBytes)
		var req struct {
			SignedTransaction string `json:"signedTransaction"`
		}
		if !httpx.DecodeBody(w, r, &req) {
			return
		}
		if req.SignedTransaction == "" {
			httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "signedTransaction is required")
			return
		}
		st, err := s.SubmitApple(r.Context(), auth.UID(r.Context()), req.SignedTransaction)
		var verr *appstore.Error
		switch {
		case err == nil:
			if s.Changed != nil {
				s.Changed(auth.UID(r.Context()))
			}
			httpx.WriteJSON(w, http.StatusOK, st)
		case errors.As(err, &verr):
			slog.WarnContext(r.Context(), "entitlement: rejected transaction", slog.String("reason", verr.Reason), slog.Any("error", verr.Err))
			httpx.ErrorWithDetails(w, http.StatusUnprocessableEntity, httpx.CodeInvalidTransaction,
				"the signed transaction could not be verified", map[string]any{"reason": verr.Reason})
		case errors.Is(err, ErrOwnedByOther):
			httpx.Error(w, http.StatusConflict, httpx.CodeTransactionOwnedByOther, "this subscription is linked to another account")
		default:
			slog.ErrorContext(r.Context(), "entitlement: submit failed", slog.Any("error", err))
			httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		}
	})
	mux.HandleFunc("POST /v1/me/entitlements/google", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxPlayBytes)
		var req struct {
			ProductID     string `json:"productId"`
			PurchaseToken string `json:"purchaseToken"`
		}
		if !httpx.DecodeBody(w, r, &req) {
			return
		}
		if req.ProductID == "" || req.PurchaseToken == "" || len(req.PurchaseToken) > maxPurchaseToken {
			httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "productId and purchaseToken are required")
			return
		}
		uid := auth.UID(r.Context())
		st, err := s.SubmitGoogle(r.Context(), uid, req.ProductID, req.PurchaseToken)
		var perr *googleplay.Error
		switch {
		case err == nil:
			if s.Changed != nil {
				s.Changed(uid)
			}
			httpx.WriteJSON(w, http.StatusOK, st)
		case errors.Is(err, ErrPlayNotConfigured):
			httpx.Error(w, http.StatusServiceUnavailable, httpx.CodeNotConfigured, "google play purchases are not configured on this server")
		case errors.As(err, &perr):
			slog.WarnContext(r.Context(), "entitlement: rejected play purchase", slog.String("reason", perr.Reason), slog.Any("error", perr.Err))
			httpx.ErrorWithDetails(w, http.StatusUnprocessableEntity, httpx.CodeInvalidTransaction,
				"the purchase could not be verified", map[string]any{"reason": perr.Reason})
		case errors.Is(err, ErrAccountMismatch):
			httpx.Error(w, http.StatusConflict, httpx.CodeAccountMismatch, "this purchase was made for another account")
		case errors.Is(err, ErrOwnedByOther):
			httpx.Error(w, http.StatusConflict, httpx.CodeTransactionOwnedByOther, "this subscription is linked to another account")
		case errors.Is(err, ErrPlayUnavailable):
			slog.WarnContext(r.Context(), "entitlement: google play unavailable", slog.Any("error", err))
			httpx.Error(w, http.StatusServiceUnavailable, httpx.CodeUpstreamUnavailable, "google play could not be reached; try again")
		default:
			slog.ErrorContext(r.Context(), "entitlement: play submit failed", slog.Any("error", err))
			httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		}
	})
}
