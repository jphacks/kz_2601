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
        XCTAssertEqual(FeedLeaf.dateText("2026-11-07T15:31:00+09:00", provider: "warnings"), "11/7 15:31")
        XCTAssertEqual(FeedLeaf.dateText("2026-11-09T06:00:00+09:00", provider: "pressure"), "11/9(月)")
        XCTAssertEqual(FeedLeaf.dateText("2026-11-06T03:59:29+09:00", provider: "arxiv"), "11/6 3:59")
        XCTAssertEqual(FeedLeaf.dateText("2026-10-23", provider: "diet"), "10/23(金)")
    }

    func testPressureRangeAndSparkline() {
        XCTAssertEqual(FeedLeaf.pressureRange(FeedItem(title: "0時〜6時", max: 1013.4, min: 1006.2)), "1006〜1013 hPa")
        XCTAssertEqual(FeedLeaf.pressureRange(FeedItem(title: "0時〜6時", max: 1013.2, min: 1012.8)), "1013 hPa")
        XCTAssertNil(FeedLeaf.pressureRange(FeedItem(title: "0時〜6時")))
        let pts = PressureSparkline.points([1010, 1005, 1000], in: CGSize(width: 40, height: 10))
        XCTAssertEqual(pts, [CGPoint(x: 0, y: 0), CGPoint(x: 20, y: 5), CGPoint(x: 40, y: 10)])
        XCTAssertEqual(PressureSparkline.points([1000, 1000], in: CGSize(width: 10, height: 10)).map(\.y), [5, 5])
        XCTAssertTrue(PressureSparkline.points([1000], in: CGSize(width: 10, height: 10)).isEmpty)
    }

    func testIconAndAttribution() {
        XCTAssertEqual(FeedLeaf.icon("weather"), "cloud.sun")
        XCTAssertEqual(FeedLeaf.icon("rss"), "newspaper")
        XCTAssertEqual(FeedLeaf.icon("warnings"), "exclamationmark.triangle")
        XCTAssertEqual(FeedLeaf.icon("arxiv"), "doc.text")
        XCTAssertEqual(FeedLeaf.icon("diet"), "building.columns")
        let w = FeedResult(provider: "warnings", title: "石川県の警報・注意報", items: [], source: FeedSource(name: "気象庁ホームページ", url: "https://www.jma.go.jp/bosai/warning/", license: "公共データ利用規約(第1.0版)"))
        XCTAssertTrue(FeedLeaf.attribution(w).contains("気象庁"))
        let r = FeedResult(provider: "holidays", title: "国民の祝日", items: [], source: FeedSource(name: "内閣府ホームページ", url: "https://www8.cao.go.jp/", license: "公共データ利用規約(第1.0版)"))
        XCTAssertTrue(FeedLeaf.attribution(r).contains("内閣府ホームページ"))
    }
}
#endif
