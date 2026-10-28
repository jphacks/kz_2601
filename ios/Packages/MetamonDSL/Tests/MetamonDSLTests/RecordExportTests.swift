import XCTest
@testable import MetamonDSL

/// `export`(#421): CSV のエスケープ・BOM、列の選び方(写真・hidden)、行の並び、ファイル名、Action のデコード。
final class RecordExportTests: XCTestCase {
    private func entity(_ json: String) throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    private lazy var quiz: Entity = try! entity(#"""
    {"id":"q","name":"問題","fields":[
      {"id":"text","name":"問題文","type":"string"},
      {"id":"correct","name":"正解","type":"enum","options":["A","B"],"hidden":true},
      {"id":"photo","name":"写真","type":"photo"},
      {"id":"date","name":"日付","type":"date"},
      {"id":"points","name":"配点","type":"integer"},
      {"id":"double","name":"倍","type":"integer","computed":{"op":"mul","args":[{"ref":"points"},2]}}
    ]}
    """#)

    // MARK: - CSV

    func testEscapePlainCellsUntouched() {
        XCTAssertEqual(CSVWriter.escape("ラーメン"), "ラーメン")
        XCTAssertEqual(CSVWriter.escape(""), "")
        XCTAssertEqual(CSVWriter.escape("¥1200"), "¥1200")
    }

    func testEscapeCommasQuotesNewlines() {
        XCTAssertEqual(CSVWriter.escape("¥1,200"), "\"¥1,200\"")
        XCTAssertEqual(CSVWriter.escape("彼は\"はい\"と言った"), "\"彼は\"\"はい\"\"と言った\"")
        XCTAssertEqual(CSVWriter.escape("1行目\n2行目"), "\"1行目\n2行目\"")
        XCTAssertEqual(CSVWriter.escape("a\r\nb"), "\"a\r\nb\"")
    }

    func testEscapeFormulaInjection() {
        XCTAssertEqual(CSVWriter.escape("=SUM(A1:A9)"), "'=SUM(A1:A9)")
        XCTAssertEqual(CSVWriter.escape("+81 90"), "'+81 90")
        XCTAssertEqual(CSVWriter.escape("@cmd"), "'@cmd")
        XCTAssertEqual(CSVWriter.escape("-cmd"), "'-cmd")
        XCTAssertEqual(CSVWriter.escape("=1,2"), "\"'=1,2\"", "式除けの後でも区切りを含めば囲む")
        // 負の数・負の金額はそのまま(Excel で数値として読める)
        XCTAssertEqual(CSVWriter.escape("-500"), "-500")
        XCTAssertEqual(CSVWriter.escape("-¥500"), "-¥500")
        XCTAssertEqual(CSVWriter.escape("-"), "-")
    }

    func testCSVHasBOMHeaderAndCRLF() throws {
        let table = ExportTable(title: "家計簿", headers: ["日付", "金額", "メモ"],
                                rows: [["6/1", "¥1,200", "ランチ, 同僚と"], ["6/2", "¥300", "コーヒー"]])
        let data = CSVWriter.data(table)
        XCTAssertEqual(Array(data.prefix(3)), [0xEF, 0xBB, 0xBF], "Excel が UTF-8 と分かるよう BOM を付ける")
        let text = try XCTUnwrap(String(data: data.dropFirst(3), encoding: .utf8))
        XCTAssertEqual(text, "日付,金額,メモ\r\n6/1,\"¥1,200\",\"ランチ, 同僚と\"\r\n6/2,¥300,コーヒー\r\n")
        XCTAssertFalse(text.hasPrefix("\u{FEFF}"), "BOM は 1 回だけ")
    }

    func testEmptyTableIsHeaderOnly() {
        let t = ExportTable(title: "x", headers: ["a", "b"], rows: [])
        XCTAssertEqual(CSVWriter.string(t), "a,b\r\n")
    }

    // MARK: - 列と行

    func testDefaultColumnsIncludeComputedSkipMediaAndHidden() {
        let cols = RecordExport.columns(entity: quiz, fields: nil, includeHidden: false).map(\.id)
        XCTAssertEqual(cols, ["text", "date", "points", "double"])
        let author = RecordExport.columns(entity: quiz, fields: nil, includeHidden: true).map(\.id)
        XCTAssertEqual(author, ["text", "correct", "date", "points", "double"])
    }

    func testExplicitFieldsKeepOrderAndDropUnknown() {
        let cols = RecordExport.columns(entity: quiz, fields: ["points", "nope", "photo", "text"], includeHidden: false)
        XCTAssertEqual(cols.map(\.id), ["points", "text"])
    }

    func testHiddenValuesOnlyForAuthorRows() {
        let mine = Record(id: "1", data: ["text": "富士山は？", "correct": "A", "points": 10, "double": 20], author: "me")
        let theirs = Record(id: "2", data: ["text": "琵琶湖は？", "correct": "B", "points": 5, "double": 10], author: "other")
        let display: (JSONValue, Field) -> String = { v, _ in v.displayString }

        let others = RecordExport.table(title: "t", entity: quiz, records: [theirs], fields: nil, currentUid: "me", display: display)
        XCTAssertFalse(others.headers.contains("正解"), "他人の行しか無ければ正解の列ごと出さない")
        XCTAssertFalse(others.rows.flatMap { $0 }.contains("B"))

        let mixed = RecordExport.table(title: "t", entity: quiz, records: [mine, theirs], fields: nil, currentUid: "me", display: display)
        let col = try! XCTUnwrap(mixed.headers.firstIndex(of: "正解"))
        XCTAssertEqual(mixed.rows[0][col], "A", "自分が書いた行は正解を出す")
        XCTAssertEqual(mixed.rows[1][col], "", "他人の行は空欄")
        XCTAssertEqual(mixed.numericColumns, [false, false, false, true, true])

        let unknownUser = RecordExport.table(title: "t", entity: quiz, records: [mine], fields: nil, currentUid: nil, display: display)
        XCTAssertFalse(unknownUser.headers.contains("正解"), "自分が分からなければ出さない")
    }

    func testMissingStoredValueIsBlank() {
        let r = Record(id: "1", data: ["text": "x"])
        let t = RecordExport.table(title: "t", entity: quiz, records: [r], fields: ["text", "points"], currentUid: nil) { _, _ in "NG" }
        XCTAssertEqual(t.rows, [["NG", ""]], "値の無い保存項目は display を通さず空欄")
    }

    func testChronologicalByFirstDateField() {
        let rs = [
            Record(id: "b", data: ["date": "2026-06-02"]),
            Record(id: "none", data: [:]),
            Record(id: "a", data: ["date": "2026-06-01"]),
            Record(id: "c", data: ["date": "2026-06-02"]),
        ]
        XCTAssertEqual(RecordExport.chronological(rs, entity: quiz).map(\.id), ["a", "b", "c", "none"])
    }

    func testFileName() {
        XCTAssertEqual(RecordExport.fileName("6 月の家計簿", ext: "csv"), "6 月の家計簿.csv")
        XCTAssertEqual(RecordExport.fileName("a/b:c", ext: "pdf"), "a_b_c.pdf")
        XCTAssertEqual(RecordExport.fileName("../x", ext: "csv"), "_x.csv")
        XCTAssertEqual(RecordExport.fileName("  ", ext: "csv"), "書き出し.csv")
    }

    // MARK: - Action

    func testActionDecodes() throws {
        let a = try JSONDecoder().decode(Action.self, from: Data(#"""
        {"type":"export","entity":"expense","format":"pdf","title":"今月の家計簿",
         "filter":{"field":"date","op":"thisMonth","value":null},"fields":["date","amount"]}
        """#.utf8))
        XCTAssertEqual(a.type, .export)
        XCTAssertEqual(a.format, "pdf")
        XCTAssertEqual(a.title, "今月の家計簿")
        XCTAssertEqual(a.filters.map(\.op), ["thisMonth"])
        XCTAssertEqual(a.fields, ["date", "amount"])

        let many = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"export","entity":"e","format":"csv","filter":[{"field":"a","op":"eq","value":1},{"field":"b","op":"thisYear","value":null}]}"#.utf8))
        XCTAssertEqual(many.filters.count, 2)
        XCTAssertNil(many.fields)
    }
}
