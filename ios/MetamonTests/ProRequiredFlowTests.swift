import XCTest
import MetamonDSL
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// #316: サーバーが有料の機能の保存を 402 `pro_required` で止めた時。課金画面を出し、買えば同じ依頼を送り直す。
/// 閉じたら何も保存されていないことを伝える。課金の制限が端末で無効(Debug の既定)でもサーバーの判定に従う。
@MainActor
final class ProRequiredFlowTests: XCTestCase {
    /// Pro でない間は 402、Pro になったらモックの生成を返す ai。
    final class GatedAI: AIClient, @unchecked Sendable {
        let base = MockAIClient(delay: .milliseconds(30))
        private let lock = NSLock()
        private var _sent: [String] = []
        private var _allowed = false
        var sent: [String] { lock.withLock { _sent } }
        var allowed: Bool {
            get { lock.withLock { _allowed } }
            set { lock.withLock { _allowed = newValue } }
        }

        static let feature = PaidFeature(id: "serverRules", title: "サーバーの自動の動き", reason: "r")

        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            lock.withLock { _sent.append(transcript) }
            guard allowed else {
                throw APIError.server(status: 402, code: "pro_required", message: "this needs Tailor It Pro: serverRules",
                                      details: ["features": .array([.object(["id": .string(Self.feature.id), "title": .string(Self.feature.title), "reason": .string("r")])])])
            }
            return try await base.morph(transcript: transcript, appId: appId, currentVersion: currentVersion)
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
        }
        func resetChat(appId: String) async {}
    }

    private let request = "家計簿アプリを作って"

    private func makeRuntime() throws -> (AppRuntime, GatedAI) {
        let ai = GatedAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = ai
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = false
        let defaults = UserDefaults(suiteName: "ProRequiredFlowTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: true)
        return (rt, ai)
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

    func testDismissSavesNothingAndExplains() async throws {
        let (rt, ai) = try makeRuntime()
        let run = Task { await rt.submitUtterance(request) }
        let paywall = try await waitForPaywall(rt)
        XCTAssertEqual(paywall.reason, .paidFeature([GatedAI.feature]))
        XCTAssertNil(rt.errorCard, "状態カードではなく課金画面")
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertEqual(ai.sent, [request], "閉じたら送り直さない")
        XCTAssertTrue(rt.dsl.isVoiceOnly, "何も保存されていない")
        XCTAssertEqual(rt.summaryBanner?.text, ProRequired(features: [GatedAI.feature]).notSavedMessage)
    }

    func testPurchaseResendsTheSameRequest() async throws {
        let (rt, ai) = try makeRuntime()
        let run = Task { await rt.submitUtterance(request) }
        _ = try await waitForPaywall(rt)
        let outcome = try await rt.proGate.store.purchaseOffer()
        XCTAssertEqual(outcome, .purchased)
        ai.allowed = true // サーバーが Pro を知った
        rt.proGate.finishPaywall()
        await run.value
        XCTAssertEqual(ai.sent, [request, request], "買ったら同じ依頼をもう一度送る")
        XCTAssertFalse(rt.dsl.isVoiceOnly)
    }

    /// 端末では Pro なのにサーバーが 402(取引の送信がまだ)。課金画面を出さず、送り直しも繰り返さない。
    func testProOnDeviceButNotOnServerDoesNotLoop() async throws {
        let (rt, ai) = try makeRuntime()
        let outcome = try await rt.proGate.store.purchaseOffer()
        XCTAssertEqual(outcome, .purchased)
        await rt.submitUtterance(request)
        XCTAssertNil(rt.proGate.paywall)
        XCTAssertEqual(ai.sent, [request])
        XCTAssertNotNil(rt.summaryBanner)
    }

    /// 共有の記録の書き込みの 402: 参加者は作成者の Pro が要る(課金画面は出さない)
    func testWriteMessages() {
        let api = APIError.server(status: 402, code: "pro_required", message: "x", details: ["reason": .string("creator_needs_pro")])
        let participant = try? XCTUnwrap(ProRequired(api))
        XCTAssertEqual(participant?.creatorNeedsPro, true)
        let (rt, _) = try! makeRuntime()
        rt.presentProRequiredWrite(participant!)
        XCTAssertNil(rt.proGate.paywall, "参加者には課金画面を出さない")
        XCTAssertEqual(rt.toast, participant?.writeMessage)
        XCTAssertEqual(rt.message(for: api, entity: nil), participant?.writeMessage)
    }
}
