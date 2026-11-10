import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer
#if os(macOS)
import AppKit
#endif

@MainActor
@Observable
private final class CopyRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var store: [String: [Record]] = [:]
    var currentUserId: String?

    init(dsl: AppDSL) { self.dsl = dsl }

    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// #1332: 一覧の行の「コピー」のテキストと、クリップボードの書いて読む往復。
@MainActor
final class ListRowCopyTests: XCTestCase {
    private func runtime() throws -> CopyRuntime {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
         "entities":[{"id":"expense","name":"支出","titleField":"memo","fields":[
            {"id":"date","name":"日付","type":"date"},
            {"id":"amount","name":"金額","type":"currency"},
            {"id":"memo","name":"メモ","type":"string"},
            {"id":"receipt","name":"レシート","type":"photo"},
            {"id":"answer","name":"正解","type":"string","hidden":true}
         ]}],
         "screens":[{"id":"home","title":"ホーム","components":[]}]}
        """#.utf8))
        return CopyRuntime(dsl: dsl)
    }

    private func item(_ json: String) throws -> ListItem {
        try JSONDecoder().decode(ListItem.self, from: Data(json.utf8))
    }

    func testVisibleFieldsAsNameValueLinesInDisplayFormat() throws {
        let rt = try runtime()
        let entity = rt.dsl.entity("expense")
        let record = Record(id: "e1", data: ["date": "2026-06-01", "amount": 1200, "memo": "ランチ"])
        let text = ListRowCopyText.text(item: try item(#"{"title":"memo","subtitle":"date","trailing":"amount"}"#),
                                        record: record, entity: entity, rt: rt)
        let expected = [
            "メモ: ランチ",
            "日付: \(rt.display(record["date"], field: entity?.field("date")))",
            "金額: \(rt.display(record["amount"], field: entity?.field("amount")))",
        ].joined(separator: "\n")
        XCTAssertEqual(text, expected, "値は画面と同じ rt.display の書式")
    }

    func testSkipsEmptyDuplicateHiddenAndMedia() throws {
        let rt = try runtime()
        let entity = rt.dsl.entity("expense")
        let record = Record(id: "e1", data: ["memo": "ランチ", "receipt": "data:image/jpeg;base64,AAAA", "answer": "秘密", "date": ""])
        let skipped = ListRowCopyText.text(item: try item(#"{"title":"memo","subtitle":"receipt","trailing":"answer"}"#),
                                           record: record, entity: entity, rt: rt)
        XCTAssertEqual(skipped, "メモ: ランチ")
        let dup = ListRowCopyText.text(item: try item(#"{"title":"memo","subtitle":"memo","trailing":"date"}"#),
                                       record: record, entity: entity, rt: rt)
        XCTAssertEqual(dup, "メモ: ランチ")
    }

    func testTitleFallsBackToEntityTitleField() throws {
        let rt = try runtime()
        let text = ListRowCopyText.text(item: nil, record: Record(id: "e1", data: ["memo": "ランチ"]),
                                        entity: rt.dsl.entity("expense"), rt: rt)
        XCTAssertEqual(text, "メモ: ランチ")
    }

    func testCopyCommandTargetsFocusedRowOnly() {
        let rows = [Record(id: "a", data: [:]), Record(id: "b", data: [:])]
        XCTAssertEqual(ListRowCopyCommand.target(focusedRowId: "b", textInputActive: false, rows: rows)?.id, "b")
        XCTAssertNil(ListRowCopyCommand.target(focusedRowId: nil, textInputActive: false, rows: rows), "行を選んでいない時は何もしない")
        XCTAssertNil(ListRowCopyCommand.target(focusedRowId: "gone", textInputActive: false, rows: rows), "消えた行は対象外")
    }

    func testCopyCommandYieldsToTextInput() {
        let rows = [Record(id: "a", data: [:])]
        XCTAssertNil(ListRowCopyCommand.target(focusedRowId: "a", textInputActive: true, rows: rows),
                     "入力欄が文字を受けている時は入力欄のコピーに任せる")
    }

    /// #1334: ドラッグで渡すのはコピーと同じテキスト。空なら何も載せない。
    func testDragItemCarriesCopyText() async throws {
        let provider = ListRowCopyText.dragItem("メモ: ランチ\n金額: ¥1,200")
        XCTAssertTrue(provider.canLoadObject(ofClass: NSString.self))
        let loaded: String = try await withCheckedThrowingContinuation { cont in
            _ = provider.loadObject(ofClass: NSString.self) { obj, error in
                if let s = obj as? NSString { cont.resume(returning: s as String) }
                else { cont.resume(throwing: error ?? CocoaError(.fileReadUnknown)) }
            }
        }
        XCTAssertEqual(loaded, "メモ: ランチ\n金額: ¥1,200")
        XCTAssertTrue(ListRowCopyText.dragItem("").registeredTypeIdentifiers.isEmpty)
    }

    #if os(macOS)
    func testPasteboardRoundTripOnNamedBoard() {
        let board = NSPasteboard(name: NSPasteboard.Name("metamon.test.\(UUID().uuidString)"))
        defer { board.releaseGlobally() }
        PlatformPasteboard.setString("メモ: ランチ", on: board)
        XCTAssertEqual(PlatformPasteboard.string(on: board), "メモ: ランチ")
        PlatformPasteboard.setString("次", on: board)
        XCTAssertEqual(PlatformPasteboard.string(on: board), "次", "前の中身は置き換える")
    }
    #endif
}
