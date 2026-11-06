#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #348: feed の行の表示の組み立て。
@MainActor
final class FeedLeafTests: XCTestCase {
    func testTemps() {
        XCTAssertEqual(FeedLeaf.temps(FeedItem(title: "雨", max: 13.4, min: 8.6)), "13° / 9°")
        XCTAssertEqual(FeedLeaf.temps(FeedItem(title: "雨", max: 13)), "13°")
        XCTAssertNil(FeedLeaf.temps(FeedItem(title: "雨")))
    }

    func testDateText() {
        XCTAssertEqual(FeedLeaf.dateText("2026-11-07", provider: "weather"), "11/7(土)")
        XCTAssertEqual(FeedLeaf.dateText("2026-11-23", provider: "holidays"), "11/23(月)")
        XCTAssertEqual(FeedLeaf.dateText("2026-11-05T18:30:00+09:00", provider: "rss"), "11/5 18:30")
        XCTAssertNil(FeedLeaf.dateText("きのう", provider: "rss"))
        XCTAssertNil(FeedLeaf.dateText(nil, provider: "weather"))
    }

    func testIconAndAttribution() {
        XCTAssertEqual(FeedLeaf.icon("weather"), "cloud.sun")
        XCTAssertEqual(FeedLeaf.icon("rss"), "newspaper")
        let r = FeedResult(provider: "holidays", title: "国民の祝日", items: [], source: FeedSource(name: "内閣府ホームページ", url: "https://www8.cao.go.jp/", license: "公共データ利用規約(第1.0版)"))
        XCTAssertTrue(FeedLeaf.attribution(r).contains("内閣府ホームページ"))
    }
}
#endif
