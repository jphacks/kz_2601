#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import Metamon

/// オンライン AI の課金画面の文言: aiChat(task chat)は「AI と話す」、aiButton は今までのまま。
final class AITextPaywallCopyTests: XCTestCase {
    func testChatCopyTalksAboutTalking() {
        let chat = AITextPaywallCopy(chat: true)
        XCTAssertEqual(chat.feature.id, "aiChat")
        XCTAssertEqual(chat.feature.title, L10n.tr("AI と話す"))
        XCTAssertFalse(chat.closeNote.contains("ボタン"), "aiChat にボタンの言い方を出さない")
    }

    func testButtonCopyUnchanged() {
        let button = AITextPaywallCopy(chat: false)
        XCTAssertEqual(button.feature.id, "aiText")
        XCTAssertEqual(button.feature.title, L10n.tr("オンラインの AI で書く"))
        XCTAssertEqual(button.closeNote, L10n.tr("閉じると、この端末では AI で書くボタンは使えません。"))
    }

    func testChatRequestIsDetected() {
        let r = AIChat.request(persona: "p", history: [], next: "hi", long: false, locale: "ja")
        XCTAssertTrue(r.isChat)
        XCTAssertFalse(AITextRequest(instruction: "x", shape: .line).isChat)
    }
}
#endif
