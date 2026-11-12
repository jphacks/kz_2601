package ai

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// UsageEvent is the message (and `event` attr) of the per-call usage log (#298).
// Cloud Logging filter: jsonPayload.event="ai.usage".
const UsageEvent = "ai.usage"

// Operation names in the ai.usage log, set by the call site via WithOperation.
const (
	OpGenerate    = "generate"     // create turn: full DSL
	OpPatch       = "patch"        // edit turn: JSON Patch
	OpLogic       = "logic"        // calculations pass (#100)
	OpServerRules = "server_rules" // server-rules pass (#343)
	OpChat        = "chat"         // clarify turn
	OpChatFirst   = "chat_first"   // first chat turn that may already build
	OpText        = "text"         // aiButton text generation (#417)
	OpUnknown     = "unknown"      // unlabeled call (eval/smoke tools)
)

type operationKey struct{}

// WithOperation labels the Claude calls made with ctx for the usage log.
func WithOperation(ctx context.Context, op string) context.Context {
	return context.WithValue(ctx, operationKey{}, op)
}

func operationOf(ctx context.Context) string {
	if op, _ := ctx.Value(operationKey{}).(string); op != "" {
		return op
	}
	return OpUnknown
}

type clientPlatformKey struct{}

// WithClientPlatform labels the Claude calls made with ctx with the caller's
// OS (raw X-Client-Platform, normalised by httpx.NormalizePlatform).
func WithClientPlatform(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, clientPlatformKey{}, httpx.NormalizePlatform(raw))
}

// clientPlatformOf is "" for calls made outside an HTTP request (batch, ai-eval).
func clientPlatformOf(ctx context.Context) string {
	p, _ := ctx.Value(clientPlatformKey{}).(string)
	return p
}

// ClientPlatformMiddleware puts the request's OS on its context for the usage
// log. A request without the header is "other", as in the request log.
func ClientPlatformMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithClientPlatform(r.Context(), r.Header.Get(httpx.ClientPlatformHeader))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UIDHash is the pseudonymous user key of the usage log, never the raw uid.
// Same as httpx.UIDHash (request logs, #295); its prefix keeps it unlinkable
// to hashes other tools log (cmd/purge). Empty uid (anonymous caller) gives "".
func UIDHash(uid string) string { return httpx.UIDHash(uid) }

// PricingAsOf is the date modelPrices was last checked against the pricing page.
const PricingAsOf = "2026-11-09"

// modelPrice is USD per million tokens.
type modelPrice struct {
	Input, Output, CacheWrite5m, CacheRead float64
}

// modelPrices is the only pricing table. Check every row against
// https://platform.claude.com/docs/en/about-claude/pricing before relying on
// the cost or adding a model, then bump PricingAsOf. A model id missing here
// logs no cost rather than a wrong one. The client only writes 5-minute cache
// entries, so cache_creation is priced at the 5m write rate. Claude Haiku 5.5
// has two rate cards by prompt length: its row here is the one for prompts up
// to haikuShortPromptTokens, haikuLongPrice the one above (EstimateCostUSD).
// Both are from the model's release notes (input / output, cache reads 0.1x,
// 5m writes 1.25x): confirm them on the pricing page before relying on them.
var modelPrices = map[string]modelPrice{
	"claude-haiku-5-5":  {Input: 0.10, Output: 0.50, CacheWrite5m: 0.125, CacheRead: 0.01},
	"claude-sonnet-5-5": {Input: 2, Output: 10, CacheWrite5m: 2.5, CacheRead: 0.10},
	"claude-sonnet-5":   {Input: 2, Output: 10, CacheWrite5m: 2.5, CacheRead: 0.20},
	"claude-opus-5-5":   {Input: 4, Output: 20, CacheWrite5m: 5, CacheRead: 0.20},
	"claude-opus-5":     {Input: 5, Output: 25, CacheWrite5m: 6.25, CacheRead: 0.50},
}

