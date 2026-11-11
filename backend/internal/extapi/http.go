package extapi

import (
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// The runner's routes. /internal/* is called by appconfig only (Cloud Run
// IAM: the invoker is appconfig's SA, OIDC); appconfig has already checked
// the person (ID token, App Check), the app's published DSL and the args'
// types. /oauth/callback/{provider} is the one route a browser reaches
// (ASWebAuthenticationSession), checked by its one-use state.
//
// The internal answer is {"reason": "<Reason*>"} with the status of
// StatusOf; appconfig turns the reason into its error code (the contract
// child issue adds ext_not_connected / ext_unavailable / ext_busy).

// StatusOf is the HTTP status of a reason.
func StatusOf(reason string) int {
	switch reason {
	case ReasonOK:
		return http.StatusOK
	case ReasonBadRequest:
		return http.StatusBadRequest
	case ReasonNotConnected, ReasonDuplicateRequest:
		return http.StatusConflict
	case ReasonUnavailable, ReasonBusy:
		return http.StatusServiceUnavailable
	case ReasonProRequired, ReasonAllowanceExhausted, ReasonOverflowCap, ReasonCoinsInsufficient:
		return http.StatusPaymentRequired
	case ReasonUpstreamTimeout:
		return http.StatusGatewayTimeout
	case ReasonUpstream:
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// CallBody is POST /internal/ext/call.
type CallBody struct {
	Provider         string            `json:"provider"`
	Op               string            `json:"op"`
	Args             map[string]string `json:"args"`
	AppID            string            `json:"appId"`
	CreatorUID       string            `json:"creatorUid"`
	CreatorAnonymous bool              `json:"creatorAnonymous"`
	UserUID          string            `json:"userUid"`
	UserAnonymous    bool              `json:"userAnonymous"`
	RequestID        string            `json:"requestId"`
}

// CallAnswer is the 200 of POST /internal/ext/call.
type CallAnswer struct {
	Result Output `json:"result"`
	// CoinsCharged / CoinsBalance become X-Coins-Charged / X-Coins-Balance
	// on appconfig's answer (paid providers).
	CoinsCharged int64 `json:"coinsCharged"`
	CoinsBalance int64 `json:"coinsBalance"`
}

// ConnBody is the body of the /internal/ext/connections/* routes.
type ConnBody struct {
	UID      string   `json:"uid"`
	Provider string   `json:"provider"`
	Scopes   []string `json:"scopes,omitempty"`
}

func (s *Service) fail(w http.ResponseWriter, err error) {
	reason := Reason(err)
	httpx.WriteJSON(w, StatusOf(reason), map[string]string{"reason": reason})
}

// Routes mounts the external API routes on mux.
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/ext/call", func(w http.ResponseWriter, r *http.Request) {
		var b CallBody
		if !httpx.DecodeBody(w, r, &b) {
			return
		}
		res, err := s.Call(r.Context(), CallRequest{Provider: b.Provider, Op: b.Op, Args: b.Args, AppID: b.AppID,
			CreatorUID: b.CreatorUID, CreatorAnonymous: b.CreatorAnonymous, UserUID: b.UserUID, UserAnonymous: b.UserAnonymous, RequestID: b.RequestID})
		if err != nil {
			if Reason(err) == ReasonInternal {
				s.Log.ErrorContext(r.Context(), "extapi: call failed", slog.String("provider", b.Provider), slog.String("op", b.Op), slog.Any("error", err))
			}
			s.fail(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, CallAnswer{Result: res.Output, CoinsCharged: res.Charge.Coins, CoinsBalance: res.Charge.Balance})
	})
	mux.HandleFunc("POST /internal/ext/connections/start", func(w http.ResponseWriter, r *http.Request) {
		var b ConnBody
		if !httpx.DecodeBody(w, r, &b) {
			return
		}
		u, err := s.Start(r.Context(), b.UID, b.Provider, b.Scopes)
		if err != nil {
			s.fail(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"authUrl": u})
	})
	mux.HandleFunc("POST /internal/ext/connections/revoke", func(w http.ResponseWriter, r *http.Request) {
		var b ConnBody
		if !httpx.DecodeBody(w, r, &b) {
			return
		}
		if err := s.Revoke(r.Context(), b.UID, b.Provider); err != nil {
			s.fail(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
	})
	mux.HandleFunc("POST /internal/ext/connections/revokeAll", func(w http.ResponseWriter, r *http.Request) {
		var b ConnBody
		if !httpx.DecodeBody(w, r, &b) {
			return
		}
		n, err := s.RevokeAll(r.Context(), b.UID)
		if err != nil {
			s.fail(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]int{"revoked": n})
	})
	mux.HandleFunc("POST /internal/ext/connections/list", func(w http.ResponseWriter, r *http.Request) {
		var b ConnBody
		if !httpx.DecodeBody(w, r, &b) {
			return
		}
		out := ConnectionList{Connections: []ConnectionView{}, Enabled: s.EnabledProviders()}
		if s.Conns != nil && b.UID != "" {
			list, err := s.Conns.List(r.Context(), b.UID)
			if err != nil {
				s.fail(w, err)
				return
			}
			out.Connections = list
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /internal/ext/expire", func(w http.ResponseWriter, r *http.Request) {
		if s.Conns == nil {
			httpx.WriteJSON(w, http.StatusOK, map[string]int{"deleted": 0})
			return
		}
		n, err := s.Conns.ExpireUnused(r.Context(), 500)
		if err != nil {
			s.fail(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]int{"deleted": n})
	})
	mux.HandleFunc("GET /oauth/callback/{provider}", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		to := s.Callback(r.Context(), r.PathValue("provider"), q.Get("state"), q.Get("code"))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, to, http.StatusFound)
	})
}
