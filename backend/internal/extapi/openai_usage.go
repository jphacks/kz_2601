package extapi

import (
	"encoding/json"
	"errors"
	"time"
)

// OpenAI's organization usage (#2138): GET
// /v1/organization/usage/completions with bucket_width=1d and
// group_by=model. This file turns one page of it into the same UsageRows
// as Claude's (usage_rows.go); the ops come with #2138's second half.

// openaiUsageAnswer is one page. Fields not listed (input_audio_tokens,
// num_model_requests, project_id, batch ...) are ignored.
type openaiUsageAnswer struct {
	Data []struct {
		StartTime *int64 `json:"start_time"`
		Results   []struct {
			Model  *string `json:"model"`
			Input  int64   `json:"input_tokens"`
			Output int64   `json:"output_tokens"`
			Cached int64   `json:"input_cached_tokens"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage *string `json:"next_page"`
}

// parseOpenAIUsage reads one page of the completions usage. OpenAI's
// input_tokens include the cached ones: InputTokens is the rest, so the
// row adds up the same way as Claude's. OpenAI does not count cache
// writes (CacheWrite 0).
func parseOpenAIUsage(body []byte) (usagePage[UsageRow], error) {
	var a openaiUsageAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		return usagePage[UsageRow]{}, errors.New("openai usage: the answer is not the expected JSON")
	}
	p := usagePage[UsageRow]{HasMore: a.HasMore}
	if a.NextPage != nil {
		p.NextPage = *a.NextPage
	}
	if p.HasMore && p.NextPage == "" {
		return usagePage[UsageRow]{}, errors.New("openai usage: has_more without next_page")
	}
	for _, b := range a.Data {
		if b.StartTime == nil || *b.StartTime < 0 {
			return usagePage[UsageRow]{}, errors.New("openai usage: a bucket without start_time")
		}
		day := time.Unix(*b.StartTime, 0).UTC().Format(time.DateOnly)
		for _, r := range b.Results {
			cached := max(min(r.Cached, r.Input), 0)
			row := UsageRow{Day: day, InputTokens: max(r.Input-cached, 0), OutputTokens: r.Output, CacheRead: cached}
			if r.Model != nil {
				row.Model = *r.Model
			}
			p.Rows = append(p.Rows, row)
		}
	}
	return p, nil
}
