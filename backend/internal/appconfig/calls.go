package appconfig

// Calls (#447): POST /v1/apps/{appId}/calls/token issues a LiveKit token for
// the room of a published callButton. Only the app's participants (its
// creator and the members) get one; the caller's own coins pay by the
// minute (internal/calls). Off (404 calls_disabled) unless CALLS_ENABLED.
// docs/backend/features/calls.md

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/calls"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// details.reason values of call errors.
const (
	reasonCallsDisabled = "calls_disabled"
	reasonNoCallButton  = "no_call_button" // the published app has no callButton for that room
	reasonCallEnded     = "call_ended"     // a renewal of a session that left the room
)

func init() {
	registerRoutes(func(h *Handler, mux *http.ServeMux) {
		if h.callUsers == nil {
			perHour := calls.DefaultTokensPerHour
			if h.calls != nil {
				perHour = h.calls.Config.TokensPerHour
			}
			h.callUsers = newFeedLimiter(perHour, time.Now)
		}
		mux.HandleFunc("POST /v1/apps/{appId}/calls/token", h.callToken)
	})
}

// WithCalls sets the call service (nil or CALLS_ENABLED off = 404).
func WithCalls(s *calls.Service) Option { return func(h *Handler) { h.calls = s } }

// WithCallLimit replaces the per-user token cap (tests).
func WithCallLimit(perHour int, now func() time.Time) Option {
	return func(h *Handler) { h.callUsers = newFeedLimiter(perHour, now) }
}

func (h *Handler) callToken(w http.ResponseWriter, r *http.Request) {
	if !h.calls.Enabled() {
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "calls are not available").With("reason", reasonCallsDisabled))
		return
	}
	appID, err := pathID(r, "appId")
	if err != nil {
		fail(w, err)
		return
	}
	var in struct {
		Room      string `json:"room"`
		Video     bool   `json:"video"`
		Name      string `json:"name"`
		SessionID string `json:"sessionId"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if !httpx.DecodeBody(w, r, &in) {
		return
	}
	if !dsl.ValidCallRoom(in.Room) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid room").With("field", "room"))
		return
	}
	if in.SessionID != "" && !idRe.MatchString(in.SessionID) {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "invalid sessionId").With("field", "sessionId"))
		return
	}
	a, pub, err := h.publishedDSL(r.Context(), appID)
	if err != nil {
		fail(w, err)
		return
	}
	if a == nil || !hasSharedEntity(a) {
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "this app is not shared, so it has no call").With("reason", reasonNoSharedData))
		return
	}
	if pub.Hidden {
		fail(w, unpublishedErr("this shared app", pub.ModerationState))
		return
	}
	btn := a.CallButton(in.Room)
	if btn == nil {
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "this app has no call for that room").With("reason", reasonNoCallButton))
		return
	}
	role, err := h.memberRole(r.Context(), appID, uidOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	if role == "" {
		fail(w, notMember())
		return
	}
	if wait, ok := h.callUsers.take(uidOf(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		httpx.ErrorWithDetails(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "しばらくしてからもう一度試してください。",
			map[string]any{"scope": "user", "limit": h.callUsers.perHour, "remaining": 0, "retryAfterSec": wait})
		return
	}
	j, err := h.calls.Join(r.Context(), calls.JoinRequest{
		UID: uidOf(r), Anonymous: auth.Anonymous(r.Context()), AppID: appID, Room: in.Room,
		Video: btn.Video && in.Video, Name: in.Name, SessionID: in.SessionID,
	})
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, j)
	case coins.WriteError(w, err): // 402 coins_insufficient (the caller's own coins)
	case errors.Is(err, calls.ErrSessionClosed):
		fail(w, apiErr(http.StatusNotFound, httpx.CodeNotFound, "this call has ended; join again").With("reason", reasonCallEnded))
	default:
		fail(w, err)
	}
}
