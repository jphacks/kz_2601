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
	// pending counts Claude calls still running after the response (an
	// early chat reply, track); idle is closed when it drops to 0.
	pending int
	idle    chan struct{}
}

// track counts a call that may outlive the response until done is called
// (the coin charge waits for it). No-op on a nil tally.
func (t *costTally) track() (done func()) {
	if t == nil {
		return func() {}
	}
	t.mu.Lock()
	t.pending++
	t.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.pending--; t.pending == 0 && t.idle != nil {
				close(t.idle)
				t.idle = nil
			}
		})
	}
}

// busy reports whether a tracked call is still running.
func (t *costTally) busy() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pending > 0
}

// wait blocks until no tracked call is running or ctx ends.
func (t *costTally) wait(ctx context.Context) {
	t.mu.Lock()
	if t.pending == 0 {
		t.mu.Unlock()
		return
	}
	if t.idle == nil {
		t.idle = make(chan struct{})
	}
	idle := t.idle
	t.mu.Unlock()
	select {
	case <-idle:
	case <-ctx.Done():
	}
}

// total is the cost so far: calls made, their USD and whether every call
// had a price (EstimateCostUSD ok).
func (t *costTally) total() (calls int, usd float64, priced bool) {
	if t == nil {
		return 0, 0, true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls, t.usd, t.unpriced == 0
}

type costTallyKey struct{}

// withCostTally attaches a fresh tally that Client.logUsage adds to. A ctx
// that already has one (the coin charge of the request) keeps it.
func withCostTally(ctx context.Context) context.Context {
	if tallyOf(ctx) != nil {
		return ctx
	}
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
