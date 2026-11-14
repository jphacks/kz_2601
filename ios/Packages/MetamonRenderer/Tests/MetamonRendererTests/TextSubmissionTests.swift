import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #627: 送った後に入力欄へ文字が残らない(送る文字の取り出し・書き戻しの判定・フォームを読む前の確定)。
final class TextSubmissionTests: XCTestCase {
    func testValueTrimsAndRejectsBlank() {
        XCTAssertEqual(TextSubmission.value("  こんにちは\n"), "こんにちは")
        XCTAssertNil(TextSubmission.value(""))
        XCTAssertNil(TextSubmission.value(" \n\t"))
    }

    func testEchoOfWholeSentTextIsCleared() {
        let t0 = Date()
        let echo = SentTextEcho(sent: "家計簿を作って", at: t0)
        XCTAssertTrue(echo.shouldClear("家計簿を作って", now: t0.addingTimeInterval(0.1)))
        XCTAssertTrue(echo.shouldClear("家計簿を作って\n", now: t0.addingTimeInterval(0.1)))
    }

    /// 変換中だった末尾(「作って」)だけが遅れて書き戻されても消す。
    func testEchoOfLateCommittedTailIsCleared() {
        let t0 = Date()
        let echo = SentTextEcho(sent: "家計簿を作って", at: t0)
        XCTAssertTrue(echo.shouldClear("作って", now: t0.addingTimeInterval(0.2)))
    }

    /// 新しく打ち始めた文字・時間が経った後は消さない。
    func testNewTypingIsKept() {
        let t0 = Date()
        let echo = SentTextEcho(sent: "家計簿を作って", at: t0)
        XCTAssertFalse(echo.shouldClear("あ", now: t0.addingTimeInterval(0.1)))
        XCTAssertFalse(echo.shouldClear("家計簿", now: t0.addingTimeInterval(0.1))) // 先頭は書き戻しではない
        XCTAssertFalse(echo.shouldClear("作って", now: t0.addingTimeInterval(SentTextEcho.window + 0.1)))
        XCTAssertFalse(echo.shouldClear("", now: t0))
    }

    func testFormActionsCommitInputFirst() {
        XCTAssertTrue(TextSubmission.commitsInput(before: .saveRecord))
        XCTAssertTrue(TextSubmission.commitsInput(before: .resetForm))
        XCTAssertFalse(TextSubmission.commitsInput(before: .navigate))
        XCTAssertFalse(TextSubmission.commitsInput(before: nil))
    }
}
