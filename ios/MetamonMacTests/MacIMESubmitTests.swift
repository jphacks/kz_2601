import XCTest
import AppKit
@testable import MetamonRenderer
@testable import Metamon

/// 日本語の変換中(未確定)に送る・保存する(#1047)。本物の NSTextView / フィールドエディタで確かめる。
/// Return の「次へ」と Tab の順は、画面に出たウインドウとシステムのキーボード操作の設定に依るので手順書で確かめる。
@MainActor
final class MacIMESubmitTests: XCTestCase {
    private let noReplacement = NSRange(location: NSNotFound, length: 0)

    private func composing(_ committed: String, marked: String) -> NSTextView {
        let view = NSTextView()
        view.string = committed
        view.setSelectedRange(NSRange(location: (committed as NSString).length, length: 0))
        view.setMarkedText(marked, selectedRange: NSRange(location: (marked as NSString).length, length: 0),
                           replacementRange: noReplacement)
        return view
    }

    private func window(with fields: [NSTextField]) -> NSWindow {
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 300, height: 40 * CGFloat(fields.count + 1)),
                              styleMask: [.titled], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false
        for (i, field) in fields.enumerated() {
            field.frame = NSRect(x: 10, y: window.contentView!.bounds.height - 40 * CGFloat(i + 1), width: 200, height: 24)
            window.contentView!.addSubview(field)
        }
        addTeardownBlock { window.close() }
        return window
    }

    // MARK: 確定

    func testCommitKeepsMarkedTextOnce() {
        let view = composing("今日は", marked: "へんかん")
        XCTAssertTrue(view.hasMarkedText())
        XCTAssertEqual(view.commitMarkedTextNow(), "今日はへんかん")
        XCTAssertFalse(view.hasMarkedText())
        // 2 回目も同じ(二重に残らない)
        XCTAssertEqual(view.commitMarkedTextNow(), "今日はへんかん")
        XCTAssertEqual(view.string, "今日はへんかん")
    }

    func testCommitWithoutMarkedTextIsNoop() {
        let view = NSTextView()
        view.string = "確定済み"
        XCTAssertEqual(view.commitMarkedTextNow(), "確定済み")
    }

    // MARK: 会話の入力欄(送るボタン・声の文を足す)

    func testComposeDraftTakesMarkedText() {
        let view = composing("家計簿を", marked: "つくって")
        // バインディングにはまだ確定前の文字が届いていない
        XCTAssertEqual(MacComposeInput.committedDraft("家計簿を", editor: view), "家計簿をつくって")
        XCTAssertFalse(view.hasMarkedText())
    }

    func testComposeDraftWithoutEditorKeepsDraft() {
        XCTAssertEqual(MacComposeInput.committedDraft("そのまま", editor: nil), "そのまま")
        XCTAssertEqual(MacComposeInput.committedDraft("そのまま", inputFocused: false), "そのまま")
    }

    // MARK: 保存(⌘S・保存ボタン = 確定 → フォーカスを外す)

    func testFieldEditorCommitThenResignKeepsComposedText() {
        let field = NSTextField(string: "")
        let window = window(with: [field])
        XCTAssertTrue(window.makeFirstResponder(field))
        guard let editor = window.firstResponder as? NSTextView else { return XCTFail("フィールドエディタが無い") }
        editor.setMarkedText("メモ", selectedRange: NSRange(location: 2, length: 0), replacementRange: noReplacement)
        XCTAssertFalse(IMEReturnGuard.allowsReturn(editor))

        // KeyboardState.finishEditing と同じ順: 確定 → フォーカスを外す
        editor.commitMarkedTextNow()
        XCTAssertTrue(IMEReturnGuard.allowsReturn(editor))
        window.makeFirstResponder(nil)
        XCTAssertEqual(field.stringValue, "メモ")
    }
}
