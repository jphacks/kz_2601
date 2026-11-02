package ai

import (
	"context"
	"log/slog"
	"sync"
)

// costTally sums the ai.usage attempts of one request so the request log
// (ai.generate) can carry the cost of the whole generation (#298): retries,
// failed attempts and the logic pass included.
type costTally struct {
	mu       sync.Mutex
	calls    int
	unpriced int
	usd      float64
	usage    Usage
}

type costTallyKey struct{}

// withCostTally attaches a fresh tally that Client.logUsage adds to.
func withCostTally(ctx context.Context) context.Context {
	return context.WithValue(ctx, costTallyKey{}, &costTally{})
}

func tallyOf(ctx context.Context) *costTally {
	t, _ := ctx.Value(costTallyKey{}).(*costTally)
	return t
}

func (t *costTally) add(u Usage, usd float64, priced bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	t.usage.Add(u)
	if priced {
		t.usd += usd
	} else {
		t.unpriced++
	}
}

// attrs are the ai.generate fields. est_cost_usd is left out when any
// attempt was unpriced: a partial sum would read as the full cost.
func (t *costTally) attrs() []slog.Attr {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.calls == 0 {
		return nil
	}
	a := []slog.Attr{
		slog.Int("claude_calls", t.calls),
		slog.Int("input_tokens", t.usage.InputTokens),
		slog.Int("output_tokens", t.usage.OutputTokens),
		slog.Int("cache_creation_input_tokens", t.usage.CacheCreationInputTokens),
		slog.Int("cache_read_input_tokens", t.usage.CacheReadInputTokens),
	}
	if t.unpriced > 0 {
		return append(a, slog.Int("unpriced_calls", t.unpriced))
	}
	return append(a, slog.Float64("est_cost_usd", t.usd), slog.String("pricing_as_of", PricingAsOf))
}
