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

    func testItemLinkIsHTTPSOnly() {
        XCTAssertNotNil(FeedItem(title: "a", url: "https://www.digital.go.jp/news/1").link)
        XCTAssertNil(FeedItem(title: "a", url: "http://www.digital.go.jp/").link)
        XCTAssertNil(FeedItem(title: "a", url: "javascript:alert(1)").link)
        XCTAssertNil(FeedItem(title: "a").link)
    }
}
#endif
