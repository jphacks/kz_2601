// Package coinsapi is the appconfig HTTP API of the coin wallet (#630).
package coinsapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// maxJWSBytes bounds the request (a JWSTransaction is ~3-4 KB).
const maxJWSBytes = 32 << 10

// Register mounts the authenticated routes on mux (auth and RejectDeleted
// are applied by the caller, like the other /v1 routes).
//
//	GET  /v1/me/coins                         balance by source
//	GET  /v1/me/coins/ledger?limit=&cursor=   history, newest first
//	PUT  /v1/me/coins/settings                {"overflow": {"enabled", "monthlyCap"}} (the runner enforces it, part 3)
//	POST /v1/me/coins/apple                   {"signedTransaction": "<JWS>"} credits a coin pack
func Register(mux *http.ServeMux, s *coins.Service) {
	mux.HandleFunc("GET /v1/me/coins", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Wallet(r.Context(), auth.UID(r.Context()), auth.Anonymous(r.Context()))
		if err != nil {
			internal(w, r, "wallet", err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("GET /v1/me/coins/ledger", func(w http.ResponseWriter, r *http.Request) {
		limit := 0
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 100 {
				httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "limit must be 1-100")
				return
			}
			limit = n
		}
		es, next, err := s.Ledger(r.Context(), auth.UID(r.Context()), limit, r.URL.Query().Get("cursor"))
		if errors.Is(err, coins.ErrBadCursor) {
			httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "bad cursor")
			return
		}
		if err != nil {
			internal(w, r, "ledger", err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": es, "nextCursor": next})
	})
	mux.HandleFunc("PUT /v1/me/coins/settings", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Overflow *coins.OverflowSettings `json:"overflow"`
		}
		if !httpx.DecodeBody(w, r, &req) {
			return
		}
		if req.Overflow == nil {
			httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "overflow is required")
			return
		}
		v, err := s.SetOverflow(r.Context(), auth.UID(r.Context()), auth.Anonymous(r.Context()), *req.Overflow)
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, v)
		case errors.Is(err, coins.ErrOff):
			httpx.Error(w, http.StatusNotFound, httpx.CodeNotFound, "coins are not available")
		case errors.Is(err, coins.ErrBadCap):
			httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		default:
			internal(w, r, "settings", err)
		}
	})
	mux.HandleFunc("POST /v1/me/coins/apple", func(w http.ResponseWriter, r *http.Request) {
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
		c, err := s.CreditApple(r.Context(), auth.UID(r.Context()), auth.Anonymous(r.Context()), req.SignedTransaction)
		var verr *appstore.Error
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, c)
		case errors.Is(err, coins.ErrOff):
			httpx.Error(w, http.StatusNotFound, httpx.CodeNotFound, "coins are not available")
		case errors.Is(err, coins.ErrAnonymous):
			httpx.ErrorWithDetails(w, http.StatusForbidden, httpx.CodeForbidden, "sign in to buy coins", map[string]any{"reason": "anonymous"})
		case errors.As(err, &verr):
			slog.WarnContext(r.Context(), "coins: rejected transaction", slog.String("reason", verr.Reason), slog.Any("error", verr.Err))
			httpx.ErrorWithDetails(w, http.StatusUnprocessableEntity, httpx.CodeInvalidTransaction,
				"the signed transaction could not be verified", map[string]any{"reason": verr.Reason})
		case errors.Is(err, coins.ErrOwnedByOther):
			httpx.Error(w, http.StatusConflict, httpx.CodeTransactionOwnedByOther, "this purchase was credited to another account")
		default:
			internal(w, r, "credit", err)
		}
	})
}

// Public mounts GET /v1/coins/config (no authentication): the display
// numbers (packs, estimates, grants, allowance) read once from env.
func Public(cfg coins.Config) func(*http.ServeMux) {
	body := cfg.Public()
	return func(mux *http.ServeMux) {
		mux.HandleFunc("GET /v1/coins/config", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "public, max-age=300")
			httpx.WriteJSON(w, http.StatusOK, body)
		})
	}
}

func internal(w http.ResponseWriter, r *http.Request, what string, err error) {
	slog.ErrorContext(r.Context(), "coins: "+what+" failed", slog.Any("error", err))
	httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
}
