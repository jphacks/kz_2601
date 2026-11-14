package calls

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// WebhookPath is the URL registered in the LiveKit Cloud project.
const WebhookPath = "/v1/calls/webhook"

// maxWebhookBytes bounds a webhook body (a few KB).
const maxWebhookBytes = 64 << 10

// Public mounts POST /v1/calls/webhook (no Firebase auth: LiveKit signs the
// body with the API secret, checked by the LiveKit SDK). Nothing is mounted
// while calls are off.
func Public(s *Service) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		if !s.Enabled() {
			return
		}
		mux.Handle("POST "+WebhookPath, s.Webhook())
	}
}

// Webhook verifies and applies a LiveKit webhook. A bad signature is 401;
// a failed settlement is 500 so LiveKit sends it again (settling is
// idempotent).
func (s *Service) Webhook() http.Handler {
	keys := auth.NewSimpleKeyProvider(s.Config.APIKey, s.Config.APISecret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBytes)
		ev, err := webhook.ReceiveWebhookEvent(r, keys)
		if err != nil {
			slog.WarnContext(r.Context(), "calls: webhook rejected", slog.Any("error", err))
			httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "invalid webhook signature")
			return
		}
		if err := s.Handle(r.Context(), eventOf(ev)); err != nil {
			slog.ErrorContext(r.Context(), "calls: webhook failed", slog.String("kind", ev.GetEvent()), slog.Any("error", err))
			httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// eventOf keeps what settlement needs from a LiveKit event.
func eventOf(ev *livekit.WebhookEvent) Event {
	e := Event{Kind: ev.GetEvent(), Room: ev.GetRoom().GetName(), At: unix(ev.GetCreatedAt(), 0)}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if p := ev.GetParticipant(); p != nil {
		e.Identity = p.GetIdentity()
		e.JoinedAt = unix(p.GetJoinedAt(), p.GetJoinedAtMs())
	}
	return e
}

func unix(sec, ms int64) time.Time {
	switch {
	case ms > 0:
		return time.UnixMilli(ms).UTC()
	case sec > 0:
		return time.Unix(sec, 0).UTC()
	}
	return time.Time{}
}
