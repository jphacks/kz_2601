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
