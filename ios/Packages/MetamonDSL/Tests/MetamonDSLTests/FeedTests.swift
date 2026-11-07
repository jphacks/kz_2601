#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonDSL

/// #348: feed の部品から問い合わせを作る(URL は持たない・provider ごとのパラメータだけ)。
final class FeedTests: XCTestCase {
    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    func testQueryFromComponent() throws {
        let w = try XCTUnwrap(FeedQuery(try component(#"{"type":"feed","provider":"weather","region":"130000","feedId":"x","limit":7}"#)))
        XCTAssertEqual(w, FeedQuery(provider: "weather", region: "130000", limit: 7), "weather は feedId を送らない")
        XCTAssertEqual(w.path, "v1/feeds/weather?region=130000&limit=7")
        let r = try XCTUnwrap(FeedQuery(try component(#"{"type":"feed","provider":"rss","feedId":"digital-news","region":"1","limit":99}"#)))
        XCTAssertEqual(r.path, "v1/feeds/rss?feedId=digital-news&limit=20", "limit は 1〜20")
        let h = try XCTUnwrap(FeedQuery(try component(#"{"type":"feed","provider":"holidays"}"#)))
        XCTAssertEqual(h.path, "v1/feeds/holidays?limit=5")
        XCTAssertNil(FeedQuery(try component(#"{"type":"feed","provider":"https://evil.example"}"#)))
        XCTAssertNil(FeedQuery(try component(#"{"type":"feed"}"#)))
    }

    func testWarningsAndPressureSendRegion() throws {
        let w = try XCTUnwrap(FeedQuery(try component(#"{"type":"feed","provider":"warnings","region":"170000","feedId":"x"}"#)))
        XCTAssertEqual(w.path, "v1/feeds/warnings?region=170000&limit=5")
        let p = try XCTUnwrap(FeedQuery(try component(#"{"type":"feed","provider":"pressure","region":"016000","limit":8}"#)))
        XCTAssertEqual(p.path, "v1/feeds/pressure?region=016000&limit=8")
    }

    func testArxivAndDietQueries() throws {
        let a = try XCTUnwrap(FeedQuery(try component(#"{"type":"feed","provider":"arxiv","feedId":"cs.CL","query":"x","limit":10}"#)))
        XCTAssertEqual(a.path, "v1/feeds/arxiv?feedId=cs.CL&limit=10", "arxiv は q を送らない")
        let d = try XCTUnwrap(FeedQuery(try component(#"{"type":"feed","provider":"diet","query":"こども 予算+α&x=1","feedId":"cs.AI"}"#)))
        XCTAssertNil(d.feedId)
        XCTAssertEqual(d.path, "v1/feeds/diet?q=%E3%81%93%E3%81%A9%E3%82%82%20%E4%BA%88%E7%AE%97%2B%CE%B1%26x%3D1&limit=5")
        XCTAssertNil(FeedQuery(try component(#"{"type":"feed","provider":"diet"}"#)), "語の無い diet は問い合わせない")
        XCTAssertNil(FeedQuery(try component(#"{"type":"feed","provider":"nhk","feedId":"cat0"}"#)))
    }

    func testDecodesDietRow() throws {
        let json = #"{"provider":"diet","title":"国会会議録「こども家庭庁」","items":[{"date":"2026-10-23","title":"中司宏(日本維新の会)","subtitle":"参議院 こども・子育て・若者活躍に関する特別委員会 第2号","url":"https://kokkai.ndl.go.jp/txt/122215386X00220261023/3"}],"source":{"name":"国立国会図書館 国会会議録検索システム","url":"https://kokkai.ndl.go.jp/","license":"検索用 API"}}"#
        let r = try JSONDecoder().decode(FeedResult.self, from: Data(json.utf8))
        XCTAssertEqual(r.items.first?.link?.host(), "kokkai.ndl.go.jp")
        XCTAssertEqual(r.items.first?.subtitle, "参議院 こども・子育て・若者活躍に関する特別委員会 第2号")
    }

    func testDecodesPressureRow() throws {
        let json = #"{"provider":"pressure","title":"石川県の気圧","items":[{"date":"2026-11-09T00:00:00+09:00","title":"0時〜6時","min":1027.9,"max":1029.5,"drop":true,"series":[1029.5,1028.7,1027.9]}],"source":{"name":"MET Norway","url":"https://api.met.no/","license":"CC BY 4.0"}}"#
        let r = try JSONDecoder().decode(FeedResult.self, from: Data(json.utf8))
        XCTAssertEqual(r.items.first?.drop, true)
        XCTAssertEqual(r.items.first?.series, [1029.5, 1028.7, 1027.9])
    }

    func testItemLinkIsHTTPSOnly() {
        XCTAssertNotNil(FeedItem(title: "a", url: "https://www.digital.go.jp/news/1").link)
        XCTAssertNil(FeedItem(title: "a", url: "http://www.digital.go.jp/").link)
        XCTAssertNil(FeedItem(title: "a", url: "javascript:alert(1)").link)
        XCTAssertNil(FeedItem(title: "a").link)
    }
}
#endif
