package extapi

import (
	"slices"
	"testing"
)

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
