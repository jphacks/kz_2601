import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 声で「歩数計作って」→ 対話の ready → 自動で生成(stream)→ `event: error` の `app_limit_reached`(#630)。
/// 置き換えのカード(+ シート)を出し、聞き取りの失敗(「うまく聞き取れませんでした」)や「言い方を変えて」は出さない。
@MainActor
final class StreamAppLimitFlowTests: XCTestCase {
    /// 対話はモックのまま、途中経過付きの生成(stream)だけ決めたエラーで断る ai。
    final class RefusingAI: AIClient, @unchecked Sendable {
        let base = MockAIClient(delay: .milliseconds(20))
        let error: Error
        private let lock = NSLock()
        private var _streamed = 0
        var streamed: Int { lock.withLock { _streamed } }

        init(error: Error) { self.error = error }

        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult { throw error }
        func morph(transcript: String, appId: String, currentVersion: Int, locale: String?,
                   progress: @escaping @Sendable (GenerateProgress) -> Void) async throws -> MorphResult {
            lock.withLock { _streamed += 1 }
            throw error
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            try await base.chat(appId: appId, messages: messages, locale: locale, mode: mode)
        }
        func resetChat(appId: String) async {}
    }

    /// ai の handler.go が stream で返す上限の断り(`status` があれば 402)。
    private func appLimitError(status: Int = 402) -> APIError {
        .server(status: status, code: "app_limit_reached", message: "持てるアプリの数(2 個)に達しました。",
                details: ["resource": .string("apps"), "plan": .string("free"), "held": .number(2), "limit": .number(2),
                          "base": .number(2), "purchasedSlots": .number(0), "canBuySlot": .bool(false), "slotCoins": .number(0),
                          "upgrade": .bool(true),
                          "apps": .array([.object(["id": .string("a-old"), "name": .string("家計簿"), "mine": .bool(true),
                                                   "updatedAt": .string("2026-11-01T00:00:00Z")])])])
    }

    private func makeRuntime(_ ai: RefusingAI) throws -> AppRuntime {
        var services = BuildConfig.make(environment: .mock)
        services.ai = ai
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: services)
        rt.clarifyEnabled = true
        rt.clarifyAutoBuild = true
        let defaults = UserDefaults(suiteName: "StreamAppLimitFlowTests.\(UUID().uuidString)")!
        rt.proGate = ProGate(store: ProEntitlementStore(mode: .mock), usage: CreationUsageStore(defaults: defaults),
                             quota: CreationQuota(dailyFreeLimit: 100), limitDisabled: true)
        return rt
    }

    /// 声で話して ready → 自動の生成が断られて状態カードが出るまで待つ。
    private func speakAndWaitForCard(_ rt: AppRuntime, _ ai: RefusingAI) async throws -> ErrorPresentation {
        await rt.submitUtterance("歩数計作って", source: .voice)
        XCTAssertEqual(rt.chatTurn?.kind, .ready)
        let deadline = ContinuousClock.now + .seconds(8)
        while ContinuousClock.now < deadline {
            if ai.streamed > 0, rt.morphJob == nil, let card = rt.errorCard { return card }
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTFail("状態カードが出ない(streamed=\(ai.streamed))")
        throw CancellationError()
    }

    private func assertAppLimitCard(_ card: ErrorPresentation, _ rt: AppRuntime, file: StaticString = #filePath, line: UInt = #line) {
        guard case .appLimit(let limit) = card.kind else {
            return XCTFail("置き換えのカードを出す: \(card.kind)", file: file, line: line)
        }
        XCTAssertEqual(limit.limit, 2, file: file, line: line)
        XCTAssertEqual(limit.apps?.map(\.id), ["a-old"], file: file, line: line)
        XCTAssertEqual(card.primary, .replaceApp, file: file, line: line)
        XCTAssertEqual(card.title, AppLimitCopy.cardTitle, file: file, line: line)
        XCTAssertNotEqual(card.title, ErrorCopy.notHeardTitle, "聞き取りの失敗の文言は出さない", file: file, line: line)
        XCTAssertNotEqual(card.title, ErrorCopy.rejectedTitle, "「言い方を変えて」は出さない", file: file, line: line)
        XCTAssertEqual(card.transcript.isEmpty, false, "置き換えた後に同じ依頼で作り直せる", file: file, line: line)
        XCTAssertTrue(rt.isChatting, "上限で止めた時は会話を消さない(DELETE /v1/chat しない)", file: file, line: line)
        // 置き換えシートをすぐ出す(テストのホストに出し元があればシート、無ければトースト)
        let gate = AppLimitGate.shared
        XCTAssertTrue(gate.request?.limit == limit || rt.toast == AppLimitCopy.toast(limit),
                      "置き換えシート(またはトースト)を出す", file: file, line: line)
        gate.dismiss()
    }

    func testVoiceBuildRefusedByAppLimitShowsReplaceCard() async throws {
        let ai = RefusingAI(error: appLimitError())
        let rt = try makeRuntime(ai)
        let card = try await speakAndWaitForCard(rt, ai)
        assertAppLimitCard(card, rt)
        XCTAssertEqual(ai.streamed, 1, "上限の断りで生成をやり直さない")
    }

    /// stream の `event: error` に status が欠けると 500 として届く。それでも code で上限と分かる(混雑・言い直しにしない)。
    func testVoiceBuildRefusedByAppLimitWithoutStatusStillShowsReplaceCard() async throws {
        let ai = RefusingAI(error: appLimitError(status: 500))
        let rt = try makeRuntime(ai)
        let card = try await speakAndWaitForCard(rt, ai)
        assertAppLimitCard(card, rt)
    }

    /// 上限以外の生成の失敗(知らない 402)は「うまく作れませんでした」。聞き取り・言い直しの文言にしない。
    func testOtherStreamRefusalUsesGenerationFailureCopy() async throws {
        let ai = RefusingAI(error: APIError.server(status: 402, code: "something_new", message: "x"))
        let rt = try makeRuntime(ai)
        let card = try await speakAndWaitForCard(rt, ai)
        XCTAssertEqual(card.title, ErrorCopy.failedTitle)
        XCTAssertNotEqual(card.title, ErrorCopy.notHeardTitle)
        XCTAssertNotEqual(card.title, ErrorCopy.rejectedTitle)
    }

    /// 生成の失敗の分類は、どれも聞き取りの失敗の文言(`voiceFailure` だけが出す)にならない。
    func testNoGenerateFailureUsesNotHeardCopy() {
        let kinds: [FailureKind] = [
            .offline, .timeout, .sessionExpired, .appCheckFailed, .aiPaused, .freePlanPaused(resumeAt: nil),
            .appLimit(AppLimit(held: 2, limit: 2)), .appStopped, .busy(retryAfter: nil),
            .rateLimited(RateLimitInfo(scope: .user)), .rejected("x"), .other("x"),
        ]
        for kind in kinds {
            guard let card = ErrorPresentation.from(kind, transcript: "歩数計作って") else { continue }
            XCTAssertNotEqual(card.title, ErrorCopy.notHeardTitle, "\(kind)")
            XCTAssertNotEqual(card.body, ErrorCopy.notHeardBody, "\(kind)")
        }
    }
}
