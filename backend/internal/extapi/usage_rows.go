package extapi

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// The shared shape of an AI provider's own usage and cost reports
// (anthropic_usage.go #2137, openai_usage.go #2138): both providers turn
// their answers into these rows, so one graph draws either.

// UsageRow is one UTC day × one model of token usage.
//
// InputTokens is the input not read from the prompt cache (Anthropic's
// uncached_input_tokens; OpenAI's input_tokens minus input_cached_tokens),
// so InputTokens + CacheRead + CacheWrite is every input token once.
type UsageRow struct {
	Day          string // YYYY-MM-DD (UTC: the providers' day buckets)
	Model        string // "" when the provider did not say
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheWrite   int64
}

// CostRow is one UTC day of cost in the provider's currency.
type CostRow struct {
	Day      string // YYYY-MM-DD (UTC)
	Currency string // upper case (USD)
	Amount   float64
}

// usagePage is one page of a paged report.
type usagePage[T any] struct {
	Rows     []T
	HasMore  bool
	NextPage string
}

// bucketDay is the UTC date of a bucket's start (RFC 3339).
func bucketDay(s string) (string, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return "", fmt.Errorf("bad bucket start %q", s)
	}
	return t.UTC().Format(time.DateOnly), nil
}

// mergeUsageRows sums the rows of the same day and model (two pages may
// split one bucket) and sorts them by day, then model.
func mergeUsageRows(rows []UsageRow) []UsageRow {
	type key struct{ day, model string }
	at := map[key]int{}
	out := make([]UsageRow, 0, len(rows))
	for _, r := range rows {
		k := key{r.Day, r.Model}
		if i, ok := at[k]; ok {
			s := &out[i]
			s.InputTokens += r.InputTokens
			s.OutputTokens += r.OutputTokens
			s.CacheRead += r.CacheRead
			s.CacheWrite += r.CacheWrite
			continue
		}
		at[k] = len(out)
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b UsageRow) int {
		return cmp.Or(cmp.Compare(a.Day, b.Day), cmp.Compare(a.Model, b.Model))
	})
	return out
}

// mergeCostRows sums the rows of the same day and currency, sorted by day.
func mergeCostRows(rows []CostRow) []CostRow {
	type key struct{ day, cur string }
	at := map[key]int{}
	out := make([]CostRow, 0, len(rows))
	for _, r := range rows {
		k := key{r.Day, r.Currency}
		if i, ok := at[k]; ok {
			out[i].Amount += r.Amount
			continue
		}
		at[k] = len(out)
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b CostRow) int {
		return cmp.Or(cmp.Compare(a.Day, b.Day), cmp.Compare(a.Currency, b.Currency))
	})
	return out
}

// --- what the usage / cost ops share ---

const (
	// usageMaxDays is the longest window (the providers' 1d buckets: one
	// page of 31).
	usageMaxDays = 31
	// usageMaxPages bounds the pages one call reads.
	usageMaxPages = 5
)

// usageKey is a person's admin key of an AI provider (the per-user key
// store, #1671). It reads the whole organization's usage: never log it.
type usageKey struct{ Key string }

// String keeps the key out of %v.
func (usageKey) String() string { return "[redacted]" }

// GoString keeps the key out of %#v.
func (usageKey) GoString() string { return "[redacted]" }

// usageKeys finds the key of the person a call acts for. ErrNotConnected
// when the person has none.
type usageKeys func(ctx context.Context, in Input) (usageKey, error)

// usageClient is what both providers' ops hold.
type usageClient struct {
	keys usageKeys
	base string // the API's origin, or a test server's
	hc   *http.Client
	now  func() time.Time
}

// start is where a window of days days begins.
func (c *usageClient) start(days int) time.Time { return usageStart(c.now(), days) }

// get GETs base+path?q with the person's key (auth sets the header) and
// returns the answer. No key is ErrNotConnected before any request; 401 /
// 403 is a key the provider no longer takes (ErrNotConnected: the person
// saves it again); 429 / 5xx are retryable (sendJSON). The key never goes
// in the URL.
func (c *usageClient) get(ctx context.Context, in Input, path string, q url.Values, auth func(r *http.Request, key string)) ([]byte, error) {
	k, err := c.keys(ctx, in)
	if err != nil {
		return nil, err
	}
	if k.Key == "" {
		return nil, ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	auth(req, k.Key)
	req.Header.Set("Accept", "application/json")
	var body json.RawMessage
	err = sendJSON(ctx, c.hc, req, &body)
	var pe *ProviderError
	if errors.As(err, &pe) && (pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden) {
		return nil, ErrNotConnected
	}
	return body, err
}

// readPages reads page("") and then each next page while has_more, at
// most usageMaxPages; more than that is a provider error (a partial graph
// would read as a quiet day).
func readPages[T any](page func(next string) (usagePage[T], error)) ([]T, error) {
	var rows []T
	next := ""
	for range usageMaxPages {
		p, err := page(next)
		if err != nil {
			return nil, err
		}
		rows = append(rows, p.Rows...)
		if !p.HasMore {
			return rows, nil
		}
		next = p.NextPage
	}
	return nil, &ProviderError{Status: http.StatusOK, Err: errors.New("more pages than one call reads")}
}

// usageDays reads days (1-31, default 7).
func usageDays(a map[string]string) (int, error) { return argInt(a, "days", 7, 1, usageMaxDays) }

func checkUsageDays(a map[string]string) error {
	_, err := usageDays(a)
	return err
}

// usageStart is 00:00 UTC of the first of the last days days (today included).
func usageStart(now time.Time, days int) time.Time {
	y, m, d := now.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1-days)
}

// usageOutput: one row a day and model (title = the day, subtitle = the
// model, value = every token of it). Cells: day, model, input, output,
// cache read, cache write.
func usageOutput(rows []UsageRow) Output {
	out := Output{Rows: make([]Row, 0, len(rows)), Cells: make([][]string, 0, len(rows))}
	for _, r := range rows {
		total := r.InputTokens + r.OutputTokens + r.CacheRead + r.CacheWrite
		out.Rows = append(out.Rows, Row{Title: r.Day, Subtitle: r.Model, Value: strconv.FormatInt(total, 10)})
		out.Cells = append(out.Cells, []string{r.Day, r.Model, strconv.FormatInt(r.InputTokens, 10),
			strconv.FormatInt(r.OutputTokens, 10), strconv.FormatInt(r.CacheRead, 10), strconv.FormatInt(r.CacheWrite, 10)})
	}
	return out
}

// costOutput: one row a day (title = the day, subtitle = the currency,
// value = the amount, 2 decimals). Cells: day, currency, amount.
func costOutput(rows []CostRow) Output {
	out := Output{Rows: make([]Row, 0, len(rows)), Cells: make([][]string, 0, len(rows))}
	for _, r := range rows {
		v := strconv.FormatFloat(r.Amount, 'f', 2, 64)
		out.Rows = append(out.Rows, Row{Title: r.Day, Subtitle: r.Currency, Value: v})
		out.Cells = append(out.Cells, []string{r.Day, r.Currency, v})
	}
	return out
}
