#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// #347: 修正チャットを開いた時の「直しますか?」(どのルールを出すか・閉じた印・取れない時は出さない)。
@MainActor
final class AutomationFixOfferTests: XCTestCase {
    private func rule(_ id: String, failures: Int, enabled: Bool? = true, stopped: Bool = false, lastOkAt: Date? = nil) -> AutomationRule {
        AutomationRule(id: id, description: "説明\(id)", result: .failed, lastOkAt: lastOkAt,
                       stopped: stopped ? AutomationStop() : nil, enabled: enabled, consecutiveFailures: failures)
    }

    private func apps(_ rules: [AutomationRule], stopped: Bool = false) -> AppAutomations {
        AppAutomations(appId: "a", stopped: stopped ? AutomationStop() : nil, rules: rules)
    }

    func testBelowThresholdShowsNothing() {
        XCTAssertNil(AutomationFixOffer.pick(apps([rule("r", failures: 2)]), handled: []))
    }

    func testAtThresholdShows() {
        XCTAssertEqual(AutomationFixOffer.pick(apps([rule("r", failures: 3)]), handled: [])?.id, "r")
    }

    func testPicksMostFailuresThenListOrder() {
        let a = apps([rule("x", failures: 3), rule("y", failures: 5), rule("z", failures: 5)])
        XCTAssertEqual(AutomationFixOffer.pick(a, handled: [])?.id, "y")
    }

    func testSkipsOperatorStoppedAndOff() {
        XCTAssertNil(AutomationFixOffer.pick(apps([rule("r", failures: 4, stopped: true)]), handled: []))
        XCTAssertNil(AutomationFixOffer.pick(apps([rule("r", failures: 4, enabled: false)]), handled: []))
        XCTAssertNil(AutomationFixOffer.pick(apps([rule("r", failures: 4)], stopped: true), handled: []))
        // 古いサーバー(enabled を返さない)は出す
        XCTAssertEqual(AutomationFixOffer.pick(apps([rule("r", failures: 4, enabled: nil)]), handled: [])?.id, "r")
    }

    func testHandledStreakIsSkippedButNextStreakShows() {
        let old = rule("r", failures: 3, lastOkAt: Date(timeIntervalSince1970: 100))
        let handled: Set = [AutomationFixOffer.key(appId: "a", rule: old)]
        XCTAssertNil(AutomationFixOffer.pick(apps([old]), handled: handled))
        // 同じ連続で回数が増えても出さない
        XCTAssertNil(AutomationFixOffer.pick(apps([rule("r", failures: 6, lastOkAt: Date(timeIntervalSince1970: 100))]), handled: handled))
        // 一度うまくいってから、また続けて失敗した = 次の連続
        let next = rule("r", failures: 3, lastOkAt: Date(timeIntervalSince1970: 500))
        XCTAssertEqual(AutomationFixOffer.pick(apps([next]), handled: handled)?.id, "r")
        // 閉じたルールの代わりに、ほかのルールを出す
        XCTAssertEqual(AutomationFixOffer.pick(apps([old, rule("s", failures: 3)]), handled: handled)?.id, "s")
    }

    func testKeyIsPerApp() {
        let r = rule("r", failures: 3)
        XCTAssertNotEqual(AutomationFixOffer.key(appId: "a", rule: r), AutomationFixOffer.key(appId: "b", rule: r))
    }

    // MARK: - Model

    private final class FakeClient: AutomationsClient, @unchecked Sendable {
        var result: Result<AppAutomations, Error>
        init(_ result: Result<AppAutomations, Error>) { self.result = result }
        func automations(appId: String) async throws -> AppAutomations { try result.get() }
        func setRule(_ ruleId: String, enabled: Bool, appId: String) async throws {}
    }

    private func defaults() -> UserDefaults {
        let name = "AutomationFixOfferTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        addTeardownBlock { d.removePersistentDomain(forName: name) }
        return d
    }

    func testFixFillsSameRequestAsSettingsAndIsRemembered() async {
        let d = defaults()
        let r = rule("r", failures: 3)
        let client = FakeClient(.success(apps([r])))
        let m = AutomationFixOfferModel(appId: "a", client: client, defaults: d)
        await m.load()
        XCTAssertEqual(m.rule?.id, "r")
        XCTAssertEqual(m.fix(), AutomationsModel.fixRequest(for: r))
        XCTAssertNil(m.rule)

        let again = AutomationFixOfferModel(appId: "a", client: client, defaults: d)
        await again.load()
        XCTAssertNil(again.rule)
    }

    func testDismissIsRemembered() async {
        let d = defaults()
        let client = FakeClient(.success(apps([rule("r", failures: 3)])))
        let m = AutomationFixOfferModel(appId: "a", client: client, defaults: d)
        await m.load()
        m.dismiss()
        XCTAssertNil(m.rule)
        let again = AutomationFixOfferModel(appId: "a", client: client, defaults: d)
        await again.load()
        XCTAssertNil(again.rule)
    }

    /// 作成者でない(403)・圏外は何も出さない。
    func testFetchFailureShowsNothing() async {
        for error in [APIError.http(status: 403, body: ""), URLError(.notConnectedToInternet)] as [Error] {
            let m = AutomationFixOfferModel(appId: "a", client: FakeClient(.failure(error)), defaults: defaults())
            await m.load()
            XCTAssertNil(m.rule)
        }
    }

    func testStoreKeepsNewestWithinLimit() {
        let store = AutomationFixOfferStore(defaults: defaults())
        for i in 0..<(AutomationFixOfferStore.limit + 5) { store.markHandled("k\(i)") }
        let handled = store.handled()
        XCTAssertEqual(handled.count, AutomationFixOfferStore.limit)
        XCTAssertFalse(handled.contains("k0"))
        XCTAssertTrue(handled.contains("k\(AutomationFixOfferStore.limit + 4)"))
    }
}
#endif
