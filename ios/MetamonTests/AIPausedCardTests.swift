import XCTest
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #511: 運営が AI を一時停止した時(503 ai_paused)の状態カード。混雑・今日の上限とは別の文言を出す。
@MainActor
final class AIPausedCardTests: XCTestCase {
    private var saved: AppLanguage = .system
    private let paused = APIError.server(status: 503, code: "ai_paused", message: "AI を一時的に止めています。")

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testClassifiesAsPausedNotBusy() {
        XCTAssertEqual(FailureKind.classify(paused), .aiPaused)
        XCTAssertEqual(FailureKind.classify(APIError.server(status: 503, code: "upstream_unavailable", message: "m")), .busy(retryAfter: nil),
                       "Claude の障害の 503 は今までどおり混雑")
    }

    func testCardShowsPauseCopy() throws {
        for origin in [ErrorPresentation.Origin.generate, .chat] {
            let c = try XCTUnwrap(ErrorPresentation.from(FailureKind.classify(paused), transcript: "家計簿を作って", origin: origin))
            XCTAssertEqual(c.kind, .aiPaused)
            XCTAssertEqual(c.title, "AI を一時停止しています")
            XCTAssertEqual(c.body, "AI を一時的に止めています。しばらくしてからもう一度お試しください")
            XCTAssertEqual(c.primary, .retry)
            XCTAssertFalse(c.isWarning)
        }
    }

    func testDistinctFromRateLimitCopy() throws {
        let limited = try XCTUnwrap(ErrorPresentation.from(.rateLimited(RateLimitInfo(scope: .global)), transcript: "x"))
        let pausedCard = try XCTUnwrap(ErrorPresentation.from(.aiPaused, transcript: "x"))
        XCTAssertNotEqual(pausedCard.title, limited.title)
        XCTAssertNotEqual(pausedCard.body, limited.body)
    }

    func testEnglishCopy() {
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(ErrorCopy.aiPausedTitle, "AI is paused")
        XCTAssertEqual(ErrorCopy.aiPausedBody, "AI is temporarily paused. Please try again later")
    }
}
