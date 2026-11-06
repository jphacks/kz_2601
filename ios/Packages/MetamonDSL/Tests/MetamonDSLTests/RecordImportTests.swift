import XCTest
@testable import MetamonDSL

/// `fileImport`: CSV の読み取り(引用符・改行・BOM・Shift_JIS)、列の当て方、型の変換、上限、export の CSV の読み戻し、本文の記録。
final class RecordImportTests: XCTestCase {
    private func entity(_ json: String) throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    private lazy var member: Entity = try! entity(#"""
    {"id":"member","name":"部員","titleField":"name","fields":[
      {"id":"name","name":"名前","type":"string","required":true},
      {"id":"grade","name":"学年","type":"integer"},
      {"id":"fee","name":"部費","type":"currency","unit":"円"},
      {"id":"height","name":"身長","type":"number","unit":"cm"},
      {"id":"active","name":"在籍","type":"boolean"},
      {"id":"part","name":"パート","type":"enum","options":["前衛","後衛"]},
      {"id":"joined","name":"入部日","type":"date"},
      {"id":"photo","name":"写真","type":"photo"},
      {"id":"label","name":"表示","type":"string","computed":{"ref":"name"}}
    ]}
    """#)

    /// 2026-11-12 12:00 JST
    private let now = Date(timeIntervalSince1970: 1_794_452_400)

    private func plan(_ csv: String, map: [ImportColumn] = [], existing: Int = 0, entity: Entity? = nil) -> ImportPlan {
        RecordImport.plan(table: CSVReader.parse(csv), entity: entity ?? member, map: map, existing: existing, now: now) { _, _ in nil }
    }

    // MARK: - CSV の読み取り

    func testParseQuotesNewlinesAndEscapes() {
        let rows = CSVReader.parse("a,\"b,c\",\"he said \"\"hi\"\"\"\r\n\"1行目\n2行目\",,x\n")
        XCTAssertEqual(rows, [["a", "b,c", "he said \"hi\""], ["1行目\n2行目", "", "x"]])
    }

    func testParseSkipsBlankLinesAndHandlesCRAndNoTrailingNewline() {
        XCTAssertEqual(CSVReader.parse("a,b\r\rc,d"), [["a", "b"], ["c", "d"]])
        XCTAssertEqual(CSVReader.parse("a,b\n\n\nc,"), [["a", "b"], ["c", ""]])
        XCTAssertEqual(CSVReader.parse(""), [])
    }

    func testDecodeUTF8WithBOMAndShiftJIS() throws {
        let bom = CSVWriter.bom + Data("名前,学年\r\n".utf8)
        XCTAssertEqual(CSVReader.decode(bom), "名前,学年\r\n")
        XCTAssertEqual(CSVReader.parse(try XCTUnwrap(CSVReader.decode(bom))), [["名前", "学年"]])
        let sjis = try XCTUnwrap("名前,学年\r\n山田,2\r\n".data(using: .shiftJIS))
        XCTAssertNil(String(data: sjis, encoding: .utf8))
        XCTAssertEqual(CSVReader.decode(sjis), "名前,学年\r\n山田,2\r\n")
        XCTAssertEqual(CSVReader.parse("\u{FEFF}a,b"), [["a", "b"]])
    }

    // MARK: - 列の当て方

    func testColumnsByNameIdAndMap() {
        let cols = RecordImport.columns(headers: ["氏名", " 学年 ", "GRADE", "fee", "写真", "表示", "メモ"], entity: member,
                                        map: [ImportColumn(column: "氏名", field: "name")])
        XCTAssertEqual(cols[0]?.id, "name")       // map
        XCTAssertEqual(cols[1]?.id, "grade")      // name(空白は無視)
        XCTAssertNil(cols[2])                     // grade は 1 列目だけ
        XCTAssertEqual(cols[3]?.id, "fee")        // id
        XCTAssertNil(cols[4])                     // 写真は入れない
        XCTAssertNil(cols[5])                     // 計算項目は入れない
        XCTAssertNil(cols[6])
    }

    func testMapToUnknownFieldIsIgnored() {
        let cols = RecordImport.columns(headers: ["名前"], entity: member, map: [ImportColumn(column: "名前", field: "zzz")])
        XCTAssertEqual(cols[0]?.id, "name")
    }

    // MARK: - 型の変換

    func testValueConversions() {
        func v(_ s: String, _ id: String) -> RecordImport.CellValue {
            RecordImport.value(s, field: member.field(id)!, now: now) { _, _ in nil }
        }
        XCTAssertEqual(v("２", "grade"), .value(.number(2)))
        XCTAssertEqual(v("2.6", "grade"), .value(.number(3)))
        XCTAssertEqual(v("¥1,200", "fee"), .value(.number(1200)))
        XCTAssertEqual(v("-¥500", "fee"), .value(.number(-500)))
        XCTAssertEqual(v("'-¥500", "fee"), .value(.number(-500)))
        XCTAssertEqual(v("1,200 円", "fee"), .value(.number(1200)))
        XCTAssertEqual(v("172.5 cm", "height"), .value(.number(172.5)))
        XCTAssertEqual(v("abc", "grade"), .invalid(.invalid))
        XCTAssertEqual(v("はい", "active"), .value(.bool(true)))
        XCTAssertEqual(v("No", "active"), .value(.bool(false)))
        XCTAssertEqual(v("たぶん", "active"), .invalid(.invalid))
        XCTAssertEqual(v("後衛", "part"), .value(.string("後衛")))
        XCTAssertEqual(v("中衛", "part"), .invalid(.invalid))
        XCTAssertEqual(v("", "grade"), .empty)
        XCTAssertEqual(v("—", "grade"), .empty)
        XCTAssertEqual(v("'=SUM(A1)", "name"), .value(.string("=SUM(A1)")))
        XCTAssertEqual(v(String(repeating: "あ", count: 4001), "name"), .invalid(.tooLong))
    }

    func testDates() throws {
        let f = member.field("joined")!
        for s in ["2026-04-08", "2026/4/8", "2026/4/8(水)", "2026年4月8日", "4/8(水)", "Wed, Apr 8", "Apr 8, 2026"] {
            XCTAssertEqual(RecordImport.value(s, field: f, now: now) { _, _ in nil }, .value(.string("2026-04-08")), s)
        }
        XCTAssertEqual(RecordImport.value("2/30", field: f, now: now) { _, _ in nil }, .invalid(.invalid))
        let dt = Field(id: "at", name: "日時", type: .datetime)
        let read = RecordImport.value("2026/4/8(水) 14:30", field: dt, now: now, refId: { _, _ in nil })
        guard case .value(.string(let iso)) = read else {
            return XCTFail("datetime not read")
        }
        let d = try XCTUnwrap(FieldCoding.date(from: .string(iso)))
        XCTAssertEqual(FieldCoding.encode(d, for: Field(id: "d", name: "d", type: .date)), "2026-04-08")
    }

    func testRefsByTitle() throws {
        let e = try entity(#"{"id":"t","name":"T","fields":[{"id":"who","name":"担当","type":"ref","refEntity":"m"},{"id":"all","name":"参加","type":"refs","refEntity":"m"}]}"#)
        let ids = ["山田": "r1", "佐藤": "r2"]
        let look: (Field, String) -> String? = { _, name in ids[name] }
        XCTAssertEqual(RecordImport.value("山田", field: e.field("who")!, refId: look), .value(.string("r1")))
        XCTAssertEqual(RecordImport.value("鈴木", field: e.field("who")!, refId: look), .invalid(.invalid))
        XCTAssertEqual(RecordImport.value("山田、佐藤", field: e.field("all")!, refId: look), .value(.array([.string("r1"), .string("r2")])))
        XCTAssertEqual(RecordImport.value("全員", field: e.field("all")!, refId: look), .empty)
    }

    // MARK: - 計画(件数・失敗・上限)

    func testPlanCountsFailuresAndFillsDefaults() {
        let p = plan("名前,学年,パート,メモ\n山田,1,前衛,x\n,2,後衛,\n佐藤,二,後衛,\n鈴木,,,\n")
        XCTAssertEqual(p.matched.map { $0.field }, ["名前", "学年", "パート"])
        XCTAssertEqual(p.unused, ["メモ"])
        XCTAssertEqual(p.rows.count, 2)
        XCTAssertEqual(p.rows[0]["name"], .string("山田"))
        XCTAssertEqual(p.rows[0]["grade"], .number(1))
        XCTAssertEqual(p.rows[1]["part"], .string("前衛"))     // 空 = 既定(最初の選択肢)
        XCTAssertEqual(p.rows[1]["active"], .bool(false))     // 既定
        XCTAssertNil(p.rows[1]["grade"])
        XCTAssertEqual(p.failures, [
            ImportRowFailure(line: 3, fieldName: "名前", kind: .required, cell: ""),
            ImportRowFailure(line: 4, fieldName: "学年", kind: .invalid, cell: "二"),
        ])
        XCTAssertEqual(p.overLimit, 0)
    }

    func testPlanLimits() {
        let body = (1...520).map { "人\($0)" }.joined(separator: "\n")
        let p = plan("名前\n" + body)
        XCTAssertEqual(p.rows.count, RecordImport.maxRows)
        XCTAssertEqual(p.overLimit, 20)
        let nearlyFull = plan("名前\n" + body, existing: 1990)
        XCTAssertEqual(nearlyFull.rows.count, 10)
        XCTAssertEqual(nearlyFull.overLimit, 510)
    }

    func testPlanWithoutMatchingHeaderAddsNothing() {
        let p = plan("foo,bar\n1,2\n")
        XCTAssertTrue(p.matched.isEmpty)
        XCTAssertTrue(p.rows.isEmpty)
        XCTAssertEqual(p.unused, ["foo", "bar"])
    }

    /// export の CSV(BOM・CRLF・表示の書式)をそのまま読み戻せる。
    func testExportedCSVReadsBack() throws {
        let records = [
            Record(id: "a", data: ["name": .string("山田, 太郎"), "grade": .number(2), "fee": .number(1200), "height": .number(172.5),
                                   "active": .bool(true), "part": .string("後衛"), "joined": .string("2026-04-08")]),
            Record(id: "b", data: ["name": .string("=危ない"), "fee": .number(-500), "active": .bool(false), "joined": .string("2025-12-01")]),
        ]
        let table = RecordExport.table(title: "部員", entity: member, records: records, fields: nil, currentUid: nil) { v, f in
            FieldCoding.display(v, field: f, locale: Locale(identifier: "ja_JP"), now: self.now)
        }
        let csv = try XCTUnwrap(CSVReader.decode(CSVWriter.data(table)))
        let p = RecordImport.plan(table: CSVReader.parse(csv), entity: member, map: [], existing: 0, now: now) { _, _ in nil }
        XCTAssertEqual(p.failures, [])
        XCTAssertEqual(p.unused, ["表示"])
        XCTAssertEqual(p.rows.count, 2)
        for (r, got) in zip(records, p.rows) {
            for (k, v) in r.data { XCTAssertEqual(got[k], v, "\(r.id).\(k)") }
        }
    }

    // MARK: - テキスト / PDF の本文

    func testBodyRecordTitleTruncationAndDefaults() throws {
        let doc = try entity(#"""
        {"id":"doc","name":"資料","titleField":"title","fields":[
          {"id":"title","name":"題名","type":"string"},
          {"id":"body","name":"本文","type":"text"},
          {"id":"short","name":"要約","type":"string"},
          {"id":"read","name":"読んだ","type":"boolean"}]}
        """#)
        let made = try XCTUnwrap(RecordImport.bodyRecord(text: "  本文\r\nです  ", fileName: "取扱説明書.pdf", entity: doc, field: "body"))
        XCTAssertEqual(made.data["body"], .string("本文\nです"))
        XCTAssertEqual(made.data["title"], .string("取扱説明書"))
        XCTAssertEqual(made.data["read"], .bool(false))
        XCTAssertFalse(made.truncated)

        let long = String(repeating: "字", count: 4100)
        let cut = try XCTUnwrap(RecordImport.bodyRecord(text: long, fileName: "a.txt", entity: doc, field: "short"))
        XCTAssertTrue(cut.truncated)
        XCTAssertEqual(cut.data["short"]?.stringValue?.unicodeScalars.count, 4000)
        let text = try XCTUnwrap(RecordImport.bodyRecord(text: long, fileName: "a.txt", entity: doc, field: "body"))
        XCTAssertFalse(text.truncated)
        XCTAssertNil(RecordImport.bodyRecord(text: "x", fileName: "a", entity: doc, field: "read"))
    }

    func testPlainTextEncodings() throws {
        XCTAssertEqual(RecordImport.plainText(Data("こんにちは".utf8)), "こんにちは")
        XCTAssertEqual(RecordImport.plainText(try XCTUnwrap("こんにちは".data(using: .shiftJIS))), "こんにちは")
        XCTAssertEqual(RecordImport.plainText(try XCTUnwrap("こんにちは".data(using: .utf16))), "こんにちは")
    }

    // MARK: - デコード

    func testComponentDecodesAcceptAndMap() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"""
        {"type":"fileImport","entity":"member","accept":["csv","pdf",3],"field":"body","map":[{"column":"氏名","field":"name"},{"column":1}]}
        """#.utf8))
        XCTAssertEqual(c.type, .fileImport)
        XCTAssertEqual(c.accept, ["csv", "pdf"])
        XCTAssertEqual(c.map, [ImportColumn(column: "氏名", field: "name")])
        XCTAssertTrue(DSLExamples.names.contains("file_import"))
    }
}
