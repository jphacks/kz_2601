import XCTest
import SwiftUI
import PDFKit
import Observation
import MetamonDSL
@testable import MetamonRenderer

@MainActor
@Observable
private final class ExportRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var store: [String: [Record]]
    var currentUserId: String?

    init(dsl: AppDSL, store: [String: [Record]], currentUid: String? = nil) {
        self.dsl = dsl
        self.store = store
        self.currentUserId = currentUid
    }

    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// `export`(#421): 表は UI と同じ書式・計算項目入り、PDF はページ分け・グラフ入り、ボタンは共有アイコンで描く。
final class ExportTests: XCTestCase {
    private static let repoRoot: URL = {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return url
    }()

    private func kakeibo() throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[
          {"id":"expense","name":"支出","titleField":"memo","fields":[
            {"id":"date","name":"日付","type":"date"},
            {"id":"amount","name":"金額","type":"currency"},
            {"id":"category","name":"分類","type":"enum","options":["食費","日用品"]},
            {"id":"payer","name":"払った人","type":"ref","refEntity":"member"},
            {"id":"memo","name":"メモ","type":"string"},
            {"id":"receipt","name":"レシート","type":"photo"},
            {"id":"tax","name":"消費税","type":"currency","computed":{"op":"mul","args":[{"ref":"amount"},0.1]}}
          ]},
          {"id":"member","name":"人","titleField":"name","fields":[{"id":"name","name":"名前","type":"string"}]}
         ],
         "screens":[{"id":"home","title":"ホーム","components":[
          {"type":"chart","entity":"expense","kind":"bar","groupBy":"category","aggregate":"sum","field":"amount","title":"分類別"},
          {"type":"button","label":"CSV で書き出す","action":{"type":"export","entity":"expense","format":"csv"}},
          {"type":"button","label":"PDF","icon":"doc","action":{"type":"export","entity":"expense","format":"pdf","title":"今月の家計簿",
            "filter":{"field":"category","op":"eq","value":"食費"}}}
         ]}]}
        """#.utf8))
    }

    private let records: [String: [Record]] = [
        "expense": [
            Record(id: "e2", data: ["date": "2026-06-02", "amount": 300, "category": "日用品", "payer": "m1", "memo": "洗剤, 詰め替え"]),
            Record(id: "e1", data: ["date": "2026-06-01", "amount": 1200, "category": "食費", "payer": "m1", "memo": "ランチ", "receipt": "data:image/jpeg;base64,AAAA"]),
        ],
        "member": [Record(id: "m1", data: ["name": "たろう"])],
    ]

    private func action(_ dsl: AppDSL, _ index: Int) throws -> Action {
        try XCTUnwrap(dsl.screen("home")?.components.filter { $0.type == .button }[index].action)
    }

    @MainActor func testTableFormatsLikeUI() throws {
        let dsl = try kakeibo()
        let rt = ExportRuntime(dsl: dsl, store: records)
        let t = try rt.exportTable(action(dsl, 0))
        XCTAssertEqual(t.title, "支出")
        XCTAssertEqual(t.headers, ["日付", "金額", "分類", "払った人", "メモ", "消費税"], "写真は除き、計算項目は入れる")
        XCTAssertEqual(t.rows.count, 2)
        let first = t.rows[0]
        XCTAssertEqual(first[0], rt.display("2026-06-01", field: dsl.entity("expense")?.field("date")), "古い順・日付は UI と同じ書式")
        XCTAssertEqual(first[1], "¥1,200")
        XCTAssertEqual(first[2], "食費")
        XCTAssertEqual(first[3], "たろう", "参照は相手の名前")
        XCTAssertEqual(first[5], "¥120", "計算項目")
        XCTAssertEqual(t.numericColumns, [false, true, false, false, false, true])
        XCTAssertTrue(t.notes.contains { $0.contains("2") }, "件数を補足に出す")
    }

    @MainActor func testFilterAndTitle() throws {
        let dsl = try kakeibo()
        let rt = ExportRuntime(dsl: dsl, store: records)
        let t = try rt.exportTable(action(dsl, 1))
        XCTAssertEqual(t.title, "今月の家計簿")
        XCTAssertEqual(t.rows.map { $0[4] }, ["ランチ"])
        XCTAssertTrue(t.notes.contains { $0.contains("分類 = 食費") }, "\(t.notes)")
    }

    @MainActor func testHiddenAnswerExcludedForOthers() throws {
        let dsl = try AppDSL.load(contentsOf: Self.repoRoot.appending(path: "backend/internal/ai/examples/quiz.json"))
        let theirs = Record(id: "q2", data: ["text": "いちばん長い川は？", "correct": "B", "points": 10], author: "other")
        let mine = Record(id: "q1", data: ["text": "富士山は？", "correct": "A", "points": 10], author: "me")
        let export = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"export","entity":"questions","format":"csv"}"#.utf8))

        let others = ExportRuntime(dsl: dsl, store: ["questions": [theirs]], currentUid: "me")
        let csv = try XCTUnwrap(String(data: CSVWriter.data(try others.exportTable(export)), encoding: .utf8))
        XCTAssertFalse(csv.contains("正解"))
        XCTAssertFalse(csv.contains(",B"))

        let author = ExportRuntime(dsl: dsl, store: ["questions": [mine]], currentUid: "me")
        XCTAssertTrue(try author.exportTable(export).headers.contains("正解"), "作者は自分の正解を書き出せる")
    }

    @MainActor func testMakeCSVFile() throws {
        let dsl = try kakeibo()
        let rt = ExportRuntime(dsl: dsl, store: records)
        let file = try rt.makeExportFile(action(dsl, 0), screen: "home")
        defer { try? FileManager.default.removeItem(at: file.url.deletingLastPathComponent()) }
        XCTAssertEqual(file.url.lastPathComponent, "支出.csv")
        let data = try Data(contentsOf: file.url)
        XCTAssertEqual(Array(data.prefix(3)), [0xEF, 0xBB, 0xBF])
        let text = try XCTUnwrap(String(data: data.dropFirst(3), encoding: .utf8))
        XCTAssertTrue(text.hasPrefix("日付,金額,分類,払った人,メモ,消費税\r\n"))
        XCTAssertTrue(text.contains("\"洗剤, 詰め替え\""))
        XCTAssertTrue(text.contains("\"¥1,200\""))
    }

    @MainActor func testMakePDFFileWithChart() throws {
        let dsl = try kakeibo()
        let rt = ExportRuntime(dsl: dsl, store: records)
        XCTAssertEqual(rt.exportChartImages(screen: "home", entity: "expense").count, 1, "同じ entity の chart を画像に")
        XCTAssertEqual(rt.exportChartImages(screen: "home", entity: "member").count, 0)
        let file = try rt.makeExportFile(action(dsl, 1), screen: "home")
        defer { try? FileManager.default.removeItem(at: file.url.deletingLastPathComponent()) }
        XCTAssertEqual(file.url.lastPathComponent, "今月の家計簿.pdf")
        let doc = try XCTUnwrap(PDFDocument(url: file.url))
        XCTAssertEqual(doc.pageCount, 1)
        let text = doc.string ?? ""
        XCTAssertTrue(text.contains("今月の家計簿"), text)
        XCTAssertTrue(text.contains("ランチ"), text)
    }

    func testPDFPaginatesLongTables() throws {
        let rows = (1...120).map { ["2026/6/\($0 % 30 + 1)", "¥\($0 * 100)", "メモ \($0)"] }
        let table = ExportTable(title: "長い表", headers: ["日付", "金額", "メモ"], rows: rows, notes: ["120 件"],
                                numericColumns: [false, true, false])
        let data = ExportPDF.data(table)
        XCTAssertGreaterThan(data.count, 1000)
        let doc = try XCTUnwrap(PDFDocument(data: data))
        XCTAssertGreaterThanOrEqual(doc.pageCount, 3, "1 ページ約 36 行なので 120 行は 3 ページ以上")
        let last = doc.page(at: doc.pageCount - 1)?.string ?? ""
        XCTAssertTrue(last.contains("日付"), "見出し行はページごとに繰り返す")
        XCTAssertTrue(last.contains("メモ 120"))
    }

    func testPDFEmptyTable() throws {
        let data = ExportPDF.data(ExportTable(title: "空", headers: ["a"], rows: []))
        let doc = try XCTUnwrap(PDFDocument(data: data))
        XCTAssertEqual(doc.pageCount, 1)
    }

    @MainActor func testUnknownFormatOrEntityThrows() throws {
        let dsl = try kakeibo()
        let rt = ExportRuntime(dsl: dsl, store: records)
        let bad = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"export","entity":"expense","format":"xlsx"}"#.utf8))
        XCTAssertThrowsError(try rt.makeExportFile(bad, screen: nil)) { XCTAssertEqual($0 as? ExportError, .unknownFormat) }
        let missing = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"export","entity":"nope","format":"csv"}"#.utf8))
        XCTAssertThrowsError(try rt.exportTable(missing)) { XCTAssertEqual($0 as? ExportError, .unknownEntity) }
    }

    // MARK: - ボタン

    @MainActor func testExportButtonRendersWithShareIcon() throws {
        let dsl = try kakeibo()
        let rt = ExportRuntime(dsl: dsl, store: records)
        let buttons = try XCTUnwrap(dsl.screen("home")?.components.filter { $0.type == .button })
        XCTAssertEqual(ButtonLeaf.defaultIcon(buttons[0].action), "square.and.arrow.up", "icon 省略の書き出しは共有の矢印")
        XCTAssertNil(ButtonLeaf.defaultIcon(Action(type: .navigate, screen: "home")))
        XCTAssertTrue(ButtonLeaf.isVisible(action: buttons[0].action, form: nil))

        let view = ButtonLeaf(c: buttons[0], ctx: RenderContext(route: Route(screen: "home")))
            .frame(width: 320)
            .environment(\.dslRuntime, rt)
        let image = try XCTUnwrap(ImageRenderer(content: view).uiImage)
        XCTAssertGreaterThan(image.size.height, 40)
    }

    /// schema の Action.type と iOS の ActionType が一致する(足し忘れると古い iOS 扱いの「未対応のアクション」になる)。
    func testEverySchemaActionTypeIsKnown() throws {
        let data = try Data(contentsOf: Self.repoRoot.appending(path: "docs/dsl/dsl.schema.json"))
        let root = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        let action = try XCTUnwrap((root["$defs"] as? [String: Any])?["Action"] as? [String: Any])
        let type = try XCTUnwrap((action["properties"] as? [String: Any])?["type"] as? [String: Any])
        for name in try XCTUnwrap(type["enum"] as? [String]) {
            XCTAssertNotEqual(ActionType(rawValue: name) ?? .unknown, .unknown, name)
        }
    }
}
