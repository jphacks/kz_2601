import XCTest
@testable import Metamon

/// サイドバーの「同梱の例」は展示する例だけ(42 本全部は並べない)
final class MacShowcaseExamplesTests: XCTestCase {
    private func app(_ id: String) -> SampleApp { SampleApp(id: id, name: id, description: nil, url: nil) }

    func testShowcaseIdsAreBundled() {
        let bundled = Set(SampleAppCatalog.load().map(\.id))
        for id in SampleAppCatalog.showcaseIds {
            XCTAssertTrue(bundled.contains(id), "\(id) が同梱されていない")
        }
        XCTAssertTrue((6...8).contains(SampleAppCatalog.showcaseIds.count))
    }

    func testDevAndUnsupportedExamplesAreHidden() {
        let hidden = ["gallery-display", "gallery-input", "gallery-structure", "gallery-web", "navigation", "initial",
                      "dashcam", "health", "sensors", "walk_route"]
        let shown = Set(SampleAppCatalog.showcase(SampleAppCatalog.load()).map(\.id))
        XCTAssertTrue(shown.isDisjoint(with: hidden))
        XCTAssertEqual(shown.count, SampleAppCatalog.showcaseIds.count)
    }

    func testOrderFollowsShowcaseIds() {
        let apps = SampleAppCatalog.showcaseIds.reversed().map(app)
        XCTAssertEqual(SampleAppCatalog.showcase(apps).map(\.id), SampleAppCatalog.showcaseIds)
    }

    func testKeepsOpenHiddenExample() {
        let apps = [app("kakeibo"), app("navigation")]
        XCTAssertEqual(SampleAppCatalog.showcase(apps, keeping: "navigation").map(\.id), ["kakeibo", "navigation"])
        XCTAssertEqual(SampleAppCatalog.showcase(apps, keeping: "kakeibo").map(\.id), ["kakeibo"])
        XCTAssertEqual(SampleAppCatalog.showcase(apps).map(\.id), ["kakeibo"])
    }
}
