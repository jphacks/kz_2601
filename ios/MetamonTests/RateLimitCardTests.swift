import XCTest
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #280: ai の 1 日の上限(429 rate_limited)を受けた時の状態カード。何回分か・いつ使えるかを出し、再試行は出さない。
@MainActor
final class RateLimitCardTests: XCTestCase {
    private var saved: AppLanguage = .system
    private let jst = TimeZone(identifier: "Asia/Tokyo")!
    private let reset = ISO8601DateFormatter().date(from: "2026-10-25T15:00:00Z")! // 10/26 0:00 JST

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    private func card(_ info: RateLimitInfo, origin: ErrorPresentation.Origin = .generate) throws -> ErrorPresentation {
        try XCTUnwrap(ErrorPresentation.from(.rateLimited(info), transcript: "家計簿を作って", origin: origin))
    }

    func testUserLimitShowsCountAndResetTime() throws {
        let info = RateLimitInfo(scope: .user, tier: "signed_in", limit: 60, remaining: 0, resetAt: reset)
        let c = try card(info)
        XCTAssertEqual(c.title, "今日はここまでです")
        XCTAssertNil(c.primary, "もう一度では解けないので再試行ボタンは出さない")
        XCTAssertEqual(c.secondary, .dismiss)
        let body = ErrorCopy.rateLimitedBody(info, timeZone: jst)
        XCTAssertEqual(body, "今日の上限(60回分)に達しました。10月26日 0:00からまた使えます")
        XCTAssertFalse(body.contains("サインイン"))
    }

    func testAnonymousIsToldToSignIn() {
        let info = RateLimitInfo(scope: .user, tier: "anonymous", limit: 30, remaining: 1, resetAt: reset)
        let body = ErrorCopy.rateLimitedBody(info, timeZone: jst)
        XCTAssertTrue(body.hasPrefix("今日の上限(30回分)に達しました。10月26日 0:00から"), body)
        XCTAssertTrue(body.hasSuffix("サインインすると上限が増えます"), body)
    }

    func testGlobalBreakerAndIPCopy() throws {
        let global = RateLimitInfo(scope: .global, tier: "signed_in", limit: 3000, resetAt: reset)
        XCTAssertEqual(try card(global).title, "本日の受付を終了しました")
        XCTAssertEqual(ErrorCopy.rateLimitedBody(global, timeZone: jst), "利用が集中したため、新しい生成を止めています。10月26日 0:00に再開します")
        let ip = RateLimitInfo(scope: .ip, tier: "anonymous", limit: 300, resetAt: reset)
        XCTAssertTrue(ErrorCopy.rateLimitedBody(ip, timeZone: jst).contains("サインイン"))
    }

    func testChatOriginAlsoUsesTheCard() throws {
        let c = try card(RateLimitInfo(scope: .user, tier: "anonymous", limit: 30, resetAt: reset), origin: .chat)
        XCTAssertEqual(c.title, "今日はここまでです", "対話中の上限も「質問を用意できませんでした」にしない")
    }

    func testEnglishCopy() {
        LanguagePreference.shared.setting = .en
        let info = RateLimitInfo(scope: .user, tier: "signed_in", limit: 60, resetAt: reset)
        XCTAssertEqual(ErrorCopy.rateLimitedBody(info, timeZone: jst), "You've reached today's limit (60). You can continue from Oct 26, 0:00")
    }
}
