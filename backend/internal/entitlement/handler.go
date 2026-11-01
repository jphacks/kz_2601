package entitlement

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Error codes (openapi-appconfig.yaml).
const (
	CodeInvalidTransaction = "invalid_transaction"
	CodeOwnedByOther       = "transaction_owned_by_other"
)

// maxJWSBytes bounds the request (a JWSTransaction is ~3-4 KB).
const maxJWSBytes = 32 << 10

// Register mounts the entitlement routes on mux. Authentication (and
// appconfig.RejectDeleted) is applied by the caller, like the other /v1 routes.
//
//	GET  /v1/me/entitlements
//	POST /v1/me/entitlements/apple   {"signedTransaction": "<JWS>"}
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
			httpx.WriteJSON(w, http.StatusOK, st)
		case errors.As(err, &verr):
			slog.WarnContext(r.Context(), "entitlement: rejected transaction", slog.String("reason", verr.Reason), slog.Any("error", verr.Err))
			httpx.ErrorWithDetails(w, http.StatusUnprocessableEntity, CodeInvalidTransaction,
				"the signed transaction could not be verified", map[string]any{"reason": verr.Reason})
		case errors.Is(err, ErrOwnedByOther):
			httpx.Error(w, http.StatusConflict, CodeOwnedByOther, "this subscription is linked to another account")
		default:
			slog.ErrorContext(r.Context(), "entitlement: submit failed", slog.Any("error", err))
			httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		}
	})
}
