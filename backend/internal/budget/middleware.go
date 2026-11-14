package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/ratelimit"
)

// PausedEvent is logged for every request the breaker answers.
const PausedEvent = "budget.free_paused"

// Middleware answers 503 free_plan_paused to free callers of POST
// /v1/generate (not mode undo: no Claude call) and POST /v1/chat while
// Paused. It sits inside the rate limiter, which verified the caller
// (auth.UID) and gives the units back on a 503. /v1/text is the Pro
// feature onlineAI and is left alone. Pro callers pass.
func (b *Breaker) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !b.cfg.Enabled() {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !guarded(r) {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			paused, reset := b.Paused(ctx)
			if !paused {
				next.ServeHTTP(w, r)
				return
			}
			uid := auth.UID(ctx)
			if uid != "" && b.pro != nil && b.pro.IsPro(ctx, uid) {
				next.ServeHTTP(w, r)
				return
			}
			if r.URL.Path == "/v1/generate" && isUndo(r) {
				next.ServeHTTP(w, r)
				return
			}
			b.log.InfoContext(ctx, PausedEvent,
				slog.String("event", PausedEvent),
				slog.String("path", r.URL.Path),
				slog.String("uid_hash", httpx.UIDHash(uid)),
				slog.Float64("threshold_usd", b.cfg.DailyUSD),
			)
			writePaused(w, reset, b.cfg.Now())
		})
	}
}

func guarded(r *http.Request) bool {
	return r.Method == http.MethodPost && (r.URL.Path == "/v1/generate" || r.URL.Path == "/v1/chat")
}

// isUndo peeks at the JSON body's mode and hands the body on unchanged.
func isUndo(r *http.Request) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), r.Body))
	if err != nil {
		return false
	}
	var req struct {
		Mode string `json:"mode"`
	}
	_ = json.Unmarshal(body, &req)
	return req.Mode == "undo"
}

func writePaused(w http.ResponseWriter, reset, now time.Time) {
	retry := max(1, int(reset.Sub(now).Seconds()+0.999))
	w.Header().Set("Retry-After", strconv.Itoa(retry))
	httpx.ErrorWithDetails(w, http.StatusServiceUnavailable, httpx.CodeFreePlanPaused, Message(reset), map[string]any{
		"reason":        "daily_budget",
		"plan":          "free",
		"resumeAt":      reset.UTC().Format(time.RFC3339),
		"retryAfterSec": retry,
	})
}

// Message is the Japanese reason of free_plan_paused.
func Message(reset time.Time) string {
	reset = reset.In(ratelimit.JST)
	when := fmt.Sprintf("%d月%d日 %d:%02d", reset.Month(), reset.Day(), reset.Hour(), reset.Minute())
	return "本日は利用が集中したため、無料プランの新しい生成を一時停止しています。" + when + "に再開します。Tailor It Pro なら続けて使えます。"
}

// Tap returns next with every log record whose message is event (ai.usage)
// also adding its est_cost_usd to today's total. Records without a cost
// (unpriced models) add nothing.
func (b *Breaker) Tap(next slog.Handler, event string) slog.Handler {
	if !b.cfg.Enabled() {
		return next
	}
	return &tap{next: next, b: b, event: event}
}

type tap struct {
	next  slog.Handler
	b     *Breaker
	event string
}

func (t *tap) Enabled(ctx context.Context, l slog.Level) bool { return t.next.Enabled(ctx, l) }

func (t *tap) Handle(ctx context.Context, rec slog.Record) error {
	if rec.Message == t.event {
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key != "est_cost_usd" {
				return true
			}
			if a.Value.Kind() == slog.KindFloat64 {
				t.b.Record(ctx, a.Value.Float64())
			}
			return false
		})
	}
	return t.next.Handle(ctx, rec)
}

func (t *tap) WithAttrs(as []slog.Attr) slog.Handler {
	return &tap{next: t.next.WithAttrs(as), b: t.b, event: t.event}
}

func (t *tap) WithGroup(name string) slog.Handler {
	return &tap{next: t.next.WithGroup(name), b: t.b, event: t.event}
}
