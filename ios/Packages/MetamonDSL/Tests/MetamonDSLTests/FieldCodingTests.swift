import XCTest
@testable import MetamonDSL

final class FieldCodingTests: XCTestCase {
    private func field(_ type: String) throws -> Field {
        try JSONDecoder().decode(Field.self, from: Data(#"{"id":"f","name":"F","type":"\#(type)"}"#.utf8))
    }

    /// #121: 未入力(.null)は `.some(.null)` ではなく nil を返し、payload からキーごと落ちること。
    func testNormalizeDropsNull() throws {
        for type in ["photo", "location", "string", "text", "enum", "date", "datetime", "integer", "currency", "number"] {
            XCTAssertNil(FieldCoding.normalize(.null, for: try field(type)), "type=\(type)")
        }
    }

    func testNormalizeDropsEmptyString() throws {
        for type in ["photo", "location", "string", "integer"] {
            XCTAssertNil(FieldCoding.normalize(.string(""), for: try field(type)), "type=\(type)")
        }
    }

    func testNormalizeKeepsValues() throws {
        XCTAssertEqual(FieldCoding.normalize(.string("1,234"), for: try field("currency")), .number(1234))
        XCTAssertEqual(FieldCoding.normalize(.string("abc"), for: try field("string")), .string("abc"))
        XCTAssertEqual(FieldCoding.normalize(.string("35.0,139.0"), for: try field("location")), .string("35.0,139.0"))
        XCTAssertEqual(FieldCoding.normalize(.null, for: try field("boolean")), .bool(false))
    }

    /// #140: 一覧の日付は生の "2026-10-24" ではなく曜日付きで。今年なら年を省く。
    func testDisplayDateJaAndEn() throws {
        let now = try XCTUnwrap(FieldCoding.dateFormatter.date(from: "2026-06-01"))
        let ja = Locale(identifier: "ja_JP"), en = Locale(identifier: "en_US")
        let d = try field("date")
        XCTAssertEqual(FieldCoding.display(.string("2026-10-24"), field: d, locale: ja, now: now), "10/24(土)")
        XCTAssertEqual(FieldCoding.display(.string("2025-10-24"), field: d, locale: ja, now: now), "2025/10/24(金)")
        XCTAssertEqual(FieldCoding.display(.string("2026-10-24"), field: d, locale: en, now: now), "Sat, Oct 24")
        XCTAssertEqual(FieldCoding.display(.string("2025-10-24"), field: d, locale: en, now: now), "Fri, Oct 24, 2025")
        // 解釈できない値はそのまま
        XCTAssertEqual(FieldCoding.display(.string("来週"), field: d, locale: ja, now: now), "来週")
        XCTAssertEqual(FieldCoding.display(.null, field: d, locale: ja, now: now), "")
    }

    func testDisplayDatetimeHasWeekdayAndTime() throws {
        let now = try XCTUnwrap(FieldCoding.dateFormatter.date(from: "2026-06-01"))
        let dt = try field("datetime")
        let s = FieldCoding.display(.string("2026-10-24T12:30:00Z"), field: dt, locale: Locale(identifier: "ja_JP"), now: now)
        XCTAssertTrue(s.hasPrefix("10/2"), s)
        XCTAssertTrue(s.contains(":30"), s)
        XCTAssertFalse(s.contains("2026"), s)
    }
}
