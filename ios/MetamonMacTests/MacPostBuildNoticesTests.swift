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

    /// 写し(#1281)は、要約が閉じた後に出る直し方の例(#1278)が閉じるまで撮らない
    func testIsShowingUntilEditHintCloses() {
        let notices = MacPostBuildNotices()
        XCTAssertFalse(notices.isShowing)
        notices.summary = .init(appId: "a", text: "できました", action: nil)
        notices.showsEditHint = true
        notices.dismissActive()
        XCTAssertNil(notices.summary)
        XCTAssertTrue(notices.isShowing, "要約が閉じても直し方の例が出ている")
        notices.dismissActive()
        XCTAssertFalse(notices.isShowing)
    }

    func testEditHintAloneIsActive() {
        let notices = MacPostBuildNotices()
        notices.showsEditHint = true
        XCTAssertEqual(notices.active, .editHint)
    }
}
