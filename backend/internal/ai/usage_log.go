package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

// UsageEvent is the message (and `event` attr) of the per-call usage log (#298).
// Cloud Logging filter: jsonPayload.event="ai.usage".
const UsageEvent = "ai.usage"

// Operation names in the ai.usage log, set by the call site via WithOperation.
const (
	OpGenerate  = "generate"   // create turn: full DSL
	OpPatch     = "patch"      // edit turn: JSON Patch
	OpLogic     = "logic"      // calculations pass (#100)
	OpChat      = "chat"       // clarify turn
	OpChatFirst = "chat_first" // first chat turn that may already build
	OpText      = "text"       // aiButton text generation (#417)
	OpUnknown   = "unknown"    // unlabeled call (eval/smoke tools)
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

// UIDHash is the pseudonymous user key of the usage log, never the raw uid.
// The prefix keeps it unlinkable to hashes other tools log (cmd/purge).
// Empty uid (anonymous caller) gives "".
func UIDHash(uid string) string {
	if uid == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("ai-usage:" + uid))
	return hex.EncodeToString(sum[:8])
}

// PricingAsOf is the date modelPrices was last checked against the pricing page.
const PricingAsOf = "2026-10-30"

// modelPrice is USD per million tokens.
type modelPrice struct {
	Input, Output, CacheWrite5m, CacheRead float64
}

// modelPrices is the only pricing table. Check every row against
// https://platform.claude.com/docs/en/about-claude/pricing before relying on
// the cost or adding a model, then bump PricingAsOf. A model id missing here
// logs no cost rather than a wrong one. The client only writes 5-minute cache
// entries, so cache_creation is priced at the 5m write rate. Claude Haiku 5.5
// is left out on purpose: its price depends on the prompt length.
var modelPrices = map[string]modelPrice{
	"claude-sonnet-5-5": {Input: 2, Output: 10, CacheWrite5m: 2.5, CacheRead: 0.10},
	"claude-sonnet-5":   {Input: 2, Output: 10, CacheWrite5m: 2.5, CacheRead: 0.20},
	"claude-opus-5-5":   {Input: 4, Output: 20, CacheWrite5m: 5, CacheRead: 0.20},
	"claude-opus-5":     {Input: 5, Output: 25, CacheWrite5m: 6.25, CacheRead: 0.50},
}

// EstimateCostUSD prices u at model's list price; ok=false for an unknown model.
func EstimateCostUSD(model string, u Usage) (usd float64, ok bool) {
	p, ok := modelPrices[model]
	if !ok {
		return 0, false
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

// logUsage writes one ai.usage line. Only labels, counts and the uid hash:
// never prompt/response text or the raw uid.
func (c *Client) logUsage(ctx context.Context, r usageRecord) {
	lg := c.Logger
	if lg == nil {
		lg = slog.Default()
	}
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
	if h := UIDHash(auth.UID(ctx)); h != "" {
		attrs = append(attrs, slog.String("uid_hash", h))
	}
	// the model that answered is the one billed (a fallback may differ)
	priced := r.model
	if respModel != "" {
		priced = respModel
	}
	if usd, ok := EstimateCostUSD(priced, u); ok {
		attrs = append(attrs, slog.Float64("est_cost_usd", usd), slog.String("pricing_as_of", PricingAsOf))
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
