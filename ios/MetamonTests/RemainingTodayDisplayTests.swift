import XCTest
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #630 M1: 音声のシート・設定の「今日の残り」の出し分け(少ない・0・Pro・古い)。
@MainActor
final class RemainingTodayDisplayTests: XCTestCase {
    private var saved: AppLanguage = .system
    private let now = ISO8601DateFormatter().date(from: "2026-11-14T03:00:00Z")!
    private let reset = ISO8601DateFormatter().date(from: "2026-11-14T15:00:00Z")!

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    private func value(_ n: Int, limit: Int? = 60) -> RemainingToday {
        RemainingToday(remaining: n, limit: limit, resetAt: reset, observedAt: now)
    }

    func testHiddenWhenUnknownOrStale() {
        XCTAssertEqual(RemainingTodayDisplay.of(nil, now: now, isPro: false, isAnonymous: false), .hidden)
        XCTAssertEqual(RemainingTodayDisplay.of(value(20), now: reset, isPro: false, isAnonymous: false), .hidden, "日付が変わったら古い数を出さない")
    }

    func testNormalAndLow() {
        XCTAssertEqual(RemainingTodayDisplay.of(value(24), now: now, isPro: false, isAnonymous: false), .normal(24, pro: false))
        XCTAssertEqual(RemainingTodayDisplay.of(value(7), now: now, isPro: false, isAnonymous: false), .normal(7, pro: false))
        XCTAssertEqual(RemainingTodayDisplay.of(value(6), now: now, isPro: false, isAnonymous: false), .low(6, pro: false), "新しく作るのがあと 2 回分で少ない扱い")
        XCTAssertEqual(RemainingTodayDisplay.of(value(1), now: now, isPro: false, isAnonymous: false), .low(1, pro: false))
    }

    func testProKeepsTheMarkAndCount() {
        XCTAssertEqual(RemainingTodayDisplay.of(value(280, limit: 300), now: now, isPro: true, isAnonymous: false), .normal(280, pro: true))
        XCTAssertEqual(RemainingTodayDisplay.of(value(3, limit: 300), now: now, isPro: true, isAnonymous: false), .low(3, pro: true))
        XCTAssertEqual(RemainingTodayDisplay.low(3, pro: true).accessibilityLabel, "Pro。今日の残り 3。少なくなっています")
    }

    func testZeroHandsOffToTheRateLimitCard() throws {
        let d = RemainingTodayDisplay.of(value(0, limit: 30), now: now, isPro: false, isAnonymous: true)
        guard case .exhausted(let info) = d else { return XCTFail("\(d)") }
        XCTAssertEqual(info, RateLimitInfo(scope: .user, tier: "anonymous", limit: 30, remaining: 0, resetAt: reset))
        let card = try XCTUnwrap(ErrorPresentation.from(.rateLimited(info), transcript: ""))
        XCTAssertEqual(card.title, "今日はここまでです", "0 の時は既存の上限のカードの文言")
        XCTAssertNil(card.primary)
        if case .exhausted(let pro) = RemainingTodayDisplay.of(value(0, limit: 300), now: now, isPro: true, isAnonymous: true) {
            XCTAssertEqual(pro.tier, "pro")
        } else { XCTFail() }
    }

    func testCopyAndVoiceOver() {
        XCTAssertEqual(RemainingTodayDisplay.normal(24, pro: false).accessibilityLabel, "今日の残り 24")
        XCTAssertTrue(RemainingTodayDisplay.hint(resetAt: nil).contains("新しく作ると 3、直すと 2、相談すると 1 減ります"))
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(RemainingTodayDisplay.label(24), "24 left today")
        XCTAssertEqual(RemainingTodayDisplay.low(2, pro: false).accessibilityLabel, "2 left today. Running low")
        XCTAssertTrue(RemainingTodayDisplay.hint(resetAt: nil).hasPrefix("Creating an app uses 3, editing 2, and chatting 1."), RemainingTodayDisplay.hint(resetAt: nil))
    }

    func testSettingsAmount() {
        XCTAssertEqual(RemainingTodaySettingsSection.amount(value(24)), "24 / 60")
        XCTAssertEqual(RemainingTodaySettingsSection.amount(value(5, limit: nil)), "5")
        XCTAssertEqual(RemainingTodaySettingsSection.amount(nil), "AI を使うと表示されます")
        XCTAssertTrue(RemainingTodaySettingsSection.isLow(value(6)))
        XCTAssertFalse(RemainingTodaySettingsSection.isLow(nil))
    }
}
