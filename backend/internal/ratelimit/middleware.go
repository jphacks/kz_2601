package ratelimit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

const maxPeekBytes = 1 << 20

// Weight returns the units a request to the ai service costs, or -1 when the
// route is not limited (health, DELETE /v1/chat/{appId}, ...).
func (w Weights) Weight(r *http.Request, body []byte) int {
	if r.Method != http.MethodPost {
		return -1
	}
	switch r.URL.Path {
	case "/v1/chat":
		return w.Chat
	case "/v1/generate":
		var req struct {
			Mode string `json:"mode"`
		}
		_ = json.Unmarshal(body, &req)
		switch req.Mode {
		case "undo": // restores the previous version, no Claude call
			return 0
		case "edit":
			return w.Edit
		default: // create / auto (auto may still turn out to be an edit)
			return w.Create
		}
	}
	return -1
}

// Middleware guards POST /v1/generate and POST /v1/chat: it verifies the
// Firebase ID token (401 without one), charges the request and answers 429
// `rate_limited` when a daily limit is reached. Other routes pass through.
// The body is read once and handed on unchanged.
func (l *Limiter) Middleware(verifier auth.IdentityVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l.cfg.Weights.Weight(r, nil) < 0 {
				next.ServeHTTP(w, r)
				return
			}
			token, ok := auth.BearerToken(r.Header.Get("Authorization"))
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "missing bearer token")
				return
			}
			id, err := verifier.VerifyIdentity(r.Context(), token)
			if err != nil || id.UID == "" {
				httpx.Error(w, http.StatusUnauthorized, httpx.CodeUnauthenticated, "invalid token")
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, maxPeekBytes))
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, httpx.CodeBadRequest, "cannot read body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			dec, err := l.Allow(r.Context(), id, ClientIP(r), l.cfg.Weights.Weight(r, body))
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "rate limiter unavailable")
				return
			}
			setHeaders(w, dec, l.cfg.Now())
			if !dec.Allowed {
				writeLimited(w, dec, l.cfg.Now())
				return
			}
			// verified here: auth.Optional in the handler keeps this uid instead of verifying again
			next.ServeHTTP(w, r.WithContext(auth.ContextWithUID(r.Context(), id.UID)))
		})
	}
}

func setHeaders(w http.ResponseWriter, d Decision, now time.Time) {
	h := w.Header()
	h.Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(d.ResetAt.Unix(), 10))
	if !d.Allowed {
		h.Set("Retry-After", strconv.Itoa(retryAfter(d, now)))
	}
}

func retryAfter(d Decision, now time.Time) int {
	return max(1, int(d.ResetAt.Sub(now).Seconds()+0.999))
}

// writeLimited answers 429 in the ai error envelope.
func writeLimited(w http.ResponseWriter, d Decision, now time.Time) {
	httpx.ErrorWithDetails(w, http.StatusTooManyRequests, httpx.CodeRateLimited, Message(d), map[string]any{
		"scope":         d.Scope,
		"tier":          string(d.Tier),
		"limit":         d.Limit,
		"used":          d.Used,
		"remaining":     d.Remaining,
		"resetAt":       d.ResetAt.UTC().Format(time.RFC3339),
		"retryAfterSec": retryAfter(d, now),
	})
}

// Message is the Japanese reason shown to the user.
func Message(d Decision) string {
	reset := d.ResetAt.In(JST)
	when := fmt.Sprintf("%d月%d日 %d:%02d", reset.Month(), reset.Day(), reset.Hour(), reset.Minute())
	switch d.Scope {
	case ScopeGlobal:
		return "本日は利用が集中したため、新しい生成の受付を止めています。" + when + "に再開します。"
	case ScopeIP:
		return "このネットワークからの本日の利用上限に達しました。サインインすると続けて使えます(" + when + "にリセット)。"
	}
	msg := fmt.Sprintf("今日の利用上限(%d)に達しました。%sからまた使えます。", d.Limit, when)
	if d.Tier == TierAnonymous {
		msg += "サインインすると上限が増えます。"
	}
	return msg
}

// ClientIP is the caller address. On Cloud Run the Google front end appends
// the real peer to X-Forwarded-For, so the last entry is the trustworthy one
// (anything before it can be forged by the client).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
