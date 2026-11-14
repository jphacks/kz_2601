package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Claude's usage and cost (#2137): a developer reads their own
// organization's reports from Anthropic's Admin API. Free (one `runs` a
// call), read only.
//
//	usage(days?)  GET /v1/organizations/usage_report/messages  tokens a day and model (UsageRow)
//	cost(days?)   GET /v1/organizations/cost_report            cost a day (CostRow)
//
// days is 1-31 (default 7): today (UTC) and the days before it.
//
// Key: the person's own Admin API key (sk-ant-admin…), from the per-user
// key store (#1671). An admin key can also manage the organization, so
// this provider has read ops only, the key goes in the x-api-key header
// only (never a URL, a log or an answer) and nothing is cached (the shared
// cache is keyed by the arguments, not by the person). Until #1671 lands
// New refuses, so the provider stays off even when registered.
//
// Registered only with EXT_ANTHROPIC_USAGE_ENABLED=1 (without it the
// provider is not in the registry at all).
const (
	AnthropicUsage = "anthropicUsage"

	anthropicHost    = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
)

// errAnthropicUsageNoKeyStore keeps the provider off until #1671 can hand
// an op the person's key.
var errAnthropicUsageNoKeyStore = errors.New("anthropicUsage: per-user keys (#1671) are not in yet")

var anthropicUsageProvider = Provider{ID: AnthropicUsage, Billing: BillingFree,
	Ops: []OpSpec{
		{Name: "usage", Args: []string{"days"}, Check: checkUsageDays},
		{Name: "cost", Args: []string{"days"}, Check: checkUsageDays},
	},
	Hosts: []string{anthropicHost},
	New: func(Secrets) (map[string]Operation, error) {
		return newAnthropicUsage(nil, anthropicHost, nil, time.Now)
	},
}

func init() {
	if flag(os.Getenv("EXT_ANTHROPIC_USAGE_ENABLED")) {
		register(anthropicUsageProvider)
	}
}

// newAnthropicUsage builds the ops against base (the API's origin, or a
// test server).
func newAnthropicUsage(keys usageKeys, base string, hc *http.Client, now func() time.Time) (map[string]Operation, error) {
	if keys == nil {
		return nil, errAnthropicUsageNoKeyStore
	}
	if hc == nil {
		hc = NewHTTPClient([]string{anthropicHost}, DefaultTimeout)
	}
	c := &anthropicUsage{usageClient{keys: keys, base: base, hc: hc, now: now}}
	return map[string]Operation{"usage": OperationFunc(c.usage), "cost": OperationFunc(c.cost)}, nil
}

type anthropicUsage struct{ usageClient }

func anthropicAuth(r *http.Request, key string) {
	r.Header.Set("x-api-key", key)
	r.Header.Set("anthropic-version", anthropicVersion)
}

// query is a report's window: 1d buckets from the first day, one page of
// days buckets.
func (c *anthropicUsage) query(days int, next string) url.Values {
	q := url.Values{"starting_at": {c.start(days).Format(time.RFC3339)}, "bucket_width": {"1d"}, "limit": {strconv.Itoa(days)}}
	if next != "" {
		q.Set("page", next)
	}
	return q
}

func (c *anthropicUsage) usage(ctx context.Context, in Input) (Output, error) {
	days, err := usageDays(in.Args)
	if err != nil {
		return Output{}, err
	}
	rows, err := readPages(func(next string) (usagePage[UsageRow], error) {
		q := c.query(days, next)
		q.Set("group_by[]", "model")
		body, err := c.get(ctx, in, "/v1/organizations/usage_report/messages", q, anthropicAuth)
		if err != nil {
			return usagePage[UsageRow]{}, err
		}
		return anthropicPage(parseAnthropicUsage(body))
	})
	if err != nil {
		return Output{}, err
	}
	return usageOutput(mergeUsageRows(rows)), nil
}

func (c *anthropicUsage) cost(ctx context.Context, in Input) (Output, error) {
	days, err := usageDays(in.Args)
	if err != nil {
		return Output{}, err
	}
	rows, err := readPages(func(next string) (usagePage[CostRow], error) {
		body, err := c.get(ctx, in, "/v1/organizations/cost_report", c.query(days, next), anthropicAuth)
		if err != nil {
			return usagePage[CostRow]{}, err
		}
		return anthropicPage(parseAnthropicCost(body))
	})
	if err != nil {
		return Output{}, err
	}
	return costOutput(mergeCostRows(rows)), nil
}

// anthropicPage turns a 200 the parser refused into a provider error
// (502 upstream_unavailable, not retried).
func anthropicPage[T any](p usagePage[T], err error) (usagePage[T], error) {
	if err != nil {
		return p, &ProviderError{Status: http.StatusOK, Err: err}
	}
	return p, nil
}

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

// anthropicCostAnswer is one page of the cost report. amount is a decimal
// string in the currency's lowest unit (cents for USD).
type anthropicCostAnswer struct {
	Data []struct {
		StartingAt string `json:"starting_at"`
		Results    []struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage *string `json:"next_page"`
}

// parseAnthropicCost reads one page of the cost report into CostRows in
// the currency's main unit (USD, not cents).
func parseAnthropicCost(body []byte) (usagePage[CostRow], error) {
	var a anthropicCostAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		return usagePage[CostRow]{}, errors.New("anthropic cost: the answer is not the expected JSON")
	}
	p := usagePage[CostRow]{HasMore: a.HasMore}
	if a.NextPage != nil {
		p.NextPage = *a.NextPage
	}
	if p.HasMore && p.NextPage == "" {
		return usagePage[CostRow]{}, errors.New("anthropic cost: has_more without next_page")
	}
	for _, b := range a.Data {
		day, err := bucketDay(b.StartingAt)
		if err != nil {
			return usagePage[CostRow]{}, fmt.Errorf("anthropic cost: %w", err)
		}
		for _, r := range b.Results {
			v, err := strconv.ParseFloat(strings.TrimSpace(r.Amount), 64)
			if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
				return usagePage[CostRow]{}, errors.New("anthropic cost: bad amount")
			}
			cur := strings.ToUpper(strings.TrimSpace(r.Currency))
			if cur == "" {
				cur = "USD"
			}
			p.Rows = append(p.Rows, CostRow{Day: day, Currency: cur, Amount: v / 100})
		}
	}
	return p, nil
}
