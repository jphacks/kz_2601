package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// OpenAI's usage and cost (#2138): a developer reads their own
// organization's usage from OpenAI's Usage and Costs API. Free (one `runs`
// a call), read only. The rows are the same as Claude's (usage_rows.go,
// anthropic_usage.go), so one graph draws either.
//
//	usage(days?)  GET /v1/organization/usage/completions  tokens a day and model (UsageRow)
//	cost(days?)   GET /v1/organization/costs              cost a day (CostRow)
//
// days is 1-31 (default 7): today (UTC) and the days before it.
//
// Key: the person's own admin key (sk-admin-…), from the per-user key
// store (#1671). An admin key can also manage the organization, so this
// provider has read ops only, the key goes in the Authorization header
// only (never a URL, a log or an answer) and nothing is cached (the shared
// cache is keyed by the arguments, not by the person). Until #1671 lands
// New refuses, so the provider stays off even when registered.
//
// Registered only with EXT_OPENAI_USAGE_ENABLED=1 (without it the provider
// is not in the registry at all).
const (
	OpenAIUsage = "openaiUsage"

	openaiHost = "https://api.openai.com"
)

// errOpenAIUsageNoKeyStore keeps the provider off until #1671 can hand an
// op the person's key.
var errOpenAIUsageNoKeyStore = errors.New("openaiUsage: per-user keys (#1671) are not in yet")

var openaiUsageProvider = Provider{ID: OpenAIUsage, Billing: BillingFree,
	Ops: []OpSpec{
		{Name: "usage", Args: []string{"days"}, Check: checkUsageDays},
		{Name: "cost", Args: []string{"days"}, Check: checkUsageDays},
	},
	Hosts: []string{openaiHost},
	New: func(Secrets) (map[string]Operation, error) {
		return newOpenAIUsage(nil, openaiHost, nil, time.Now)
	},
}

func init() {
	if flag(os.Getenv("EXT_OPENAI_USAGE_ENABLED")) {
		register(openaiUsageProvider)
	}
}

// newOpenAIUsage builds the ops against base (the API's origin, or a test
// server).
func newOpenAIUsage(keys usageKeys, base string, hc *http.Client, now func() time.Time) (map[string]Operation, error) {
	if keys == nil {
		return nil, errOpenAIUsageNoKeyStore
	}
	if hc == nil {
		hc = NewHTTPClient([]string{openaiHost}, DefaultTimeout)
	}
	c := &openaiUsage{usageClient{keys: keys, base: base, hc: hc, now: now}}
	return map[string]Operation{"usage": OperationFunc(c.usage), "cost": OperationFunc(c.cost)}, nil
}

type openaiUsage struct{ usageClient }

func openaiAuth(r *http.Request, key string) { r.Header.Set("Authorization", "Bearer "+key) }

// query is a report's window: 1d buckets from the first day, one page of
// days buckets.
func (c *openaiUsage) query(days int, next string) url.Values {
	q := url.Values{"start_time": {strconv.FormatInt(c.start(days).Unix(), 10)}, "bucket_width": {"1d"}, "limit": {strconv.Itoa(days)}}
	if next != "" {
		q.Set("page", next)
	}
	return q
}

func (c *openaiUsage) usage(ctx context.Context, in Input) (Output, error) {
	days, err := usageDays(in.Args)
	if err != nil {
		return Output{}, err
	}
	rows, err := readPages(func(next string) (usagePage[UsageRow], error) {
		q := c.query(days, next)
		q.Set("group_by", "model")
		body, err := c.get(ctx, in, "/v1/organization/usage/completions", q, openaiAuth)
		if err != nil {
			return usagePage[UsageRow]{}, err
		}
		return openaiPage(parseOpenAIUsage(body))
	})
	if err != nil {
		return Output{}, err
	}
	return usageOutput(mergeUsageRows(rows)), nil
}

func (c *openaiUsage) cost(ctx context.Context, in Input) (Output, error) {
	days, err := usageDays(in.Args)
	if err != nil {
		return Output{}, err
	}
	rows, err := readPages(func(next string) (usagePage[CostRow], error) {
		body, err := c.get(ctx, in, "/v1/organization/costs", c.query(days, next), openaiAuth)
		if err != nil {
			return usagePage[CostRow]{}, err
		}
		return openaiPage(parseOpenAICost(body))
	})
	if err != nil {
		return Output{}, err
	}
	return costOutput(mergeCostRows(rows)), nil
}

// openaiPage turns a 200 the parser refused into a provider error (502
// upstream_unavailable, not retried).
func openaiPage[T any](p usagePage[T], err error) (usagePage[T], error) {
	if err != nil {
		return p, &ProviderError{Status: http.StatusOK, Err: err}
	}
	return p, nil
}

// openaiBucketDay is the UTC date of a bucket's start_time (unix seconds).
func openaiBucketDay(start *int64) (string, error) {
	if start == nil || *start < 0 {
		return "", errors.New("a bucket without start_time")
	}
	return time.Unix(*start, 0).UTC().Format(time.DateOnly), nil
}

// openaiNextPage reads has_more / next_page.
func openaiNextPage(hasMore bool, next *string) (string, error) {
	n := ""
	if next != nil {
		n = *next
	}
	if hasMore && n == "" {
		return "", errors.New("has_more without next_page")
	}
	return n, nil
}

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
	next, err := openaiNextPage(a.HasMore, a.NextPage)
	if err != nil {
		return usagePage[UsageRow]{}, errors.New("openai usage: " + err.Error())
	}
	p := usagePage[UsageRow]{HasMore: a.HasMore, NextPage: next}
	for _, b := range a.Data {
		day, err := openaiBucketDay(b.StartTime)
		if err != nil {
			return usagePage[UsageRow]{}, errors.New("openai usage: " + err.Error())
		}
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

// openaiCostAnswer is one page of the costs. amount.value is in the
// currency's main unit (dollars).
type openaiCostAnswer struct {
	Data []struct {
		StartTime *int64 `json:"start_time"`
		Results   []struct {
			Amount struct {
				Value    *float64 `json:"value"`
				Currency string   `json:"currency"`
			} `json:"amount"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage *string `json:"next_page"`
}

// parseOpenAICost reads one page of the costs into CostRows.
func parseOpenAICost(body []byte) (usagePage[CostRow], error) {
	var a openaiCostAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		return usagePage[CostRow]{}, errors.New("openai cost: the answer is not the expected JSON")
	}
	next, err := openaiNextPage(a.HasMore, a.NextPage)
	if err != nil {
		return usagePage[CostRow]{}, errors.New("openai cost: " + err.Error())
	}
	p := usagePage[CostRow]{HasMore: a.HasMore, NextPage: next}
	for _, b := range a.Data {
		day, err := openaiBucketDay(b.StartTime)
		if err != nil {
			return usagePage[CostRow]{}, errors.New("openai cost: " + err.Error())
		}
		for _, r := range b.Results {
			v := r.Amount.Value
			if v == nil || math.IsInf(*v, 0) || math.IsNaN(*v) {
				return usagePage[CostRow]{}, errors.New("openai cost: bad amount")
			}
			cur := strings.ToUpper(strings.TrimSpace(r.Amount.Currency))
			if cur == "" {
				cur = "USD"
			}
			p.Rows = append(p.Rows, CostRow{Day: day, Currency: cur, Amount: *v})
		}
	}
	return p, nil
}
