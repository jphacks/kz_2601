package ai

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
)

func bodyWithText(model, text string) string {
	b, _ := json.Marshal(map[string]any{
		"model":       model,
		"content":     []map[string]any{{"type": "text", "text": text}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 1000, "output_tokens": 500, "cache_creation_input_tokens": 2000, "cache_read_input_tokens": 4000},
	})
	return string(b)
}

func attrMap(as []slog.Attr) map[string]any {
	m := map[string]any{}
	for _, a := range as {
		m[a.Key] = a.Value.Any()
	}
	return m
}

func TestCostTally_SumsRetriedAttempts(t *testing.T) {
	srv, _ := fakeAnthropic(t,
		[2]string{"500", `{"type":"error","error":{"type":"api_error","message":"x"}}`},
		[2]string{"200", okBody("claude-sonnet-5-5")})
	c, _ := testClient(srv, "claude-sonnet-5-5")
	ctx := withCostTally(context.Background())
	if _, err := c.Complete(ctx, userCompletion()); err != nil {
		t.Fatal(err)
	}
	m := attrMap(tallyOf(ctx).attrs())
	if m["claude_calls"] != int64(2) || m["input_tokens"] != int64(1000) || m["cache_read_input_tokens"] != int64(4000) {
		t.Fatalf("tally: %v", m)
	}
	if got, _ := m["est_cost_usd"].(float64); math.Abs(got-0.0124) > 1e-12 {
		t.Fatalf("est_cost_usd = %v, want 0.0124", m["est_cost_usd"])
	}
}

func TestCostTally_UnpricedDropsCost(t *testing.T) {
	srv, _ := fakeAnthropic(t, [2]string{"200", okBody("claude-unknown-9")})
	c, _ := testClient(srv, "claude-unknown-9")
	ctx := withCostTally(context.Background())
	if _, err := c.Complete(ctx, userCompletion()); err != nil {
		t.Fatal(err)
	}
	m := attrMap(tallyOf(ctx).attrs())
	if _, ok := m["est_cost_usd"]; ok || m["unpriced_calls"] != int64(1) {
		t.Fatalf("unpriced tally: %v", m)
	}
}

func TestCostTally_NoCallsNoFields(t *testing.T) {
	if a := tallyOf(context.Background()).attrs(); a != nil {
		t.Fatalf("nil tally: %v", a)
	}
	if a := tallyOf(withCostTally(context.Background())).attrs(); a != nil {
		t.Fatalf("empty tally: %v", a)
	}
}

// The ai.generate line carries the tokens and cost of the whole request.
func TestGenerateLog_CarriesRequestCost(t *testing.T) {
	kb := kakeibo(t)
	srv, n := fakeAnthropic(t, [2]string{"200", bodyWithText("claude-sonnet-5-5", string(kb))})
	c, _ := testClient(srv, "claude-sonnet-5-5")
	c.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc, _ := newSvc(&fakeCaller{})
	svc.Claude = c
	ctx := auth.ContextWithUID(context.Background(), rawUID)
	m, _ := genLog(t, svc, func() {
		if _, err := svc.Generate(ctx, Request{AppID: "app-1", Transcript: "家計簿"}); err != nil {
			t.Fatal(err)
		}
	})
	calls := float64(n.Load())
	if m["claude_calls"] != calls || m["input_tokens"] != 1000*calls || m["pricing_as_of"] != PricingAsOf {
		t.Fatalf("ai.generate cost fields: %v", m)
	}
	if got, _ := m["est_cost_usd"].(float64); math.Abs(got-0.0124*calls) > 1e-9 {
		t.Fatalf("est_cost_usd = %v, want %v", m["est_cost_usd"], 0.0124*calls)
	}
}
