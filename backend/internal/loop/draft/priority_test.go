package draft

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

func TestBundlePriority(t *testing.T) {
	item := func(impact, priority string) loop.Item { return loop.Item{Impact: impact, Priority: priority} }
	for _, c := range []struct {
		name, category, area string
		items                []loop.Item
		count                int
		want                 string
	}{
		// the representative's P3 no longer drops a medium impact to low
		{"medium feature x2", "feature", "generate", []loop.Item{item("medium", "P3"), item("medium", "P3")}, 2, "P2"},
		{"low feature x2", "feature", "render", []loop.Item{item("low", "P3"), item("low", "P3")}, 2, "P3"},
		// the highest impact of any item counts
		{"one high item", "question", "other", []loop.Item{item("low", "P3"), item("high", "P3")}, 2, "P2"},
		{"high bug x3", "bug", "render", []loop.Item{item("high", "P1")}, 3, "P1"},
		// many reports raise one level, capped at P1
		{"question x9", "question", "other", []loop.Item{item("low", "P3")}, 9, "P3"},
		{"question x10", "question", "other", []loop.Item{item("low", "P3")}, 10, "P2"},
		{"bug x10 stays P1", "bug", "render", []loop.Item{item("high", "P1")}, 10, "P1"},
		// spam and praise are never raised
		{"spam x20", "spam", "other", []loop.Item{item("high", "P3")}, 20, "P3"},
		{"praise x20", "praise", "other", []loop.Item{item("high", "P3")}, 20, "P3"},
		// items triaged before the impact was stored fall back to their priority
		{"legacy P2 x2", "feature", "render", []loop.Item{item("", "P2"), item("", "P2")}, 2, "P2"},
	} {
		if got := BundlePriority(c.category, c.area, c.items, c.count); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
