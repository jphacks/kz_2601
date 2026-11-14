import XCTest
import SwiftUI
@testable import Metamon

/// Mac の VoiceOver の読み上げ・動きを減らす・フォーカスの輪・文字の大きさの割合(#937)
@MainActor
final class MacVoiceOverTests: XCTestCase {
    private var posted: [String] = []
    private var savedPost: ((String, MacVoiceOver.Priority) -> Void)!
    private var savedRunning: (() -> Bool)!

    override func setUp() async throws {
        savedPost = MacVoiceOver.post
        savedRunning = MacVoiceOver.isVoiceOverRunning
        posted = []
        MacVoiceOver.post = { [weak self] text, _ in self?.posted.append(text) }
    }

    override func tearDown() async throws {
        MacVoiceOver.post = savedPost
        MacVoiceOver.isVoiceOverRunning = savedRunning
    }

    // MARK: - 読み上げ

    func testAnnounceTrimsAndSkipsEmpty() {
        MacVoiceOver.announce("  保存しました \n")
        MacVoiceOver.announce("   ")
        MacVoiceOver.announce("")
        XCTAssertEqual(posted, ["保存しました"])
    }

    func testAnnounceEventPostsItsText() {
        MacVoiceOver.announce(.recordDeleted)
        MacVoiceOver.announce(.message("書き出しました"))
        XCTAssertEqual(posted, [MacAnnouncement.recordDeleted.text, "書き出しました"])
    }

    /// 続けて出した文が互いに切れないよう、失敗だけ割り込む
    func testOnlyFailuresInterrupt() {
        var priorities: [MacVoiceOver.Priority] = []
        MacVoiceOver.post = { _, priority in priorities.append(priority) }
        MacVoiceOver.announce(.saved)
        MacVoiceOver.announce(.healthSkipped)
        MacVoiceOver.announce("お知らせ")
        MacVoiceOver.announce(.failure("通信できませんでした"))
        XCTAssertEqual(priorities, [.medium, .medium, .medium, .high])
    }

    func testFixedMessagesMatchIOSWording() {
        XCTAssertEqual(MacAnnouncement.saved.text, L10n.tr("保存しました"))
        XCTAssertEqual(MacAnnouncement.recordDeleted.text, L10n.tr("削除しました"))
        XCTAssertEqual(MacAnnouncement.building.text, L10n.tr("作っています"))
    }

    func testAppMessagesNameTheApp() {
        for event in [MacAnnouncement.appCreated(name: "家計簿"), .appDeleted(name: "家計簿"), .appRestored(name: "家計簿")] {
            XCTAssertTrue(event.text.contains("家計簿"), "\(event)")
        }
        XCTAssertNotEqual(MacAnnouncement.appCreated(name: "A").text, MacAnnouncement.appDeleted(name: "A").text)
    }

    func testBuiltPrefersSummaryAndFallsBackToIOSCopy() {
        XCTAssertEqual(MacAnnouncement.built(summary: "記録の画面を足しました", appName: "日記", created: false).text,
                       "記録の画面を足しました")
        XCTAssertEqual(MacAnnouncement.built(summary: "  ", appName: "日記", created: true).text,
                       ErrorCopy.completedAnnouncement(appName: "日記", created: true))
        XCTAssertEqual(MacAnnouncement.built(summary: nil, appName: "日記", created: false).text,
                       ErrorCopy.completedAnnouncement(appName: "日記", created: false))
    }

    func testFailureAndMessagePassThrough() {
        XCTAssertEqual(MacAnnouncement.failure("通信できませんでした").text, "通信できませんでした")
        XCTAssertEqual(MacAnnouncement.message("通知を予約しました").text, "通知を予約しました")
    }

    func testHoldSecondsLongerWithVoiceOver() {
        MacVoiceOver.isVoiceOverRunning = { false }
        XCTAssertEqual(MacVoiceOver.holdSeconds(3), 3)
        MacVoiceOver.isVoiceOverRunning = { true }
        XCTAssertEqual(MacVoiceOver.holdSeconds(3), 15, "最低 15 秒")
        XCTAssertEqual(MacVoiceOver.holdSeconds(8), 24, "3 倍")
    }

    // MARK: - 動きを減らす

    func testReduceMotionDropsAnimation() {
        XCTAssertNil(MacMotion.animation(.default, reduceMotion: true))
        XCTAssertNotNil(MacMotion.animation(.default, reduceMotion: false))
        XCTAssertNil(MacMotion.animation(nil, reduceMotion: false), "元から無いものは無いまま")
    }

    func testWithMacAnimationFollowsSystemSetting() {
        let saved = MacMotion.systemReduceMotion
        defer { MacMotion.systemReduceMotion = saved }
        MacMotion.systemReduceMotion = { true }
        let result = withMacAnimation { 42 }
        XCTAssertEqual(result, 42, "動きを減らしても中身は実行する")
    }

    // MARK: - フォーカスの輪

    func testPaneFocusRingOnlyWithFullKeyboardAccess() {
        XCTAssertTrue(MacFocusRing.isVisible(focused: true, fullKeyboardAccess: true))
        XCTAssertFalse(MacFocusRing.isVisible(focused: true, fullKeyboardAccess: false), "クリックで入った時は出さない")
        XCTAssertFalse(MacFocusRing.isVisible(focused: false, fullKeyboardAccess: true))
    }

    // MARK: - 文字の大きさの割合

    func testZoomPercentIsHundredAtActualSizeAndIncreasing() {
        XCTAssertEqual(MacTextZoom.bodyPoints.count, MacTextZoom.steps.count)
        XCTAssertEqual(MacTextZoom.percent(at: MacTextZoom.defaultStep), 100)
        let percents = MacTextZoom.steps.indices.map(MacTextZoom.percent(at:))
        XCTAssertEqual(percents, percents.sorted())
        XCTAssertEqual(Set(percents).count, percents.count, "段ごとに違う")
        XCTAssertEqual(MacTextZoom.percent(at: 99), percents.last, "範囲外は端に寄せる")
        XCTAssertEqual(MacTextZoom.percent(at: -1), percents.first)
    }
}
