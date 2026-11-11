package triage

import (
	"context"
	"encoding/json"

	"github.com/jphacks/kz_2601/backend/internal/ai"
)

// AIModel calls Claude through internal/ai with structured output. The
// request carries no tools: ai.Completion has no field for them.
//
// Use a key and budget of the loop's own (#365 decision 7), not the app's.
type AIModel struct {
	C *ai.Client
	// MaxTokens per call (0 = 4000).
	MaxTokens int
}

// Complete implements Model.
func (m AIModel) Complete(ctx context.Context, system, user string, schema json.RawMessage) (string, Usage, error) {
	maxTokens := m.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	resp, err := m.C.Complete(ctx, ai.Completion{
		System:     system,
		Messages:   []ai.Message{{Role: "user", Content: user}},
		Schema:     schema,
		Structured: true,
		MaxTokens:  maxTokens,
	})
	if err != nil {
		return "", Usage{}, err
	}
	return resp.Text, Usage{InputTokens: resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.CacheCreationInputTokens,
		OutputTokens: resp.Usage.OutputTokens}, nil
}
