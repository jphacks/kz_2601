import XCTest
@testable import MetamonDSL

final class AggregatorTests: XCTestCase {
    private let records = [
        Record(id: "1", data: ["amount": 100, "category": "食費", "date": .string(FieldCoding.dateFormatter.string(from: .now))]),
        Record(id: "2", data: ["amount": 50, "category": "交通", "date": "2000-01-01"]),
    ]

    func testAggregate() {
        XCTAssertEqual(Aggregator.aggregate("sum", field: "amount", over: records), 150)
        XCTAssertEqual(Aggregator.aggregate("avg", field: "amount", over: records), 75)
        XCTAssertEqual(Aggregator.aggregate("max", field: "amount", over: records), 100)
        XCTAssertEqual(Aggregator.aggregate("count", field: nil, over: records), 2)
    }

    func testFilter() {
        XCTAssertEqual(Aggregator.apply(Filter(field: "date", op: "thisMonth"), entity: nil, to: records).count, 1)
        XCTAssertEqual(Aggregator.apply(Filter(field: "amount", op: "gte", value: 100), entity: nil, to: records).map(\.id), ["1"])
        XCTAssertEqual(Aggregator.apply(Filter(field: "category", op: "eq", value: "交通"), entity: nil, to: records).map(\.id), ["2"])
    }

    func testSortAndFormat() throws {
        // entity の field 型が数値なら数値比較、不明なら文字列比較(既存仕様)
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"e","name":"E","fields":[{"id":"amount","name":"a","type":"integer"}]}"#.utf8))
        XCTAssertEqual(Aggregator.sorted(records, by: Sort(field: "amount", direction: "asc"), entity: entity).map(\.id), ["2", "1"])
        XCTAssertEqual(Aggregator.sorted(records, by: Sort(field: "amount", direction: "asc"), entity: nil).map(\.id), ["1", "2"])
        XCTAssertEqual(Aggregator.format(1234, as: "currency"), "¥1,234")
        XCTAssertEqual(Aggregator.format(12.5, as: nil, unit: "kg"), "12.5 kg")
        XCTAssertEqual(Aggregator.format(40, as: "percent"), "40%")
    }

    func testFieldCodingNormalize() throws {
        let json = #"{"id":"e","name":"E","fields":[{"id":"n","name":"n","type":"integer"},{"id":"b","name":"b","type":"boolean"},{"id":"s","name":"s","type":"string"}]}"#
        let entity = try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
        XCTAssertEqual(FieldCoding.normalize("1,234", for: entity.field("n")!), 1234)
        XCTAssertNil(FieldCoding.normalize("", for: entity.field("s")!))
        XCTAssertEqual(FieldCoding.normalize(.null, for: entity.field("b")!), false)
        XCTAssertEqual(FieldCoding.display(980, field: nil), "980")
    }
}
