package dsl

import (
	"fmt"
	"sort"
	"strings"
)

// aiButton inputFeeds: a generate button also reads the rows feeds on the
// same screen show (weather / pressure / warnings / holidays / rss), e.g.
// "今日の天気と気圧から体調のアドバイス". The iOS app appends the rows it
// already loaded (at most MaxAIFeedRows rows / MaxAIFeedBytes in all) to the input
// as an "外部データ" section. Feed rows are public data, not records, so
// nothing about other users goes out; /v1/text is unchanged.
const (
	MaxAIInputFeeds = 3
	MaxAIFeedRows   = 20
	MaxAIFeedBytes  = 2048
)

// checkAIInputFeeds needs the whole screen (the feed may sit anywhere on it,
// also inside a container), so it runs after the per-component walk.
func checkAIInputFeeds(a *App, add func(string, ...any)) {
	for _, s := range a.Screens {
		types := map[string]ComponentType{}
		var feeds []string
		for _, c := range s.Components {
			for _, x := range append([]Component{c}, c.Children...) {
				if x.ID == "" {
					continue
				}
				types[x.ID] = x.Type
				if x.Type == CompFeed {
					feeds = append(feeds, x.ID)
				}
			}
		}
		sort.Strings(feeds)
		for i, c := range s.Components {
			for j, x := range append([]Component{c}, c.Children...) {
				if x.Type != CompAiButton || len(x.InputFeeds) == 0 {
					continue
				}
				where := fmt.Sprintf("screen %q components[%d]", s.ID, i)
				if j > 0 {
					where += fmt.Sprintf(".children[%d]", j-1)
				}
				checkAIInputFeed(where, &x, types, feeds, add)
			}
		}
	}
}

func checkAIInputFeed(where string, c *Component, types map[string]ComponentType, feeds []string, add func(string, ...any)) {
	if c.Task != "generate" {
		add("%s: aiButton inputFeeds is only for task generate (it adds the feed rows to the material)", where)
	}
	if len(c.InputFeeds) > MaxAIInputFeeds {
		add("%s: aiButton inputFeeds takes at most %d feeds", where, MaxAIInputFeeds)
	}
	onScreen := "none; give the feed an id"
	if len(feeds) > 0 {
		onScreen = strings.Join(feeds, ", ")
	}
	seen := map[string]bool{}
	for _, id := range c.InputFeeds {
		if seen[id] {
			add("%s: aiButton inputFeeds lists %q twice", where, id)
			continue
		}
		seen[id] = true
		switch t, ok := types[id]; {
		case !ok:
			add("%s: aiButton inputFeeds %q is not the id of a feed on the same screen (feeds here: %s)", where, id, onScreen)
		case t != CompFeed:
			add("%s: aiButton inputFeeds %q is a %s, not a feed (feeds here: %s)", where, id, t, onScreen)
		}
	}
}
