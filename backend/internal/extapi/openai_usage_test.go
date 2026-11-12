package extapi

import (
	"slices"
	"testing"
)

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
