import XCTest
import MetamonServices
@testable import Metamon

/// 1 日の枠が切れて作れなかった依頼を残す・消す(#3079)
@MainActor
final class MacHeldBuildTests: XCTestCase {
    /// 審査用ビルド(APP_STORE_REVIEW)は残さない
    private static let holdsUserLimit: Bool = {
        #if APP_STORE_REVIEW
        false
        #else
        true
        #endif
    }()

    private func limited(scope: String) -> APIError {
        .server(status: 429, code: "rate_limited", message: "今日の利用上限に達しました。",
                details: ["scope": .string(scope), "tier": .string("signed_in"), "limit": .number(30), "remaining": .number(0)])
    }

    /// 決まった失敗を返す AI(ネットにはつながない)
    private actor FailingAI: AIClient {
        let error: Error
        private(set) var morphCalls = 0
        init(_ error: Error) { self.error = error }
        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            morphCalls += 1
            throw error
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn { throw error }
        func resetChat(appId: String) async {}
    }

    private func session(_ ai: any AIClient) -> MacComposeSession {
        MacComposeSession(target: .create, ai: ai, reduceMotion: { true }, isConsentGranted: { true },
                          clarifyEnabled: true, clarifyAutoBuild: false, historyStore: ChatHistoryStore())
    }

    private func waitUntil(_ condition: () -> Bool) async throws {
        for _ in 0..<150 where !condition() { try await Task.sleep(for: .milliseconds(20)) }
    }

    func testOnlyUserDailyLimitHolds() {
        XCTAssertEqual(HeldBuild.holds(for: RateLimitInfo(scope: .user)), Self.holdsUserLimit)
        XCTAssertFalse(HeldBuild.holds(for: RateLimitInfo(scope: .ip)))
        XCTAssertFalse(HeldBuild.holds(for: RateLimitInfo(scope: .global)))
    }

    func testHeldBuildForPresentation() {
        let user = ErrorPresentation(kind: .rateLimited(RateLimitInfo(scope: .user)), transcript: "家計簿を作って", origin: .generate)
        XCTAssertEqual(MacComposeSession.heldBuild(for: user)?.text, Self.holdsUserLimit ? "家計簿を作って" : nil)
        let chat = ErrorPresentation(kind: .rateLimited(RateLimitInfo(scope: .user)), transcript: "x", origin: .chat)
        XCTAssertNil(MacComposeSession.heldBuild(for: chat), "作る前の会話の失敗は残さない")
        XCTAssertNil(MacComposeSession.heldBuild(for: ErrorPresentation(kind: .busy(retryAfter: nil), transcript: "x")))
        XCTAssertNil(MacComposeSession.heldBuild(for: nil))
    }

    func testBuildStoppedByDailyLimitKeepsRequestAndConversation() async throws {
        try XCTSkipUnless(Self.holdsUserLimit, "審査用ビルドは残さない")
        let s = session(FailingAI(limited(scope: "user")))
        s.build("家計簿を作って")
        try await waitUntil { s.heldBuild != nil }
        XCTAssertEqual(s.heldBuild?.text, "家計簿を作って")
        XCTAssertNotNil(s.failure)
        XCTAssertEqual(s.phase, .chatting, "会話に戻る")
    }

    func testDismissClearsHeldAndCard() async throws {
        try XCTSkipUnless(Self.holdsUserLimit, "審査用ビルドは残さない")
        let s = session(FailingAI(limited(scope: "user")))
        s.build("家計簿を作って")
        try await waitUntil { s.heldBuild != nil }
        s.dismissHeldBuild()
        XCTAssertNil(s.heldBuild)
        XCTAssertNil(s.failure)
    }

    func testContinueBuildsSameRequestAndHoldsAgainIfStillLimited() async throws {
        try XCTSkipUnless(Self.holdsUserLimit, "審査用ビルドは残さない")
        let ai = FailingAI(limited(scope: "user"))
        let s = session(ai)
        s.build("家計簿を作って")
        try await waitUntil { s.heldBuild != nil }
        s.continueHeldBuild()
        XCTAssertNil(s.heldBuild, "続きから作り始めたらカードは閉じる")
        try await waitUntil { s.heldBuild != nil }
        let calls = await ai.morphCalls
        XCTAssertEqual(calls, 2, "同じ依頼でもう一度作りに行く")
        XCTAssertEqual(s.heldBuild?.text, "家計簿を作って", "まだ枠が無ければまた残る")
    }

    func testOtherFailuresDoNotHold() async throws {
        let s = session(FailingAI(limited(scope: "global")))
        s.build("家計簿を作って")
        try await waitUntil { s.failure != nil }
        XCTAssertNil(s.heldBuild)
    }

    func testTalkingAgainClosesHeld() async throws {
        try XCTSkipUnless(Self.holdsUserLimit, "審査用ビルドは残さない")
        let s = session(FailingAI(limited(scope: "user")))
        s.build("家計簿を作って")
        try await waitUntil { s.heldBuild != nil }
        s.send("やっぱり別のアプリ")
        XCTAssertNil(s.heldBuild, "話し直したら閉じる")
    }
}
