import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// H8: AI の返事・完成の要約・失敗の状態カード・トーストを VoiceOver に読み上げる。
/// 読み上げる関数を差し替えて、呼ばれた文を確かめる。
@MainActor
final class VoiceOverAnnouncementTests: XCTestCase {
    private var posted: [String] = []
    private var savedPost: ((String) -> Void)!
    private var savedRunning: (() -> Bool)!
    private var savedLanguage: AppLanguage = .system

    override func setUp() {
        savedPost = VoiceOverAnnouncer.post
        savedRunning = VoiceOverAnnouncer.isVoiceOverRunning
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
        posted = []
        VoiceOverAnnouncer.post = { [unowned self] in posted.append($0) }
    }

    override func tearDown() {
        VoiceOverAnnouncer.post = savedPost
        VoiceOverAnnouncer.isVoiceOverRunning = savedRunning
        LanguagePreference.shared.setting = savedLanguage
    }

    private func runtime() throws -> AppRuntime {
        AppRuntime(document: try DSLExamples.document("warikan", version: 1), services: BuildConfig.make(environment: .mock))
    }

    func testToastIsAnnounced() throws {
        let rt = try runtime()
        rt.toast = "保存しました"
        XCTAssertEqual(posted, ["保存しました"])
        rt.toast = nil
        XCTAssertEqual(posted, ["保存しました"], "消した時は読まない")
    }

    func testSummaryBannerIsAnnounced() throws {
        let rt = try runtime()
        rt.summaryBanner = AppRuntime.SummaryBanner(text: "支出の一覧に合計を足しました", highlight: nil, action: .undo)
        XCTAssertEqual(posted, ["支出の一覧に合計を足しました"])
    }

    func testErrorCardIsAnnouncedOnce() throws {
        let rt = try runtime()
        let card = try XCTUnwrap(ErrorPresentation.from(.timeout, transcript: "家計簿を作って"))
        rt.errorCard = card
        rt.errorCard = card // 同じカードの置き直しでは読み直さない
        XCTAssertEqual(posted, ["時間がかかりすぎました。サーバーの応答がありませんでした。もう一度お試しください"])
        rt.errorCard = nil
        XCTAssertEqual(posted.count, 1)
    }

    func testVoiceFailureCardIsAnnounced() throws {
        let rt = try runtime()
        rt.errorCard = .voiceFailure(TranscriberFailure.tooShort(seconds: 0.2))
        XCTAssertEqual(posted, ["うまく聞き取れませんでした。タップしてから話し、話し終えたらもう一度タップしてください"])
    }

    func testBlankIsNotAnnounced() {
        VoiceOverAnnouncer.announce("  \n")
        XCTAssertTrue(posted.isEmpty)
    }

    func testCompletedAnnouncementWhenNoSummary() {
        XCTAssertEqual(ErrorCopy.completedAnnouncement(appName: "割り勘", created: true), "「割り勘」ができました")
        XCTAssertEqual(ErrorCopy.completedAnnouncement(appName: "割り勘", created: false), "変更しました")
    }

    /// AI の返事(chatRound)は吹き出しと同時に読む
    func testChatReplyIsAnnounced() async throws {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0), services: BuildConfig.make(environment: .mock))
        rt.aiConsent = AIConsentStore(defaults: UserDefaults(suiteName: "VoiceOverAnnouncementTests.\(UUID().uuidString)")!)
        rt.grantAIConsent()
        let reply = await rt.say("家計簿アプリが欲しい")
        let turn = try XCTUnwrap(reply, "AI の返事が来ない")
        XCTAssertEqual(posted.last, turn.text)
    }

    func testHoldIsLongerWithVoiceOver() {
        VoiceOverAnnouncer.isVoiceOverRunning = { false }
        XCTAssertEqual(VoiceOverAnnouncer.holdSeconds(3), 3)
        VoiceOverAnnouncer.isVoiceOverRunning = { true }
        XCTAssertGreaterThanOrEqual(VoiceOverAnnouncer.holdSeconds(3), 15)
        XCTAssertGreaterThanOrEqual(VoiceOverAnnouncer.holdSeconds(6), 18)
    }
}
