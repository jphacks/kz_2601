import XCTest
@testable import Metamon

/// 完成の後のお知らせの順番(#968)。最初のアプリの後の直し方の例(#1278)は要約が閉じてから出る。
@MainActor
final class MacPostBuildNoticesTests: XCTestCase {
    func testEditHintWaitsForSummary() {
        let notices = MacPostBuildNotices()
        notices.summary = .init(appId: "a", text: "できました", action: nil)
        notices.showsEditHint = true
        XCTAssertEqual(notices.active, .summary)

        notices.dismissActive()
        XCTAssertNil(notices.summary)
        XCTAssertEqual(notices.active, .editHint)

        notices.dismissActive()
        XCTAssertFalse(notices.showsEditHint)
        XCTAssertNil(notices.active)
    }

    func testEditHintAloneIsActive() {
        let notices = MacPostBuildNotices()
        notices.showsEditHint = true
        XCTAssertEqual(notices.active, .editHint)
    }
}
