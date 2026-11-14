package main

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// aiInputFeeds (bool): some generate aiButton reads the rows of a feed on its
// screen (inputFeeds; dsl/ai_input_feed.go). Cases:
// internal/ai/eval/ai_input_feed_cases.json.
func checkAIInputFeeds(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["aiInputFeeds"].(bool)
	if !ok {
		return nil
	}
	got := false
	for _, b := range generateButtons(a) {
		got = got || len(b.InputFeeds) > 0
	}
	switch {
	case got && !want:
		return []string{"aiInputFeeds=false but a generate button has inputFeeds"}
	case !got && want:
		return []string{"aiInputFeeds=true but no generate button has inputFeeds"}
	}
	return nil
}
