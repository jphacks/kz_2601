import XCTest
@testable import Metamon

/// UX 点検(docs/ux/ux-audit-2026-11.md)の M6・L3・L6 の出し分け。
final class UXAuditMediumLowTests: XCTestCase {
    // MARK: M6 生成中のキャンセルと目安

    func testCancelIsProminentOnlyAfter30Seconds() {
        XCTAssertFalse(GeneratingStage(elapsed: 0).cancelIsProminent)
        XCTAssertFalse(GeneratingStage(elapsed: 29.9).cancelIsProminent)
        XCTAssertTrue(GeneratingStage(elapsed: 30).cancelIsProminent)
    }

    func testEstimateOnlyForCreateInFirstStage() {
        XCTAssertNotNil(GeneratingStage(elapsed: 2).estimate(edit: false))
        XCTAssertNil(GeneratingStage(elapsed: 2).estimate(edit: true), "edit はすぐ終わるので出さない")
        XCTAssertNil(GeneratingStage(elapsed: 12).estimate(edit: false), "目安を過ぎたら段階文言に任せる")
    }

    // MARK: L3 通知がオフのバー

    func testNotificationsOffBarVisibility() {
        XCTAssertTrue(NotificationsOffBarModel.isVisible(denied: true, dismissed: false, busy: false, summaryShowing: false))
        XCTAssertFalse(NotificationsOffBarModel.isVisible(denied: false, dismissed: false, busy: false, summaryShowing: false))
        XCTAssertFalse(NotificationsOffBarModel.isVisible(denied: true, dismissed: true, busy: false, summaryShowing: false))
        XCTAssertFalse(NotificationsOffBarModel.isVisible(denied: true, dismissed: false, busy: true, summaryShowing: false), "生成中は重ねない")
        XCTAssertFalse(NotificationsOffBarModel.isVisible(denied: true, dismissed: false, busy: false, summaryShowing: true), "要約の表示中は重ねない")
    }

    // MARK: L6 FAB を隠した時の案内

    func testFabHideHintOnlyFirstTimeHidden() {
        XCTAssertTrue(FabHideHint.shouldShow(wasHidden: false, nowHidden: true, alreadyShown: false))
        XCTAssertFalse(FabHideHint.shouldShow(wasHidden: false, nowHidden: true, alreadyShown: true), "2 回目は出さない")
        XCTAssertFalse(FabHideHint.shouldShow(wasHidden: false, nowHidden: false, alreadyShown: false), "吸着しただけ")
        XCTAssertFalse(FabHideHint.shouldShow(wasHidden: true, nowHidden: true, alreadyShown: false), "もとから隠れていた")
    }
}
