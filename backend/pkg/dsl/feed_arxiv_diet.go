package dsl

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// arxiv / diet feeds (docs/backend/features/external-feeds.md). arxiv reads
// one allowlisted category (Component.FeedID); diet searches the National
// Diet Library's 国会会議録 for the app's fixed words (Component.Query, sent
// to the API as q). Neither takes a URL.

// ArxivCategories maps an allowlisted arXiv category to its name. The server
// builds the query from the key; adding one is a reviewed change.
var ArxivCategories = map[string]string{
	"cs.AI":   "人工知能",
	"cs.CL":   "自然言語処理",
	"cs.CR":   "暗号・セキュリティ",
	"cs.CV":   "コンピュータビジョン",
	"cs.HC":   "ヒューマンコンピュータインタラクション",
	"cs.LG":   "機械学習",
	"cs.RO":   "ロボティクス",
	"cs.SE":   "ソフトウェア工学",
	"stat.ML": "統計的機械学習",
}

// FeedIDTable is the allowlist feedId is checked against, nil when the
// provider takes no feedId.
func FeedIDTable(provider string) map[string]string {
	switch provider {
	case FeedRSS:
		return RSSFeeds
	case FeedArxiv:
		return ArxivCategories
	case FeedStatus:
		return StatusPages
	}
	return nil
}

// FeedQueryMaxRunes caps the diet search words.
const FeedQueryMaxRunes = 40

// NormalizeFeedQuery folds runs of spaces (incl. 全角) into one and trims.
// ok is false when the result is empty, too long, or holds a control /
// invisible character.
func NormalizeFeedQuery(q string) (string, bool) {
	for _, r := range q {
		if r == utf8.RuneError || (unicode.IsControl(r) && !unicode.IsSpace(r)) || unicode.Is(unicode.Cf, r) {
			return "", false
		}
		if unicode.IsSpace(r) && r != ' ' && r != '　' {
			return "", false // tab / newline
		}
	}
	out := strings.Join(strings.FieldsFunc(q, func(r rune) bool { return r == ' ' || r == '　' }), " ")
	if out == "" || utf8.RuneCountInString(out) > FeedQueryMaxRunes {
		return "", false
	}
	return out, true
}

func checkFeedQuery(where string, c *Component, add func(string, ...any)) {
	if c.Provider != FeedDiet {
		if c.Query != "" {
			add("%s: feed query is only for provider diet", where)
		}
		return
	}
	if _, ok := NormalizeFeedQuery(c.Query); !ok {
		add("%s: feed diet needs query, 1..%d characters of search words without control characters (got %q)", where, FeedQueryMaxRunes, c.Query)
	}
}