// Claude Haiku 5.5 above haikuShortPromptTokens prompt tokens (input + cache).
const haikuShortPromptTokens = 100_000

var haikuLongPrice = modelPrice{Input: 0.50, Output: 2.50, CacheWrite5m: 0.625, CacheRead: 0.05}

// EstimateCostUSD prices u at model's list price; ok=false for an unknown model.
func EstimateCostUSD(model string, u Usage) (usd float64, ok bool) {
	p, ok := modelPrices[model]
	if !ok {
		return 0, false
	}
	if model == "claude-haiku-5-5" && u.InputTokens+u.CacheReadInputTokens+u.CacheCreationInputTokens > haikuShortPromptTokens {
		p = haikuLongPrice
	}
	return (float64(u.InputTokens)*p.Input +
		float64(u.OutputTokens)*p.Output +
		float64(u.CacheCreationInputTokens)*p.CacheWrite5m +
		float64(u.CacheReadInputTokens)*p.CacheRead) / 1e6, true
}

// usageRecord is one HTTP attempt against the Messages API.
type usageRecord struct {
	model   string // requested
	stream  bool
	attempt int // 1-based
	resp    *Response
	err     error
	latency time.Duration
}

// logger is c.Logger, or slog.Default() when unset.
func (c *Client) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// logUsage writes one ai.usage line. Only labels, counts and the uid hash:
// never prompt/response text or the raw uid.
func (c *Client) logUsage(ctx context.Context, r usageRecord) {
	lg := c.logger()
	var u Usage
	stop, respModel := "", ""
	if r.resp != nil {
		u, stop, respModel = r.resp.Usage, r.resp.StopReason, r.resp.Model
	}
	outcome := "ok"
	switch {
	case r.err != nil:
		outcome = "error"
	case stop == "refusal":
		outcome = "refusal"
	}
	attrs := []slog.Attr{
		slog.String("event", UsageEvent),
		slog.String("operation", operationOf(ctx)),
		slog.String("model", r.model),
		slog.Bool("stream", r.stream),
		slog.Int("attempt", r.attempt),
		slog.String("outcome", outcome),
		slog.Int("input_tokens", u.InputTokens),
		slog.Int("output_tokens", u.OutputTokens),
		slog.Int("cache_creation_input_tokens", u.CacheCreationInputTokens),
		slog.Int("cache_read_input_tokens", u.CacheReadInputTokens),
		slog.Int64("latency_ms", r.latency.Milliseconds()),
	}
	if respModel != "" {
		attrs = append(attrs, slog.String("response_model", respModel))
	}
	if stop != "" {
		attrs = append(attrs, slog.String("stop_reason", stop))
	}
	if r.err != nil {
		attrs = append(attrs, slog.String("error_kind", errorKind(r.err)))
	}
	if p := clientPlatformOf(ctx); p != "" {
		attrs = append(attrs, slog.String("clientPlatform", p))
	}
	if h := UIDHash(auth.UID(ctx)); h != "" {
		attrs = append(attrs, slog.String("uid_hash", h))
	}
	// a shared app's participant: the creator's Pro covers the call
	if p := payerOf(ctx); p != "" {
		attrs = append(attrs, slog.String("payer_uid_hash", UIDHash(p)))
	}
	// the model that answered is the one billed (a fallback may differ)
	priced := r.model
	if respModel != "" {
		priced = respModel
	}
	usd, ok := EstimateCostUSD(priced, u)
	if ok {
		attrs = append(attrs, slog.Float64("est_cost_usd", usd), slog.String("pricing_as_of", PricingAsOf))
	}
	if t := tallyOf(ctx); t != nil {
		t.add(u, usd, ok)
	}
	lg.LogAttrs(ctx, slog.LevelInfo, UsageEvent, attrs...)
}

// errorKind is a content-free error label: "<status>_<type>" for API errors.
func errorKind(err error) string {
	var ae *APIError
	switch {
	case errors.As(err, &ae):
		return strconv.Itoa(ae.Status) + "_" + ae.Type
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	return "error"
}
