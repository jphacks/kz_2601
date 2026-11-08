package dsl

import (
	"strings"
	"testing"
)

func feedInputApp(comps ...Component) App {
	app := aiApp(comps...)
	app.Screens = append(app.Screens, Screen{ID: "other", Title: "別", Components: []Component{
		{Type: CompFeed, ID: "far", Provider: FeedWeather, Region: "170000"},
	}})
	return app
}

func TestAIInputFeedValidPasses(t *testing.T) {
	app := feedInputApp(
		Component{Type: CompFeed, ID: "wx", Provider: FeedWeather, Region: "170000"},
		Component{Type: CompVStack, Children: []Component{
			{Type: CompFeed, ID: "hpa", Provider: FeedPressure, Region: "170000"},
			{Type: CompAiButton, ID: "b", Task: "generate", Prompt: "気圧から体調の注意", InputFeeds: []string{"hpa"}},
		}},
		Component{Type: CompAiButton, Task: "generate", Prompt: "天気と気圧から体調", Shape: "list", InputFeeds: []string{"wx", "hpa"}},
		Component{Type: CompAiButton, ID: "c", Task: "generate", Prompt: "{{input}} から一言", Entity: "diary", InputFeeds: []string{"wx"}, Length: "long"},
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAIInputFeedSemanticErrors(t *testing.T) {
	app := feedInputApp(
		Component{Type: CompFeed, ID: "wx", Provider: FeedWeather, Region: "170000"},
		Component{Type: CompText, ID: "note", Text: "メモ"},
		Component{Type: CompAiButton, ID: "a", Task: "generate", Prompt: "x", InputFeeds: []string{"nope"}},
		Component{Type: CompAiButton, ID: "b", Task: "generate", Prompt: "x", InputFeeds: []string{"note"}},
		Component{Type: CompAiButton, ID: "c", Task: "generate", Prompt: "x", InputFeeds: []string{"far"}},
		Component{Type: CompAiButton, ID: "d", Task: "summarize", Input: "body", Output: "summary", InputFeeds: []string{"wx"}},
		Component{Type: CompAiButton, ID: "e", Task: "generate", Prompt: "x", InputFeeds: []string{"wx", "wx"}},
		Component{Type: CompAiButton, ID: "f", Task: "generate", Prompt: "x", InputFeeds: []string{"wx", "a", "b", "c"}},
	)
	err := app.Validate()
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, w := range []string{
		`inputFeeds "nope" is not the id of a feed on the same screen (feeds here: wx)`,
		`inputFeeds "note" is a text, not a feed`,
		`inputFeeds "far" is not the id of a feed on the same screen`,
		"inputFeeds is only for task generate",
		`inputFeeds lists "wx" twice`,
		"inputFeeds takes at most 3 feeds",
	} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in\n%s", w, joined)
		}
	}
	if n := strings.Count(joined, "inputFeeds"); n != 9 {
		t.Errorf("want 9 inputFeeds problems, got %d:\n%s", n, joined)
	}
}

func TestAIInputFeedNoFeedOnScreen(t *testing.T) {
	app := aiApp(Component{Type: CompAiButton, Task: "generate", Prompt: "x", InputFeeds: []string{"wx"}})
	err := app.Validate()
	if err == nil || !strings.Contains(err.Error(), "feeds here: none; give the feed an id") {
		t.Fatalf("got %v", err)
	}
}

func TestAIInputFeedSchema(t *testing.T) {
	doc := func(v string) []byte {
		return []byte(`{"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home","entities":[],
"screens":[{"id":"home","title":"h","components":[
 {"type":"feed","id":"wx","provider":"weather","region":"130000","area":"A1:F6"},
 {"type":"aiButton","task":"generate","prompt":"体調のアドバイス","inputFeeds":` + v + `,"area":"A7:F12"}]}]}`)
	}
	a, err := Parse(doc(`["wx"]`))
	if err != nil {
		t.Fatalf("valid inputFeed: %v", err)
	}
	if got := a.Screens[0].Components[1].InputFeeds; len(got) != 1 || got[0] != "wx" {
		t.Fatalf("decoded %v", got)
	}
	for _, bad := range []string{`"wx"`, `[]`, `["wx","wx"]`, `["a","b","c","d"]`} {
		if _, err := Parse(doc(bad)); err == nil {
			t.Errorf("inputFeeds %s should fail the schema", bad)
		}
	}
}
