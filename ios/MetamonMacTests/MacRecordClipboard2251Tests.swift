import XCTest
import AppKit
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の記録のコピー・貼り付け(#2251): クリップボードに書く・読む・期限切れ・型の無いクリップボード、⌘V の判定、フォームへ入れる
@MainActor
final class MacRecordClipboard2251Tests: XCTestCase {
    private var board: NSPasteboard!

    override func setUp() async throws {
        board = NSPasteboard(name: NSPasteboard.Name("record-clipboard-mac-\(UUID().uuidString)"))
    }

    override func tearDown() async throws {
        board.releaseGlobally()
        board = nil
    }

    private func payload() -> RecordClipboard.Payload {
        RecordClipboard.Payload(appId: "books-app", appName: "読書記録", entityId: "books", entityName: "本",
                                fields: [.init(id: "title", name: "メモ", type: "string", value: .string("走れメロス")),
                                         .init(id: "price", name: "金額", type: "integer", value: .number(1200)),
                                         .init(id: "author", name: "著者", type: "string", value: .string("太宰治"))])
    }

    func testWriteThenRead() {
        let now = Date()
        XCTAssertTrue(RecordPasteboard.write(payload(), text: "メモ: 走れメロス", to: board, now: now))
        XCTAssertTrue(RecordPasteboard.hasRecord(on: board, now: now))
        XCTAssertEqual(RecordPasteboard.read(from: board, now: now), payload())
        XCTAssertEqual(board.string(forType: .string), "メモ: 走れメロス", "文字で貼る先向けの文字も載る")
    }

    func testExpiresAfterTenMinutes() {
        let now = Date()
        RecordPasteboard.write(payload(), text: "", to: board, now: now)
        let later = now.addingTimeInterval(RecordClipboard.expiry + 1)
        XCTAssertFalse(RecordPasteboard.hasRecord(on: board, now: later), "10 分で消えたことにする")
        XCTAssertNil(RecordPasteboard.read(from: board, now: later))
        XCTAssertNotNil(RecordPasteboard.read(from: board, now: now.addingTimeInterval(RecordClipboard.expiry - 1)))
    }

    func testBoardWithoutRecordType() {
        board.clearContents()
        board.setString("ただの文字", forType: .string)
        XCTAssertFalse(RecordPasteboard.hasRecord(on: board))
        XCTAssertNil(RecordPasteboard.read(from: board))
    }

    func testRecordWithoutExpiryIsIgnored() throws {
        // 別のアプリが同じ型だけを書いた(失効の時刻が無い)時は読まない
        board.clearContents()
        board.setData(try XCTUnwrap(RecordClipboard.encode(payload())), forType: RecordPasteboard.recordType)
        XCTAssertFalse(RecordPasteboard.hasRecord(on: board))
        XCTAssertNil(RecordPasteboard.read(from: board))
    }

    func testPasteShortcutDecision() {
        XCTAssertTrue(MacRecordPaste.handles(textInputActive: false, dialogShown: false, hasForm: true, hasRecord: true))
        XCTAssertFalse(MacRecordPaste.handles(textInputActive: true, dialogShown: false, hasForm: true, hasRecord: true), "入力欄なら普通のペースト")
        XCTAssertFalse(MacRecordPaste.handles(textInputActive: false, dialogShown: true, hasForm: true, hasRecord: true))
        XCTAssertFalse(MacRecordPaste.handles(textInputActive: false, dialogShown: false, hasForm: false, hasRecord: true))
        XCTAssertFalse(MacRecordPaste.handles(textInputActive: false, dialogShown: false, hasForm: true, hasRecord: false))
    }

    func testApplyFillsMatchingFieldsAndToasts() throws {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        let rt = MacAppRuntime(appId: "kakeibo-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("expense")))
        RecordPasteboard.apply(payload(), into: form, rt: rt)
        XCTAssertEqual(form.values["memo"], .string("走れメロス"), "名前が同じ項目に入る")
        XCTAssertEqual(form.values["amount"], .number(1200), "integer → currency")
        XCTAssertTrue(form.hasUnsavedChanges, "保存はしない(入力として残る)")
        let toast = try XCTUnwrap(rt.toast)
        XCTAssertTrue(toast.contains("2"), toast)
        XCTAssertTrue(toast.contains("著者"), "合わなかった項目も伝える: \(toast)")
    }
}
