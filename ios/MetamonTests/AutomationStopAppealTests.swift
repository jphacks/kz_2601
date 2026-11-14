#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// #346: 運営が止めた時だけ、停止の案内の下に申し立て(お問い合わせ)の行を出す。
@MainActor
final class AutomationStopAppealTests: XCTestCase {
    private func rule(stopped: AutomationStop? = nil, result: AutomationRule.Result = .ok) -> AutomationRule {
        AutomationRule(id: "r", description: "x", result: result, stopped: stopped)
    }

    func testHiddenWhenNothingStopped() {
        XCTAssertFalse(AutomationsView.showsStopAppeal(AppAutomations(appId: "a", rules: [rule()])))
        XCTAssertFalse(AutomationsView.showsStopAppeal(AppAutomations(appId: "a")))
    }

    /// 失敗が続いているだけ(運営は止めていない)なら申し立ては出さない(下のお問い合わせは出る)。
    func testHiddenForFailuresWithoutOperatorStop() {
        let a = AppAutomations(appId: "a", rules: [rule(result: .failed)])
        XCTAssertFalse(AutomationsView.showsStopAppeal(a))
        XCTAssertTrue(AutomationsView.needsContact(a))
    }

    func testShownWhenAppStopped() {
        XCTAssertTrue(AutomationsView.showsStopAppeal(AppAutomations(appId: "a", stopped: AutomationStop())))
    }

    func testShownWhenNotifyPaused() {
        XCTAssertTrue(AutomationsView.showsStopAppeal(AppAutomations(appId: "a", notifyPaused: AutomationStop(until: Date()))))
    }

    func testShownWhenRuleStopped() {
        XCTAssertTrue(AutomationsView.showsStopAppeal(AppAutomations(appId: "a", rules: [rule(stopped: AutomationStop())])))
        XCTAssertTrue(AutomationsView.showsStopAppeal(AppAutomations(appId: "a", rules: [rule(result: .stopped)])))
    }

    /// 申し立ての行は既存の「お問い合わせ」と同じ窓口を開く(新しい宛先を作らない)。
    func testUsesExistingContactRoute() {
        XCTAssertEqual(LegalDocument.contactURL.absoluteString, "https://tailor-it.app/contact")
    }
}
#endif
