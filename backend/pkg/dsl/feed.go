package dsl

import (
	"slices"
	"strings"
)

// feed (#348): read-only data from an operator-allowlisted public source,
// fetched by appconfig (GET /v1/feeds/{provider}) and shown as a list. The
// DSL names a provider from a closed set plus that provider's typed params;
// it can never carry a URL. Nothing about the user or the app's records is
// sent out: weather / warnings / pressure take a forecast region code,
// holidays nothing, rss the id of an allowlisted feed. docs/backend/features/external-feeds.md

// Feed providers (Component.Provider).
const (
	FeedWeather  = "weather"  // 気象庁の府県天気予報(Component.Region)
	FeedHolidays = "holidays" // 内閣府の国民の祝日
	FeedRSS      = "rss"      // 運営が許可したフィード(Component.FeedID)
	FeedWarnings = "warnings" // 気象庁の警報・注意報(Component.Region)
	FeedPressure = "pressure" // MET Norway の気圧の予報(Component.Region の代表地点)
)

// FeedProviders is the closed set of feed.provider values.
var FeedProviders = map[string]bool{FeedWeather: true, FeedHolidays: true, FeedRSS: true, FeedWarnings: true, FeedPressure: true}

// FeedUsesRegion reports whether a provider reads Component.Region (a key of
// WeatherRegions). The same 56 forecast office codes serve all three.
func FeedUsesRegion(provider string) bool {
	return provider == FeedWeather || provider == FeedWarnings || provider == FeedPressure
}

// Feed list size (Component.Limit; 0 = FeedDefaultLimit).
const (
	FeedDefaultLimit = 5
	FeedMaxLimit     = 20
)

// WeatherRegions maps a JMA forecast office code (府県予報区) to its name.
// The server fetches only these codes. 十勝 and 奄美 are served by the
// 釧路・根室 and 鹿児島 offices' files, so they are not separate entries.
var WeatherRegions = map[string]string{
	"011000": "宗谷地方", "012000": "上川・留萌地方", "013000": "網走・北見・紋別地方",
	"014100": "釧路・根室・十勝地方", "015000": "胆振・日高地方", "016000": "石狩・空知・後志地方", "017000": "渡島・檜山地方",
	"020000": "青森県", "030000": "岩手県", "040000": "宮城県", "050000": "秋田県", "060000": "山形県", "070000": "福島県",
	"080000": "茨城県", "090000": "栃木県", "100000": "群馬県", "110000": "埼玉県", "120000": "千葉県", "130000": "東京都", "140000": "神奈川県",
	"150000": "新潟県", "160000": "富山県", "170000": "石川県", "180000": "福井県", "190000": "山梨県", "200000": "長野県",
	"210000": "岐阜県", "220000": "静岡県", "230000": "愛知県", "240000": "三重県",
	"250000": "滋賀県", "260000": "京都府", "270000": "大阪府", "280000": "兵庫県", "290000": "奈良県", "300000": "和歌山県",
	"310000": "鳥取県", "320000": "島根県", "330000": "岡山県", "340000": "広島県", "350000": "山口県",
	"360000": "徳島県", "370000": "香川県", "380000": "愛媛県", "390000": "高知県",
	"400000": "福岡県", "410000": "佐賀県", "420000": "長崎県", "430000": "熊本県", "440000": "大分県", "450000": "宮崎県",
	"460100": "鹿児島県", "471000": "沖縄本島地方", "472000": "大東島地方", "473000": "宮古島地方", "474000": "八重山地方",
}

// RSSFeeds maps an allowlisted feed id to its name. The URLs live on the
// server (internal/feeds); adding one is a reviewed change to both tables.
var RSSFeeds = map[string]string{
	"digital-news":  "デジタル庁 新着・更新",
	"fdma-news":     "消防庁 新着情報",
	"fdma-disaster": "消防庁 災害情報",
}

func checkFeed(where string, c *Component, add func(string, ...any)) {
	if !FeedProviders[c.Provider] {
		add("%s: feed provider must be weather, warnings, pressure, holidays or rss (got %q); other sources and URLs are not available", where, c.Provider)
		return
	}
	if FeedUsesRegion(c.Provider) {
		if _, ok := WeatherRegions[c.Region]; !ok {
			add("%s: feed %s needs region, a forecast office code such as 130000 (東京都) or 170000 (石川県) (got %q)", where, c.Provider, c.Region)
		}
	} else if c.Region != "" {
		add("%s: feed region is only for provider weather, warnings or pressure", where)
	}
	if c.Provider == FeedRSS {
		if _, ok := RSSFeeds[c.FeedID]; !ok {
			add("%s: feed rss needs feedId, one of the allowlisted feeds (%s) (got %q)", where, strings.Join(FeedIDs(), ", "), c.FeedID)
		}
	} else if c.FeedID != "" {
		add("%s: feed feedId is only for provider rss", where)
	}
	if c.Limit < 0 || c.Limit > FeedMaxLimit {
		add("%s: feed limit must be 1..%d (0 = %d)", where, FeedMaxLimit, FeedDefaultLimit)
	}
}

// FeedIDs lists the allowlisted rss feed ids, sorted.
func FeedIDs() []string {
	ids := make([]string, 0, len(RSSFeeds))
	for id := range RSSFeeds {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
