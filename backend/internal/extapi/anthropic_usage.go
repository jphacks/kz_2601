package extapi

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Anthropic's Admin API usage report (#2137): GET
// /v1/organizations/usage_report/messages with bucket_width=1d and
// group_by[]=model. This file turns one page of it into UsageRows; the ops
// come with #2137's second half.

// anthropicUsageAnswer is one page. Fields not listed (server_tool_use,
// workspace_id, service_tier ...) are ignored.
type anthropicUsageAnswer struct {
	Data []struct {
		StartingAt string `json:"starting_at"`
		Results    []struct {
			Model         *string `json:"model"`
			Uncached      int64   `json:"uncached_input_tokens"`
			Output        int64   `json:"output_tokens"`
			CacheRead     int64   `json:"cache_read_input_tokens"`
			CacheCreation struct {
				OneHour  int64 `json:"ephemeral_1h_input_tokens"`
				FiveMins int64 `json:"ephemeral_5m_input_tokens"`
			} `json:"cache_creation"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage *string `json:"next_page"`
}

// parseAnthropicUsage reads one page of the usage report. A bucket without
// results gives no rows; has_more without a next_page is an error (the
// next request could not be made).
func parseAnthropicUsage(body []byte) (usagePage[UsageRow], error) {
	var a anthropicUsageAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		return usagePage[UsageRow]{}, errors.New("anthropic usage: the answer is not the expected JSON")
	}
	p := usagePage[UsageRow]{HasMore: a.HasMore}
	if a.NextPage != nil {
		p.NextPage = *a.NextPage
	}
	if p.HasMore && p.NextPage == "" {
		return usagePage[UsageRow]{}, errors.New("anthropic usage: has_more without next_page")
	}
	for _, b := range a.Data {
		day, err := bucketDay(b.StartingAt)
		if err != nil {
			return usagePage[UsageRow]{}, fmt.Errorf("anthropic usage: %w", err)
		}
		for _, r := range b.Results {
			row := UsageRow{Day: day, InputTokens: r.Uncached, OutputTokens: r.Output,
				CacheRead: r.CacheRead, CacheWrite: r.CacheCreation.OneHour + r.CacheCreation.FiveMins}
			if r.Model != nil {
				row.Model = *r.Model
			}
			p.Rows = append(p.Rows, row)
		}
	}
	return p, nil
}
