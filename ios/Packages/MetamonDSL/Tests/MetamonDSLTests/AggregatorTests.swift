import XCTest
@testable import MetamonDSL

final class AggregatorTests: XCTestCase {
    private let records = [
        Record(id: "1", data: ["amount": 100, "category": "食費", "date": .string(FieldCoding.dateFormatter.string(from: .now))]),
        Record(id: "2", data: ["amount": 50, "category": "交通", "date": "2000-01-01"]),
    ]

    /// #235: ref の groupBy は id で束ね、label で参照先の見出しに置き換える(未指定なら id のまま)。
    func testBucketsRelabelRefGroupBy() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"chart","entity":"book","kind":"pie","groupBy":"tag","aggregate":"count"}"#.utf8))
        let rows = [Record(id: "1", data: ["tag": "t1"]), Record(id: "2", data: ["tag": "t1"]), Record(id: "3", data: ["tag": "t2"])]
        let titles = ["t1": "小説", "t2": "技術"]
        let b = Aggregator.buckets(c, entity: nil, records: rows, label: { titles[$0] ?? $0 })
        XCTAssertEqual(b.map(\.key), ["t1", "t2"])
        XCTAssertEqual(b.map(\.label), ["小説", "技術"])
        XCTAssertEqual(b.map(\.value), [2, 1])
        XCTAssertEqual(Aggregator.buckets(c, entity: nil, records: rows).map(\.label), ["t1", "t2"])
    }

    /// chart の日付の見出し: 日本語(既定)は「3月」「3/4」、英語は「Mar」「3/4」(英語の UI でも「3月」になっていた)。
    func testDateBucketLabelsFollowLocale() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"e","name":"e","fields":[{"id":"d","name":"d","type":"date"}]}"#.utf8))
        let rows = [Record(id: "1", data: ["d": "2026-03-04"]), Record(id: "2", data: ["d": "2026-04-10"])]
        func labels(_ bucket: String, _ locale: Locale?) throws -> [String] {
            let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"chart","entity":"e","groupBy":"d","bucket":"\#(bucket)","aggregate":"count"}"#.utf8))
            return Aggregator.buckets(c, entity: entity, records: rows, locale: locale).map(\.label)
        }
        XCTAssertEqual(try labels("month", nil), ["3月", "4月"])
        XCTAssertEqual(try labels("month", Locale(identifier: "ja_JP")), ["3月", "4月"])
        XCTAssertEqual(try labels("month", Locale(identifier: "en_US")), ["Mar", "Apr"])
        XCTAssertEqual(try labels("day", Locale(identifier: "en_US")), ["3/4", "4/10"])
        XCTAssertEqual(try labels("day", nil), ["3/4", "4/10"])
        XCTAssertFalse(try labels("week", Locale(identifier: "en_US")).contains { $0.contains("週") })
    }

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
        XCTAssertEqual(FieldCoding.currency(-500), "-¥500")
        XCTAssertEqual(FieldCoding.currency(-0.2), "¥0")
        XCTAssertEqual(Aggregator.format(12.5, as: nil, unit: "kg"), "12.5 kg")
        XCTAssertEqual(Aggregator.format(0.4, as: "percent"), "40%")
    }

    /// percent は比率(0〜1)を 100 倍で出す。stat の value / aggregate どちらの経路も同じ関数を通る
    func testPercentIsRatio() {
        XCTAssertEqual(Aggregator.format(0.5, as: "percent"), "50%")
        XCTAssertEqual(Aggregator.format(1.42, as: "percent"), "142%")
        XCTAssertEqual(Aggregator.format(0, as: "percent"), "0%")
        // aggregate モード(avg of 0/1)も同じ尺度: 4 件中 1 件 → 25%
        let rs = [0.0, 1, 0, 0].enumerated().map { i, v in Record(id: "\(i)", data: ["done": .number(v)]) }
        XCTAssertEqual(Aggregator.format(Aggregator.aggregate("avg", field: "done", over: rs), as: "percent"), "25%")
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
