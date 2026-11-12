package extapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOpenAI is the organization Usage and Costs API on httptest (no live
// calls): Bearer "ok" answers from testdata, "limited" is 429, anything
// else 401.
type fakeOpenAI struct {
	mu      sync.Mutex
	queries []url.Values
	headers []http.Header
	urls    []string
}

func newFakeOpenAI(t *testing.T) (*fakeOpenAI, map[string]Operation) {
	f := &fakeOpenAI{}
	serve := func(pages map[string]string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			f.queries = append(f.queries, r.URL.Query())
			f.headers = append(f.headers, r.Header.Clone())
			f.urls = append(f.urls, r.URL.String())
			f.mu.Unlock()
			switch r.Header.Get("Authorization") {
			case "Bearer ok":
			case "Bearer limited":
				w.WriteHeader(429)
				_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_exceeded"}}`))
				return
			default:
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error"}}`))
				return
			}
			file, ok := pages[r.URL.Query().Get("page")]
			if !ok {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(readTestdata(t, "openai_usage", file))
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/organization/usage/completions", serve(map[string]string{"": "page1.json", "page_AAAAAGdGxdEiJdKOAAAAAGcqsYA=": "page2.json"}))
	mux.HandleFunc("GET /v1/organization/costs", serve(map[string]string{"": "cost.json"}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	now := func() time.Time { return time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC) }
	ops, err := newOpenAIUsage(usageTestKeys, srv.URL, srv.Client(), now)
	if err != nil {
		t.Fatal(err)
	}
	return f, ops
}

func TestOpenAIUsageOp(t *testing.T) {
	f, ops := newFakeOpenAI(t)
	out, err := ops["usage"].Do(context.Background(), usageTestIn("ok", map[string]string{"days": "3"}))
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Title: "2026-11-10", Subtitle: "gpt-4o-mini-2024-07-18", Value: "154"},
		{Title: "2026-11-11", Value: "10"},
	}
	if !slices.Equal(out.Rows, want) || !slices.Equal(out.Cells[0], []string{"2026-11-10", "gpt-4o-mini-2024-07-18", "70", "44", "40", "0"}) {
		t.Fatalf("%+v %v", out.Rows, out.Cells)
	}
	if len(f.queries) != 2 {
		t.Fatalf("pages %d", len(f.queries))
	}
	start := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC).Unix()
	q := f.queries[0]
	if q.Get("start_time") != strconv.FormatInt(start, 10) || q.Get("bucket_width") != "1d" || q.Get("group_by") != "model" || q.Get("limit") != "3" || q.Has("page") {
		t.Fatalf("query %v", q)
	}
	if f.queries[1].Get("page") != "page_AAAAAGdGxdEiJdKOAAAAAGcqsYA=" {
		t.Fatalf("second page %v", f.queries[1])
	}
	for _, u := range f.urls {
		if strings.Contains(u, "ok") {
			t.Fatalf("key in the URL: %s", u)
		}
	}
}

func TestOpenAICostOp(t *testing.T) {
	f, ops := newFakeOpenAI(t)
	out, err := ops["cost"].Do(context.Background(), usageTestIn("ok", nil))
	if err != nil {
		t.Fatal(err)
	}
	if want := []Row{{Title: "2026-11-10", Subtitle: "USD", Value: "1.56"}}; !slices.Equal(out.Rows, want) {
		t.Fatalf("%+v", out.Rows)
	}
	start := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC).Unix()
	if q := f.queries[0]; q.Get("start_time") != strconv.FormatInt(start, 10) || q.Get("limit") != "7" || q.Has("group_by") {
		t.Fatalf("query %v", q)
	}
}

func TestOpenAIUsageErrors(t *testing.T) {
	f, ops := newFakeOpenAI(t)
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
		if _, err := ops[op].Do(ctx, usageTestIn("ok", map[string]string{"days": "0"})); !errors.Is(err, ErrBadArgs) {
			t.Fatalf("%s days: %v", op, err)
		}
	}
	if len(f.queries) != 4 {
		t.Fatalf("calls %d (no key and bad days must not reach the API)", len(f.queries))
	}
}

