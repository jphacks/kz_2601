package feeds

import (
	"os"
	"testing"

	"golang.org/x/text/encoding/japanese"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// soumuRDF is shaped like www.soumu.go.jp/news.rdf (RSS 1.0, Shift_JIS,
// CRLF, rdf:Seq before the items). ① is a Windows-31J character.
const soumuRDF = "<?xml version=\"1.0\" encoding=\"Shift_JIS\" ?>\r\n" +
	`<rdf:RDF xmlns="http://purl.org/rss/1.0/" xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:dc="http://purl.org/dc/elements/1.1/" xml:lang="ja">` + "\r\n" +
	`<channel rdf:about="https://www.soumu.go.jp/news.rdf"><title>総務省 ホームページ新着情報</title><link>https://www.soumu.go.jp/</link>` +
	`<items><rdf:Seq><rdf:li rdf:resource="https://www.soumu.go.jp/menu_news/s-news/example01.html" /><rdf:li rdf:resource="https://www.soumu.go.jp/menu_news/s-news/example02.html" /></rdf:Seq></items></channel>` + "\r\n" +
	`<item rdf:about="https://www.soumu.go.jp/menu_news/s-news/example02.html"><title>例の研究会①の開催</title><description>例の研究会①の開催</description>` +
	`<link>https://www.soumu.go.jp/menu_news/s-news/example02.html</link><dc:date>2026-11-10T14:00:00+09:00</dc:date><dc:subject>新着情報</dc:subject><dc:creator>総務省</dc:creator></item>` + "\r\n" +
	`<item rdf:about="https://www.soumu.go.jp/menu_news/s-news/example01.html"><title>例の検討会の開催</title><description>例の検討会の開催</description>` +
	`<link>https://www.soumu.go.jp/menu_news/s-news/example01.html</link><dc:date>2026-11-11T05:00:00+09:00</dc:date><dc:subject>新着情報</dc:subject><dc:creator>総務省</dc:creator></item>` + "\r\n" +
	"</rdf:RDF>\r\n"

func TestParseAddedFeeds(t *testing.T) {
	sjis, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(soumuRDF))
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, tc := range []struct {
		id   string
		body []byte
		want []Item // titles, links and JST dates, newest first
	}{
		{"soumu-news", sjis, []Item{
			{Title: "例の検討会の開催", URL: "https://www.soumu.go.jp/menu_news/s-news/example01.html", Date: "2026-11-11T05:00:00+09:00"},
			{Title: "例の研究会①の開催", URL: "https://www.soumu.go.jp/menu_news/s-news/example02.html", Date: "2026-11-10T14:00:00+09:00"},
		}},
		{"osaka-city-news", read("osaka_rss_new.xml"), []Item{
			{Title: "市営施設の休館日について（例局）", URL: "https://www.city.osaka.lg.jp/example/page/0000000002.html", Date: "2026-11-12T00:00:00+09:00"},
			{Title: "区民まつりのお知らせ（例区役所）", URL: "https://www.city.osaka.lg.jp/example/page/0000000001.html", Date: "2026-11-11T17:00:00+09:00"},
			{Title: "http のリンク（例）", Date: "2026-11-10T09:00:00+09:00"}, // non-https link dropped
		}},
		{"go-blog", read("go_blog_feed.atom"), []Item{
			{Title: "Example post", URL: "https://go.dev/blog/example", Date: "2026-11-10T09:00:00+09:00"},
			{Title: "Older post", URL: "https://go.dev/blog/older", Date: "2026-09-01T09:00:00+09:00"},
		}},
	} {
		if _, ok := dsl.RSSFeeds[tc.id]; !ok {
			t.Fatalf("%s is not in dsl.RSSFeeds", tc.id)
		}
		got, err := parseRSS(tc.body)
		if err != nil {
			t.Fatalf("%s: %v", tc.id, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: items = %+v", tc.id, got)
		}
		for i, w := range tc.want {
			if g := got[i]; g.Title != w.Title || g.URL != w.URL || g.Date != w.Date || g.Subtitle != "" {
				t.Errorf("%s[%d] = %+v, want %+v (headline and link only)", tc.id, i, g, w)
			}
		}
	}
}

// Each feed is attributed as its terms ask; feeds without their own license
// fall back to 公共データ利用規約.
func TestRSSSourceAttribution(t *testing.T) {
	ep := DefaultEndpoints()
	for id, want := range map[string]Source{
		"digital-news":    {Name: "デジタル庁ホームページ", URL: "https://www.digital.go.jp/", License: pdl},
		"fdma-news":       {Name: "消防庁ホームページ", URL: "https://www.fdma.go.jp/", License: pdl},
		"fdma-disaster":   {Name: "消防庁ホームページ", URL: "https://www.fdma.go.jp/", License: pdl},
		"soumu-news":      {Name: "総務省ホームページ", URL: "https://www.soumu.go.jp/", License: pdl},
		"osaka-city-news": {Name: "大阪市ホームページ", URL: "https://www.city.osaka.lg.jp/", License: "RSS は CC BY"},
		"go-blog":         {Name: "The Go Blog(go.dev)", URL: "https://go.dev/blog/", License: "CC BY 4.0"},
	} {
		if got := ep.source(Query{Provider: dsl.FeedRSS, FeedID: id}); got != want {
			t.Errorf("%s source = %+v, want %+v", id, got, want)
		}
	}
	if len(ep.RSS) != 6 {
		t.Errorf("rss feeds = %d; add the new one's attribution here", len(ep.RSS))
	}
}
