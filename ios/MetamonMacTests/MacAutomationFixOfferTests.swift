#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 直す会話の「自動の動きが続けて失敗しています。直しますか?」(#347・`MacComposeSession.loadAutomationFix`)。
/// 出す条件(直す会話・サーバーのアプリで自動の動きがある・取れた時だけ)と、「直す」が依頼を入力欄に入れる(送らない)こと
@MainActor
final class MacAutomationFixOfferTests: XCTestCase {
    private let appId = "mac-fix-offer"
    private let edit = MacComposeSession.Target.edit(appId: "mac-fix-offer", name: "家計簿", version: 2)

    private final class FakeClient: AutomationsClient, @unchecked Sendable {
        var result: Result<AppAutomations, Error>
        init(_ result: Result<AppAutomations, Error>) { self.result = result }
        func automations(appId: String) async throws -> AppAutomations { try result.get() }
        func setRule(_ ruleId: String, enabled: Bool, appId: String) async throws {}
    }

    private func rule(_ id: String, failures: Int) -> AutomationRule {
        AutomationRule(id: id, description: "毎朝まとめを通知", result: .failed, enabled: true, consecutiveFailures: failures)
    }

    private func client(_ rules: [AutomationRule]) -> FakeClient {
        FakeClient(.success(AppAutomations(appId: appId, rules: rules)))
    }

    private func defaults() -> UserDefaults {
        let name = "MacAutomationFixOfferTests.\(UUID().uuidString)"
        let d = UserDefaults(suiteName: name)!
        addTeardownBlock { d.removePersistentDomain(forName: name) }
        return d
    }

    private func session(_ target: MacComposeSession.Target) -> MacComposeSession {
        MacComposeSession(target: target, ai: MockAIClient(delay: .zero), reduceMotion: { true }, isConsentGranted: { true },
                          clarifyEnabled: true, clarifyAutoBuild: false, historyStore: ChatHistoryStore(defaults: defaults()))
    }

    func testOffersOnlyForEditWithServerRulesAndClient() {
        XCTAssertTrue(MacComposeSession.offersAutomationFix(target: edit, hasServerRules: true, hasClient: true))
        XCTAssertFalse(MacComposeSession.offersAutomationFix(target: .create, hasServerRules: true, hasClient: true), "作る会話では出さない")
        XCTAssertFalse(MacComposeSession.offersAutomationFix(target: edit, hasServerRules: false, hasClient: true), "自動の動きが無いアプリは取りに行かない")
        XCTAssertFalse(MacComposeSession.offersAutomationFix(target: edit, hasServerRules: true, hasClient: false), "サーバーにつながらない時は出さない")
    }

    func testShowsRuleThatFailedThreeTimes() async {
        let s = session(edit)
        await s.loadAutomationFix(client: client([rule("r", failures: 3)]), hasServerRules: true, defaults: defaults())
        XCTAssertEqual(s.automationFix?.rule?.id, "r")
    }

    func testBelowThresholdAndCreateShowNothing() async {
        let few = session(edit)
        await few.loadAutomationFix(client: client([rule("r", failures: 2)]), hasServerRules: true, defaults: defaults())
        XCTAssertNil(few.automationFix?.rule, "続けて 3 回に満たなければ出さない")

        let create = session(.create)
        await create.loadAutomationFix(client: client([rule("r", failures: 5)]), hasServerRules: true, defaults: defaults())
        XCTAssertNil(create.automationFix)
    }

    func testFailureToLoadShowsNothing() async {
        let s = session(edit)
        await s.loadAutomationFix(client: FakeClient(.failure(URLError(.notConnectedToInternet))), hasServerRules: true, defaults: defaults())
        XCTAssertNil(s.automationFix?.rule, "取れなければ何も出さない")
    }

    func testFixFillsDraftWithoutSendingAndIsRemembered() async {
        let d = defaults()
        let r = rule("r", failures: 3)
        let s = session(edit)
        await s.loadAutomationFix(client: client([r]), hasServerRules: true, defaults: d)
        s.applyAutomationFix()
        XCTAssertEqual(s.draft, AutomationsModel.fixRequest(for: r), "依頼が入力欄に入る")
        XCTAssertTrue(s.bubbles.isEmpty, "まだ送らない")
        XCTAssertNil(s.automationFix?.rule)

        let again = session(edit)
        await again.loadAutomationFix(client: client([r]), hasServerRules: true, defaults: d)
        XCTAssertNil(again.automationFix?.rule, "直した連続失敗は聞き直さない")
    }

    func testFixAppendsToTypedDraft() async {
        let r = rule("r", failures: 3)
        let s = session(edit)
        s.draft = "背景を青に"
        await s.loadAutomationFix(client: client([r]), hasServerRules: true, defaults: defaults())
        s.applyAutomationFix()
        XCTAssertEqual(s.draft, "背景を青に\n" + AutomationsModel.fixRequest(for: r))
    }
}
#endif