func TestParseOpenAICost(t *testing.T) {
	p, err := parseOpenAICost(readTestdata(t, "openai_usage", "cost.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []CostRow{{Day: "2026-11-10", Currency: "USD", Amount: 0.06}, {Day: "2026-11-10", Currency: "USD", Amount: 1.5}}
	if !slices.Equal(p.Rows, want) || p.HasMore {
		t.Fatalf("%+v", p)
	}
	for _, body := range []string{`{"data":[{"start_time":0,"results":[{"amount":{"currency":"usd"}}]}]}`,
		`{"data":[{"results":[]}]}`, `{"has_more":true}`, `[]`} {
		if _, err := parseOpenAICost([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

// Read ops only, api.openai.com only, never cached; not in the registry
// without EXT_OPENAI_USAGE_ENABLED=1; and until #1671 New refuses.
func TestOpenAIUsageDefinition(t *testing.T) {
	p := openaiUsageProvider
	if err := p.check(); err != nil || p.NeedsOAuth || p.Billing != BillingFree || p.Fake || p.EnvName() != "OPENAI_USAGE" || !slices.Equal(p.Hosts, []string{openaiHost}) {
		t.Fatalf("%+v %v", p, err)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 || o.CacheTTLFor != nil || !slices.Equal(o.Args, []string{"days"}) {
			t.Errorf("%s: %+v", o.Name, o)
		}
	}
	if _, err := p.CheckArgs("cost", map[string]string{"project": "x"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("other arg: %v", err)
	}
	if _, ok := Lookup(OpenAIUsage); ok != flag(os.Getenv("EXT_OPENAI_USAGE_ENABLED")) {
		t.Fatalf("registered=%v, but EXT_OPENAI_USAGE_ENABLED=%q", ok, os.Getenv("EXT_OPENAI_USAGE_ENABLED"))
	}
	if _, err := p.New(Secrets{APIKey: "operator"}); !errors.Is(err, errOpenAIUsageNoKeyStore) {
		t.Fatalf("built before #1671: %v", err)
	}
	u, _ := url.Parse(openaiHost + "/v1/organization/costs")
	if !AllowedURL(p.Hosts, u) {
		t.Fatal("api not on the allowlist")
	}
	if _, err := NewHTTPClient(p.Hosts, DefaultTimeout).Get("https://api.anthropic.com/"); !errors.Is(err, ErrEgress) {
		t.Fatalf("egress: %v", err)
	}
}

func TestParseOpenAIUsage(t *testing.T) {
	cases := []struct {
		file     string
		want     []UsageRow
		hasMore  bool
		nextPage string
	}{
		// input_tokens include the cached ones: the row keeps the rest
		{file: "normal.json", want: []UsageRow{
			{Day: "2026-11-10", Model: "gpt-4o-mini-2024-07-18", InputTokens: 600, OutputTokens: 300, CacheRead: 400},
			{Day: "2026-11-10", Model: "o3-mini", InputTokens: 50, OutputTokens: 20},
		}},
		{file: "empty.json"},
		{file: "page1.json", hasMore: true, nextPage: "page_AAAAAGdGxdEiJdKOAAAAAGcqsYA=", want: []UsageRow{
			{Day: "2026-11-10", Model: "gpt-4o-mini-2024-07-18", InputTokens: 70, OutputTokens: 40, CacheRead: 30},
		}},
	}
	for _, c := range cases {
		p, err := parseOpenAIUsage(readTestdata(t, "openai_usage", c.file))
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if !slices.Equal(p.Rows, c.want) || p.HasMore != c.hasMore || p.NextPage != c.nextPage {
			t.Errorf("%s: %+v", c.file, p)
		}
	}
}

// Two pages split one bucket: merged, the rows of the same day and model
// add up; a null model is "".
func TestParseOpenAIUsageTwoPages(t *testing.T) {
	var rows []UsageRow
	for _, f := range []string{"page1.json", "page2.json"} {
		p, err := parseOpenAIUsage(readTestdata(t, "openai_usage", f))
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, p.Rows...)
	}
	want := []UsageRow{
		{Day: "2026-11-10", Model: "gpt-4o-mini-2024-07-18", InputTokens: 70, OutputTokens: 44, CacheRead: 40},
		{Day: "2026-11-11", InputTokens: 7, OutputTokens: 3},
	}
	if got := mergeUsageRows(rows); !slices.Equal(got, want) {
		t.Fatalf("%+v", got)
	}
}

func TestParseOpenAIUsageBad(t *testing.T) {
	for _, body := range []string{
		`<html>`,
		`{"data": [{"results": []}]}`,
		`{"data": [{"start_time": "today", "results": []}]}`,
		`{"data": [], "has_more": true, "next_page": null}`,
	} {
		if _, err := parseOpenAIUsage([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	// more cached than input (never seen): no negative input
	p, err := parseOpenAIUsage([]byte(`{"data":[{"start_time":0,"results":[{"input_tokens":5,"input_cached_tokens":9}]}]}`))
	if err != nil || p.Rows[0] != (UsageRow{Day: "1970-01-01", CacheRead: 5}) {
		t.Fatalf("%+v %v", p, err)
	}
}
