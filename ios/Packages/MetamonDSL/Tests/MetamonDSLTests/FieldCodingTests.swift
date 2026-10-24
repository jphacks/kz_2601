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
}
