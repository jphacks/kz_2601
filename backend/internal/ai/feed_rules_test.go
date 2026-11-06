package ai

import (
	"strings"
	"testing"
)

// #348: the feed rules reach the generation prompt and the schema-built
// catalog / chat index describe feed.
func TestFeedRulesReachPrompts(t *testing.T) {
	for _, want := range []string{"feed{provider: weather, region:", "feed{provider: holidays", "feedId: digital-news"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("systemPrompt lacks %q", want)
		}
	}
	if !strings.Contains(catalog, "\n- feed: provider") {
		t.Error("catalog lacks the feed line with its required keys")
	}
	if !strings.Contains(chatCapabilityIndex, "feed") {
		t.Error("chat capability index lacks feed")
	}
}

// Every feed rule line routes with "」は feed", so a build without feed
// (review build, older iOS) loses all of them and is told feed is unusable.
func TestFeedRulesDroppedWithoutFeed(t *testing.T) {
	for _, ln := range strings.Split(feedRules, "\n") {
		if !strings.Contains(ln, "」は feed") {
			t.Errorf("rule line is not dropped by the restriction: %q", ln)
		}
	}
	var allowed []string
	for typ := range knownComponentTypes {
		if typ != "feed" {
			allowed = append(allowed, typ)
		}
	}
	r := NewRestriction(allowed, nil)
	got := buildRestrictedPrompt(systemPrompt, r)
	for _, gone := range []string{"provider: weather", "feedId: digital-news", "\n- feed:"} {
		if strings.Contains(got, gone) {
			t.Errorf("restricted prompt still has %q", gone)
		}
	}
	if !strings.Contains(got, "使えない部品: feed") {
		t.Error("restricted prompt does not name feed as unusable")
	}
}
