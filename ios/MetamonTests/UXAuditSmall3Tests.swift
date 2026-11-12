import SwiftUI
import XCTest
import MetamonServices
@testable import Metamon

/// UX 点検(docs/ux/ux-audit-2026-11.md)の M5(残りの質問数)・L7(端末のダークモード)・M15(言葉のぶれの残り)。
final class UXAuditSmall3Tests: XCTestCase {
    private typealias Bubble = AppRuntime.ChatBubble

    // MARK: M5 質問 n / 3

    func testQuestionNumberCountsQuestionsInCreate() {
        let bubbles = [
            Bubble(role: .user, text: "家計簿"),
            Bubble(role: .assistant, text: "何を記録しますか?", kind: .question),
            Bubble(role: .user, text: "食費"),
            Bubble(role: .assistant, text: "グラフは要りますか?", kind: .question),
        ]
        XCTAssertEqual(QuestionProgress.number(bubbles: bubbles, mode: .create, latest: .question), 2)
        XCTAssertEqual(QuestionProgress.number(bubbles: Array(bubbles.prefix(2)), mode: .create, latest: .question), 1)
    }

    func testQuestionNumberHiddenOutsideWaitingQuestion() {
        let asked = [Bubble(role: .user, text: "家計簿"), Bubble(role: .assistant, text: "何を記録しますか?", kind: .question)]
        XCTAssertNil(QuestionProgress.number(bubbles: asked, mode: .edit, latest: .question), "直す時は上限が無いので出さない")
        XCTAssertNil(QuestionProgress.number(bubbles: asked + [Bubble(role: .user, text: "食費")], mode: .create, latest: .question),
                     "答えた後(返事待ち)は出さない")
        let ready = asked + [Bubble(role: .user, text: "食費"), Bubble(role: .assistant, text: "では作りますね。", kind: .ready)]
        XCTAssertNil(QuestionProgress.number(bubbles: ready, mode: .create, latest: .ready), "案・作る前は出さない")
        XCTAssertNil(QuestionProgress.number(bubbles: [Bubble](), mode: .create, latest: nil))
    }

    func testQuestionNumberHiddenBeyondMax() {
        var bubbles: [Bubble] = []
        for i in 1...4 {
            bubbles.append(Bubble(role: .user, text: "答え\(i)"))
            bubbles.append(Bubble(role: .assistant, text: "質問\(i)", kind: .question))
        }
        XCTAssertNil(QuestionProgress.number(bubbles: bubbles, mode: .create, latest: .question), "上限を超える数は出さない(n / 3 が壊れる)")
    }

    // MARK: L7 theme.mode と端末のダークモード

    func testThemeModeDoesNotForceLight() {
        XCTAssertNil(RootView.colorScheme(themeMode: "light"), "light は固定しない(端末がダークなら暗いまま)")
        XCTAssertNil(RootView.colorScheme(themeMode: "system"))
        XCTAssertNil(RootView.colorScheme(themeMode: nil))
        XCTAssertEqual(RootView.colorScheme(themeMode: "dark"), .dark)
    }

    // MARK: M15 言葉のぶれ(生成・初期画面・版の番号を出さない)

    func testCopyAvoidsInternalWords() {
        let global = RateLimitInfo(scope: .global, tier: "signed_in", limit: 3000, resetAt: nil)
        let body = ErrorCopy.rateLimitedBody(global)
        XCTAssertFalse(body.contains("生成"), body)
    }
}
