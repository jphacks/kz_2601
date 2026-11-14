package ratelimit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	case "/v1/text":
		var req struct {
			Task   string          `json:"task"`
			Length string          `json:"length"`
			Image  json.RawMessage `json:"image"` // aiButton inputPhoto: not decoded here (nor aiChat messages)
		}
		_ = json.Unmarshal(body, &req)
		n := w.Text // the heaviest of task / long / image
		if req.Task != "" && req.Task != "generate" {
			n = max(n, w.TextTask)
		}
		if req.Length == "long" || req.Task == "chat" { // aiChat weighs like long (up to 16 KB of turns)
			n = max(n, w.TextLong)
		}
		if len(req.Image) > 0 && string(req.Image) != "null" {
			n = max(n, w.TextImage)
		}
		return n
	case "/v1/generate":
		var req struct {
			Mode string `json:"mode"`
		}
		_ = json.Unmarshal(body, &req)
		return w.modeWeight(req.Mode)
	}
	return -1
}

// modeWeight is the units of one generate mode. create / auto / "" weigh as
// create (auto may still turn out to be an edit: ReportMode refunds that).
func (w Weights) modeWeight(mode string) int {
	switch mode {
	case "undo": // restores the previous version, no Claude call
		return 0
	case "edit":
		return w.Edit
	default:
		return w.Create
	}
}

// Middleware guards POST /v1/generate, /v1/chat and /v1/text: it verifies the
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

			weight := l.cfg.Weights.Weight(r, body)
			// verified here: auth.Optional in the handler keeps this uid instead of verifying again
			// (anonymous too: an anonymous account gets no initial coins, #630); the
			// tier's Pro check sees it as the verified caller (#2085)
			ctx := auth.ContextWithIdentity(r.Context(), id)
			dec, err := l.AllowPlatform(ctx, id, ClientIP(r), httpx.ClientPlatform(r), weight)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, httpx.CodeInternal, "rate limiter unavailable")
				return
			}
			setHeaders(w, dec, l.cfg.Now())
			if !dec.Allowed {
				writeLimited(w, dec, l.cfg.Now())
				return
			}
			out := &outcome{}
			out.waive = func(ctx context.Context) bool { return l.waive(ctx, w, id.UID, ClientIP(r), dec, weight) }
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r.WithContext(context.WithValue(ctx, outcomeKey{}, out)))
			l.settle(r, id.UID, dec, weight, out, sw.status)
		})
	}
}

// settle refunds what the request should not pay after all (refund.go, #548).
// It runs once per request, so a request is refunded at most once.
func (l *Limiter) settle(r *http.Request, uid string, dec Decision, charged int, out *outcome, httpStatus int) {
	status, mode, unchanged, waived := out.final(httpStatus)
	if waived {
		return
	}
	gone := r.Context().Err() != nil // the client disconnected before the end
	units, reason := l.cfg.Weights.refundUnits(charged, status, mode, unchanged, gone)
	if units <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), refundTimeout)
	defer cancel()
	var taken unchangedTake
	if reason == refundUnchanged {
		var ok bool
		if taken, ok = l.takeUnchanged(ctx, dec.charge.store, uid); !ok {
			// past the daily cap: pay the resolved mode as usual
			if units, reason = l.cfg.Weights.refundUnits(charged, status, mode, false, gone); units <= 0 {
				return
			}
		}
	}
	if err := l.Refund(ctx, dec, units); err != nil {
		l.log.WarnContext(ctx, "ratelimit: refund failed", slog.Any("error", err), slog.Int("units", units))
		l.untakeUnchanged(ctx, dec.charge.store, taken) // not refunded after all
		return
	}
	l.log.InfoContext(ctx, "ratelimit.refund",
		slog.Int("units", units),
		slog.Int("status", status),
		slog.String("mode", mode),
		slog.String("reason", reason),
		slog.String("path", r.URL.Path),
	)
}

// waive refunds all of dec at once (Waive) and rewrites the headers that
// are not sent yet to the count without this request. Past the hourly caps
// of waived requests per uid and per ip (waive_cap.go) the request keeps its charge.
func (l *Limiter) waive(ctx context.Context, w http.ResponseWriter, uid, ip string, dec Decision, charged int) bool {
	if charged <= 0 || dec.charge.store == nil {
		return false
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refundTimeout)
	defer cancel()
	taken, ok := l.takeWaive(rctx, dec.charge.store, uid, ip)
	if !ok {
		return false
	}
	if err := l.Refund(rctx, dec, charged); err != nil {
		l.log.WarnContext(rctx, "ratelimit: waive failed", slog.Any("error", err), slog.Int("units", charged))
		l.untakeWaive(rctx, dec.charge.store, taken) // not a waive after all
		return false
	}
	units := min(charged, dec.charge.weight)
	if !dec.Unmetered {
		dec.Used = max(0, dec.Used-units)
		dec.Remaining = max(0, dec.Limit-dec.Used)
		setHeaders(w, dec, l.cfg.Now())
	}
	l.log.InfoContext(rctx, "ratelimit.refund", slog.Int("units", units), slog.String("reason", "waived"))
	return true
}

// setHeaders writes X-RateLimit-* (and Retry-After on 429). An Unmetered
// allowed request gets none: "remaining 0" or "remaining = limit" would both
// mislead the client's "今日の残り" (#630).
func setHeaders(w http.ResponseWriter, d Decision, now time.Time) {
	h := w.Header()
	if d.Allowed && d.Unmetered {
		return
	}
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
