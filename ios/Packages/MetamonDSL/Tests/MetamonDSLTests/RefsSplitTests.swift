import XCTest
@testable import MetamonDSL

/// #231: 割り勘の「割り勘する人」(type refs = メンバー id の配列)と精算。
/// 正典 docs/dsl/examples/warikan.json の式(per_person = amount / len(split_with)・share = sum per_person where split_with contains id)で確かめる。
final class RefsSplitTests: XCTestCase {
    private func warikan() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/warikan.json"))
    }

    private func balances(_ dsl: AppDSL, members: [Record], expenses: [Record]) -> [String: Double] {
        let records = ["members": members, "expenses": expenses]
        let ev = ExprEvaluator(dsl: dsl, records: { records[$0] ?? [] })
        var out: [String: Double] = [:]
        for r in ev.viewRecords(for: "members") { out[r["name"].stringValue ?? r.id] = r["balance"].doubleValue }
        return out
    }

    private let members = ["a": "A", "b": "B", "c": "C"].map { Record(id: $0.key, data: ["name": .string($0.value)]) }

    func testFieldTypeDecodes() throws {
        let f = try XCTUnwrap(try warikan().entity("expenses")?.field("split_with"))
        XCTAssertEqual(f.type, .refs)
        XCTAssertEqual(f.refEntity, "members")
    }

    func testSplitAmongChosenMembers() throws {
        let dsl = try warikan()
        // A が 3000 を A・B で割る / C が 900 を全員(未選択)で割る
        let expenses = [
            Record(id: "e1", data: ["title": "夕食", "amount": 3000, "payer": "a", "split_with": .array(["a", "b"])]),
            Record(id: "e2", data: ["title": "タクシー", "amount": 900, "payer": "c"]),
        ]
        let b = balances(dsl, members: members, expenses: expenses)
        // 負担: A 1500+300 / B 1500+300 / C 300 → 残高 = 払った − 負担
        XCTAssertEqual(b["A"], 1200)
        XCTAssertEqual(b["B"], -1800)
        XCTAssertEqual(b["C"], 600)
        XCTAssertEqual(b.values.reduce(0, +), 0, "精算は合計 0")
        let rows = Settlement.transfers(b.map { (name: $0.key, balance: $0.value) })
        XCTAssertEqual(rows.reduce(0) { $0 + $1.amount }, 1800)
    }

    func testEmptyArrayMeansEveryone() throws {
        let dsl = try warikan()
        let expenses = [Record(id: "e1", data: ["title": "宿", "amount": 9000, "payer": "a", "split_with": .array([])])]
        let b = balances(dsl, members: members, expenses: expenses)
        XCTAssertEqual(b["A"], 6000)
        XCTAssertEqual(b["B"], -3000)
    }

    /// 割り勘する人に入っていたメンバーを消しても、残りの人で割り直す(残り 0 人なら全員)。お金が消えない。
    func testDeletedMemberIsDroppedFromSplit() throws {
        let dsl = try warikan()
        let remaining = members.filter { $0.id != "b" }
        let expenses = [
            Record(id: "e1", data: ["title": "夕食", "amount": 3000, "payer": "a", "split_with": .array(["a", "b"])]),
            Record(id: "e2", data: ["title": "おやつ", "amount": 600, "payer": "c", "split_with": .array(["b"])]),
        ]
        let b = balances(dsl, members: remaining, expenses: expenses)
        // e1 は A だけ(3000)、e2 は B が消えて 0 人 → 全員(A・C で 300 ずつ)
        XCTAssertEqual(b["A"], 3000 - 3000 - 300)
        XCTAssertEqual(b["C"], 600 - 300)
        XCTAssertEqual(b.values.reduce(0, +), 0)
    }

    func testAggregatorContainsAndCoding() throws {
        let f = Filter(field: "split_with", op: "contains", value: "a")
        XCTAssertTrue(Aggregator.matches(f, record: Record(id: "1", data: ["split_with": .array(["a", "b"])]), field: nil))
        XCTAssertFalse(Aggregator.matches(f, record: Record(id: "2", data: ["split_with": .array(["b"])]), field: nil))
        XCTAssertTrue(Aggregator.matches(f, record: Record(id: "3", data: [:]), field: nil), "未選択 = 全員")
        let field = try XCTUnwrap(try warikan().entity("expenses")?.field("split_with"))
        XCTAssertNil(FieldCoding.normalize(.array([]), for: field), "未選択はキーごと落とす")
        XCTAssertEqual(FieldCoding.normalize(.array(["a", "", "b"]), for: field), .array(["a", "b"]))
        XCTAssertEqual(FieldCoding.defaultValue(for: field), .null)
    }
}
