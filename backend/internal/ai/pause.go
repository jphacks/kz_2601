package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// PausedMessage is the body message of 503 ai_paused. The app shows its own
// localized text for the code; this one is for other callers and logs.
const PausedMessage = "AI を一時的に止めています。しばらくしてからもう一度お試しください"

// PausedFromEnv reports whether AI_PAUSED turns the switch on (#511).
func PausedFromEnv(getenv func(string) string) bool {
	switch getenv("AI_PAUSED") {
	case "1", "true", "on":
		return true
	}
	return false
}

// Pause answers 503 ai_paused to every route that calls Claude while paused
// is true. It sits in front of the rate limiter so nothing is charged; health,
// DELETE /v1/chat/{appId} and generate mode=undo pass through (#511).
func Pause(paused bool, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !paused {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !callsClaude(r) {
				next.ServeHTTP(w, r)
				return
			}
			if logger != nil {
				logger.Info("ai.paused", "method", r.Method, "path", r.URL.Path)
			}
			httpx.Error(w, http.StatusServiceUnavailable, httpx.CodeAIPaused, PausedMessage)
		})
	}
}

// callsClaude reports whether the request reaches Claude. For generate it
// peeks at the body (mode=undo restores a version without Claude) and hands
// the body on unchanged.
func callsClaude(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/v1/chat", "/v1/text":
		return true
	case "/v1/generate":
		head, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(head), r.Body))
		var req struct {
			Mode string `json:"mode"`
		}
		_ = json.Unmarshal(head, &req)
		return req.Mode != "undo"
	}
	return false
}
