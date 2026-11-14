import XCTest
@testable import MetamonDSL

/// aiButton `saveTo`: list の 1 項目を 1 件の記録にする(backend `ai_save_to.go`)。
final class AISaveToTests: XCTestCase {
    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
            {"id":"card","name":"カード","fields":[
             {"id":"q","name":"問題","type":"string","required":true},
             {"id":"learned","name":"覚えた","type":"boolean"},
             {"id":"level","name":"難しさ","type":"enum","options":["易","難"]},
             {"id":"memo","name":"メモ","type":"text","defaultValue":"AI"},
             {"id":"n","name":"点","type":"integer"}]}
            """#.utf8))
    }

    func testComponentDecodesSaveTo() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"generate","prompt":"x","shape":"list","saveTo":{"entity":"card","field":"q"}}"#.utf8))
        XCTAssertEqual(c.saveTo, AISaveTo(entity: "card", field: "q"))
        let broken = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"aiButton","task":"generate","prompt":"x","saveTo":{"entity":1}}"#.utf8))
        XCTAssertNil(broken.saveTo, "読めない saveTo は無視(表示だけ)")
    }

    func testRecordFillsFieldAndDefaults() throws {
        let e = try entity()
        let data = try XCTUnwrap(AISaveTo(entity: "card", field: "q").record("  apple の意味は? \n", in: e))
        XCTAssertEqual(data["q"], .string("apple の意味は?"))
        XCTAssertEqual(data["learned"], .bool(false))
        XCTAssertEqual(data["level"], .string("易"))
        XCTAssertEqual(data["memo"], .string("AI"))
        XCTAssertNil(data["n"])
    }

    func testRecordRejectsWrongFieldOrEmpty() throws {
        let e = try entity()
        XCTAssertNil(AISaveTo(entity: "card", field: "n").record("x", in: e), "integer には入れない")
        XCTAssertNil(AISaveTo(entity: "card", field: "zz").record("x", in: e))
        XCTAssertNil(AISaveTo(entity: "other", field: "q").record("x", in: e))
        XCTAssertNil(AISaveTo(entity: "card", field: "q").record("   ", in: e))
    }

    func testRecordTruncatesToFieldLimit() throws {
        let e = try entity()
        let long = String(repeating: "あ", count: 5000)
        let data = try XCTUnwrap(AISaveTo(entity: "card", field: "q").record(long, in: e))
        XCTAssertEqual(data["q"]?.stringValue?.unicodeScalars.count, 4000)
    }

    func testRoomStopsAtEntityLimit() {
        XCTAssertEqual(AISaveTo.room(existing: 0, wanted: 5), 5)
        XCTAssertEqual(AISaveTo.room(existing: RecordImport.maxRecordsPerEntity - 2, wanted: 5), 2)
        XCTAssertEqual(AISaveTo.room(existing: RecordImport.maxRecordsPerEntity + 3, wanted: 5), 0)
    }
}
