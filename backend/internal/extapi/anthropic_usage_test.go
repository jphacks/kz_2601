package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAnthropic is the Admin API's usage and cost reports on httptest (no
// live calls): x-api-key "ok" answers from testdata, "limited" is 429,
// anything else 401.
type fakeAnthropic struct {
	mu      sync.Mutex
	queries []url.Values
	headers []http.Header
	urls    []string
}

func newFakeAnthropic(t *testing.T) (*fakeAnthropic, map[string]Operation) {
	f := &fakeAnthropic{}
	serve := func(pages map[string]string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			f.queries = append(f.queries, r.URL.Query())
			f.headers = append(f.headers, r.Header.Clone())
			f.urls = append(f.urls, r.URL.String())
			f.mu.Unlock()
			switch r.Header.Get("x-api-key") {
			case "ok":
			case "limited":
				w.WriteHeader(429)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error"}}`))
				return
			default:
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error"}}`))
				return
			}
			file, ok := pages[r.URL.Query().Get("page")]
			if !ok {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(readTestdata(t, "anthropic_usage", file))
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/organizations/usage_report/messages", serve(map[string]string{"": "page1.json", "page_MjAyNi0xMS0xMQ": "page2.json"}))
	mux.HandleFunc("GET /v1/organizations/cost_report", serve(map[string]string{"": "cost.json"}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	now := func() time.Time { return time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC) }
	ops, err := newAnthropicUsage(usageTestKeys, srv.URL, srv.Client(), now)
	if err != nil {
		t.Fatal(err)
	}
	return f, ops
}

func TestAnthropicUsageOp(t *testing.T) {
	f, ops := newFakeAnthropic(t)
	out, err := ops["usage"].Do(context.Background(), usageTestIn("ok", map[string]string{"days": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Title: "2026-11-10", Subtitle: "claude-sonnet-4-5", Value: "210"},
		{Title: "2026-11-11", Subtitle: "claude-opus-4-1", Value: "17"},
	}
	if !slices.Equal(out.Rows, want) || !slices.Equal(out.Cells[0], []string{"2026-11-10", "claude-sonnet-4-5", "101", "54", "23", "32"}) {
		t.Fatalf("%+v %v", out.Rows, out.Cells)
	}
	if len(f.queries) != 2 {
		t.Fatalf("pages %d", len(f.queries))
	}
	q := f.queries[0]
	if q.Get("starting_at") != "2026-11-10T00:00:00Z" || q.Get("bucket_width") != "1d" || q.Get("group_by[]") != "model" || q.Get("limit") != "3" || q.Has("page") {
		t.Fatalf("query %v", q)
	}
	if f.queries[1].Get("page") != "page_MjAyNi0xMS0xMQ" {
		t.Fatalf("second page %v", f.queries[1])
	}
	h := f.headers[0]
	if h.Get("x-api-key") != "ok" || h.Get("anthropic-version") != anthropicVersion || h.Get("Authorization") != "" {
		t.Fatalf("headers %v", h)
	}
	for _, u := range f.urls {
		if strings.Contains(u, "ok") {
			t.Fatalf("key in the URL: %s", u)
		}
	}
}

func TestAnthropicCostOp(t *testing.T) {
	f, ops := newFakeAnthropic(t)
	out, err := ops["cost"].Do(context.Background(), usageTestIn("ok", nil))
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{{Title: "2026-11-10", Subtitle: "USD", Value: "2.00"}, {Title: "2026-11-11", Subtitle: "USD", Value: "0.05"}}
	if !slices.Equal(out.Rows, want) {
		t.Fatalf("%+v", out.Rows)
	}
	if q := f.queries[0]; q.Get("starting_at") != "2026-11-06T00:00:00Z" || q.Get("limit") != "7" || q.Has("group_by[]") {
		t.Fatalf("query %v", q)
	}
}

func TestAnthropicUsageErrors(t *testing.T) {
	f, ops := newFakeAnthropic(t)
	ctx := context.Background()
	for _, op := range []string{"usage", "cost"} {
		if _, err := ops[op].Do(ctx, Input{}); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("%s no key: %v", op, err)
		}
		if _, err := ops[op].Do(ctx, usageTestIn("dead", nil)); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("%s 401: %v", op, err)
		}
		_, err := ops[op].Do(ctx, usageTestIn("limited", nil))
		var pe *ProviderError
		if !errors.As(err, &pe) || !pe.Retryable || Reason(err) != ReasonUpstream {
			t.Fatalf("%s 429: %v", op, err)
		}
		if _, err := ops[op].Do(ctx, usageTestIn("ok", map[string]string{"days": "32"})); !errors.Is(err, ErrBadArgs) {
			t.Fatalf("%s days: %v", op, err)
		}
	}
	if len(f.queries) != 4 {
		t.Fatalf("calls %d (no key and bad days must not reach the API)", len(f.queries))
	}
}

func TestParseAnthropicCost(t *testing.T) {
	p, err := parseAnthropicCost(readTestdata(t, "anthropic_usage", "cost.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []CostRow{{Day: "2026-11-10", Currency: "USD", Amount: 1.235}, {Day: "2026-11-10", Currency: "USD", Amount: 0.765}, {Day: "2026-11-11", Currency: "USD", Amount: 0.05}}
	if !slices.Equal(p.Rows, want) || p.HasMore {
		t.Fatalf("%+v", p)
	}
	for _, body := range []string{`{"data":[{"starting_at":"2026-11-10T00:00:00Z","results":[{"amount":"NaN","currency":"USD"}]}]}`,
		`{"data":[{"starting_at":"2026-11-10T00:00:00Z","results":[{"amount":"x"}]}]}`, `{"has_more":true}`, `[]`} {
		if _, err := parseAnthropicCost([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

// Read ops only, api.anthropic.com only, never cached; not in the registry
// without EXT_ANTHROPIC_USAGE_ENABLED=1; and until #1671 New refuses.
func TestAnthropicUsageDefinition(t *testing.T) {
	p := anthropicUsageProvider
	if err := p.check(); err != nil || p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "ANTHROPIC_USAGE" || !slices.Equal(p.Hosts, []string{anthropicHost}) {
		t.Fatalf("%+v %v", p, err)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 || o.CacheTTLFor != nil || !slices.Equal(o.Args, []string{"days"}) {
			t.Errorf("%s: %+v", o.Name, o)
		}
	}
	if _, err := p.CheckArgs("usage", map[string]string{"days": "0"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("days 0: %v", err)
	}
	if _, err := p.CheckArgs("cost", map[string]string{"workspace": "x"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("other arg: %v", err)
	}
	if _, ok := Lookup(AnthropicUsage); ok != flag(os.Getenv("EXT_ANTHROPIC_USAGE_ENABLED")) {
		t.Fatalf("registered=%v, but EXT_ANTHROPIC_USAGE_ENABLED=%q", ok, os.Getenv("EXT_ANTHROPIC_USAGE_ENABLED"))
	}
	if _, err := p.New(Secrets{APIKey: "operator"}); !errors.Is(err, errAnthropicUsageNoKeyStore) {
		t.Fatalf("built before #1671: %v", err)
	}
	u, _ := url.Parse(anthropicHost + "/v1/organizations/cost_report")
	if !AllowedURL(p.Hosts, u) {
		t.Fatal("api not on the allowlist")
	}
	if _, err := NewHTTPClient(p.Hosts, DefaultTimeout).Get("https://api.openai.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
}

func TestParseAnthropicUsage(t *testing.T) {
	cases := []struct {
		file     string
		want     []UsageRow
		hasMore  bool
		nextPage string
	}{
		{file: "normal.json", want: []UsageRow{
			{Day: "2026-11-10", Model: "claude-sonnet-4-5", InputTokens: 1500, OutputTokens: 700, CacheRead: 200, CacheWrite: 1500},
			{Day: "2026-11-10", Model: "claude-haiku-4-5", InputTokens: 40, OutputTokens: 10},
		}},
		{file: "empty.json"},
		{file: "page1.json", hasMore: true, nextPage: "page_MjAyNi0xMS0xMQ", want: []UsageRow{
			{Day: "2026-11-10", Model: "claude-sonnet-4-5", InputTokens: 100, OutputTokens: 50, CacheRead: 20, CacheWrite: 30},
		}},
		// unknown fields are ignored; the day is the bucket's UTC date; a
		// null model is ""
		{file: "unknown_fields.json", want: []UsageRow{
			{Day: "2026-11-09", InputTokens: 5, OutputTokens: 6, CacheWrite: 2},
		}},
	}
	for _, c := range cases {
		p, err := parseAnthropicUsage(readTestdata(t, "anthropic_usage", c.file))
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if !slices.Equal(p.Rows, c.want) || p.HasMore != c.hasMore || p.NextPage != c.nextPage {
			t.Errorf("%s: %+v", c.file, p)
		}
	}
}

// Two pages split one bucket: merged, the rows of the same day and model add up.
func TestParseAnthropicUsageTwoPages(t *testing.T) {
	var rows []UsageRow
	for _, f := range []string{"page1.json", "page2.json"} {
		p, err := parseAnthropicUsage(readTestdata(t, "anthropic_usage", f))
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, p.Rows...)
	}
	want := []UsageRow{
		{Day: "2026-11-10", Model: "claude-sonnet-4-5", InputTokens: 101, OutputTokens: 54, CacheRead: 23, CacheWrite: 32},
		{Day: "2026-11-11", Model: "claude-opus-4-1", InputTokens: 9, OutputTokens: 8},
	}
	if got := mergeUsageRows(rows); !slices.Equal(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestParseAnthropicUsageBad(t *testing.T) {
	for _, body := range []string{
		`<html>`,
		`{"data": [{"starting_at": "yesterday", "results": []}]}`,
		`{"data": [], "has_more": true, "next_page": null}`,
		`{"data": "x"}`,
	} {
		if _, err := parseAnthropicUsage([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}
