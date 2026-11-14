import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// H3・M3: 無料枠切れ・402 `pro_required`・上限(429)で作らずに止めても、聞き返しに答えた会話を消さない。
/// 買ったら(枠が戻ったら)同じ依頼で続きから作る。
@MainActor
final class KeepConversationTests: XCTestCase {
    /// 生成の返し方を切り替えられる ai(対話はモックのまま)。
    final class SwitchAI: AIClient, @unchecked Sendable {
        enum Mode { case ok, proRequired, rateLimited }
        let base = MockAIClient(delay: .milliseconds(20))
        private let lock = NSLock()
        private var _sent: [String] = []
        private var _mode: Mode = .ok
        var sent: [String] { lock.withLock { _sent } }
        var mode: Mode {
            get { lock.withLock { _mode } }
            set { lock.withLock { _mode = newValue } }
        }

        static let feature = PaidFeature(id: "serverRules", title: "サーバーの自動の動き", reason: "r")

        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            lock.withLock { _sent.append(transcript) }
            switch mode {
            case .ok:
                return try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
            case .proRequired:
                throw APIError.server(status: 402, code: "pro_required", message: "this needs Tailor It Pro: serverRules",
                                      details: ["features": .array([.object(["id": .string(Self.feature.id), "title": .string(Self.feature.title), "reason": .string("r")])])])
            case .rateLimited:
                throw APIError.server(status: 429, code: "rate_limited", message: "limit",
                                      details: ["scope": .string("user"), "tier": .string("signed_in"), "limit": .number(60)])
            }
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
        }
        func resetChat(appId: String) async {}
    }

    private let request = "毎日の支出を記録する家計簿アプリを作って"

    private func makeRuntime(freeLimit: Int, enforced: Bool) throws -> (AppRuntime, SwitchAI) {
        let ai = SwitchAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = ai
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = true
        rt.clarifyAutoBuild = false
        let defaults = UserDefaults(suiteName: "KeepConversationTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: freeLimit), limitDisabled: !enforced)
        return (rt, ai)
    }

    /// 対話で ready まで進める(「これでいく」の手前)。
    private func chatUntilReady(_ rt: AppRuntime) async throws -> String {
        await rt.submitUtterance(request)
        let turn = try XCTUnwrap(rt.chatTurn)
        XCTAssertEqual(turn.kind, .ready)
        XCTAssertNotEqual(turn.requiresPro, true)
        return try XCTUnwrap(turn.transcript)
    }

    private func waitForPaywall(_ rt: AppRuntime) async throws -> PaywallRequest {
        let deadline = ContinuousClock.now + .seconds(5)
        while ContinuousClock.now < deadline {
            if let request = rt.proGate.paywall { return request }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("課金画面が出ない")
        throw CancellationError()
    }

    // MARK: - 無料枠切れ

    func testLimitKeepsConversationThenContinuesAfterPurchase() async throws {
        let (rt, ai) = try makeRuntime(freeLimit: 0, enforced: true)
        let text = try await chatUntilReady(rt)
        let bubbles = rt.chatBubbles

        let build = Task { await rt.buildFromChat() }
        let paywall = try await waitForPaywall(rt)
        XCTAssertEqual(paywall.reason, .limitReached)
        rt.proGate.finishPaywall() // 買わずに閉じる
        await build.value

        XCTAssertTrue(ai.sent.isEmpty, "枠切れでは送らない")
        XCTAssertTrue(rt.dsl.isVoiceOnly)
        XCTAssertTrue(rt.isChatting, "会話は消さない")
        XCTAssertEqual(rt.chatBubbles, bubbles)
        XCTAssertEqual(rt.heldBuild?.text, text, "「続きから作る」を出す")
        XCTAssertEqual(rt.chatTurn?.kind, .ready, "「これでいく」もそのまま")

        // 「続きから作る」→ まだ枠が無いので課金画面 → 買う → 同じ依頼で作る
        let resume = Task { await rt.continueHeldBuild() }
        _ = try await waitForPaywall(rt)
        let outcome = try await rt.proGate.store.purchaseOffer()
        XCTAssertEqual(outcome, .purchased)
        rt.proGate.finishPaywall()
        await resume.value

        XCTAssertEqual(ai.sent, [text], "買ったら同じ依頼で作る")
        XCTAssertFalse(rt.dsl.isVoiceOnly, "アプリができる")
        XCTAssertFalse(rt.isChatting, "完成で会話は閉じる")
        XCTAssertNil(rt.heldBuild)
    }

    func testPurchaseInLimitPaywallBuildsRightAway() async throws {
        let (rt, ai) = try makeRuntime(freeLimit: 0, enforced: true)
        let text = try await chatUntilReady(rt)
        let build = Task { await rt.buildFromChat() }
        _ = try await waitForPaywall(rt)
        _ = try await rt.proGate.store.purchaseOffer()
        rt.proGate.finishPaywall()
        await build.value
        XCTAssertEqual(ai.sent, [text], "課金画面で買ったら、話し直さずにそのまま作る")
        XCTAssertFalse(rt.dsl.isVoiceOnly)
        XCTAssertNil(rt.heldBuild)
    }

    /// 枠が戻った(翌日・制限の無効化)なら「続きから作る」は課金画面を出さずに作る
    func testContinueWithoutPaywallWhenQuotaIsBack() async throws {
        let (rt, ai) = try makeRuntime(freeLimit: 0, enforced: true)
        let text = try await chatUntilReady(rt)
        let build = Task { await rt.buildFromChat() }
        _ = try await waitForPaywall(rt)
        rt.proGate.finishPaywall()
        await build.value
        XCTAssertNotNil(rt.heldBuild)

        // 枠が戻った相当(翌日): 同じ端末の記録のまま上限だけ広い司令塔に替える
        let defaults = UserDefaults(suiteName: "KeepConversationTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 3), limitDisabled: false)
        await rt.continueHeldBuild()
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(ai.sent, [text])
        XCTAssertFalse(rt.dsl.isVoiceOnly)
    }

    func testDismissHeldCardReturnsToChatAndTalkingAgainClearsIt() async throws {
        let (rt, _) = try makeRuntime(freeLimit: 0, enforced: true)
        _ = try await chatUntilReady(rt)
        let build = Task { await rt.buildFromChat() }
        _ = try await waitForPaywall(rt)
        rt.proGate.finishPaywall()
        await build.value
        rt.dismissHeldBuild()
        XCTAssertNil(rt.heldBuild)
        XCTAssertTrue(rt.isChatting, "カードを閉じても会話は残る")

        let again = Task { await rt.buildFromChat() }
        _ = try await waitForPaywall(rt)
        rt.proGate.finishPaywall()
        await again.value
        XCTAssertNotNil(rt.heldBuild)
        await rt.submitUtterance("やっぱり日記アプリにして")
        XCTAssertNil(rt.heldBuild, "話し直したらカードは閉じる")
    }

    // MARK: - 402 pro_required

    func testProRequiredKeepsConversationWithPaidCard() async throws {
        let (rt, ai) = try makeRuntime(freeLimit: 100, enforced: false)
        let text = try await chatUntilReady(rt)
        ai.mode = .proRequired

        let build = Task { await rt.buildFromChat() }
        let paywall = try await waitForPaywall(rt)
        XCTAssertEqual(paywall.reason, .paidFeature([SwitchAI.feature]))
        rt.proGate.finishPaywall()
        await build.value

        XCTAssertTrue(rt.isChatting, "402 でも会話は消さない")
        XCTAssertTrue(rt.dsl.isVoiceOnly)
        let card = try XCTUnwrap(rt.paidConfirm, "チャットに確認カード(購入して作る / この機能なしで作る)")
        XCTAssertEqual(card.features, [SwitchAI.feature])
        XCTAssertEqual(card.paidText, text)
        XCTAssertEqual(rt.paidConfirmMessage, ProRequired(features: [SwitchAI.feature]).notSavedMessage)
        XCTAssertNil(rt.summaryBanner, "会話の中で伝えるのでバナーは出さない")

        // 「この機能なしで作る」
        ai.mode = .ok
        await rt.buildWithoutPaidFeatures()
        XCTAssertEqual(ai.sent.last, card.freeText)
        XCTAssertFalse(rt.dsl.isVoiceOnly, "アプリができる")
        XCTAssertNil(rt.paidConfirm)
    }

    func testProRequiredPurchaseFromCardBuildsTheSameRequest() async throws {
        let (rt, ai) = try makeRuntime(freeLimit: 100, enforced: false)
        let text = try await chatUntilReady(rt)
        ai.mode = .proRequired
        let build = Task { await rt.buildFromChat() }
        _ = try await waitForPaywall(rt)
        rt.proGate.finishPaywall()
        await build.value
        XCTAssertNotNil(rt.paidConfirm)

        // カードの「購入して作る」→ 買う → 同じ依頼で作る
        let purchase = Task { await rt.purchaseAndBuild() }
        let paywall = try await waitForPaywall(rt)
        XCTAssertEqual(paywall.placement, .chat)
        _ = try await rt.proGate.store.purchaseOffer()
        ai.mode = .ok // サーバーが Pro を知った
        rt.proGate.finishPaywall()
        await purchase.value
        XCTAssertEqual(ai.sent, [text, text])
        XCTAssertFalse(rt.dsl.isVoiceOnly)
    }

    // MARK: - 上限(429)

    func testRateLimitKeepsConversationWithNextStep() async throws {
        let (rt, ai) = try makeRuntime(freeLimit: 100, enforced: false)
        _ = try await chatUntilReady(rt)
        ai.mode = .rateLimited
        await rt.buildFromChat()
        XCTAssertTrue(rt.isChatting, "上限でも会話は消さない")
        let card = try XCTUnwrap(rt.errorCard)
        guard case .rateLimited = card.kind else { return XCTFail("上限のカードでない: \(card.kind)") }
        XCTAssertEqual(card.primary, .upgradePro, "次の一手(M3)")
    }

    /// 作れた時はこれまでどおり会話を閉じる
    func testSuccessClosesConversationAndResetsFlag() async throws {
        let (rt, ai) = try makeRuntime(freeLimit: 100, enforced: false)
        _ = try await chatUntilReady(rt)
        ai.mode = .ok
        await rt.buildFromChat()
        XCTAssertFalse(rt.isChatting)
        XCTAssertFalse(rt.keepChatAfterStop, "印は使ったら戻す")
    }
}
