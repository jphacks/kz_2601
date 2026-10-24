import XCTest
@testable import MetamonDSL

/// #172 毎日チェックの履歴(toggle field + "__dates")と streak / doneInDays。
final class DailyHistoryTests: XCTestCase {
    private let cal = Calendar.current

    private func at(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 12, _ min: Int = 0) -> Date {
        cal.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: min))!
    }

    private func expr(_ json: String) throws -> Expr { try JSONDecoder().decode(Expr.self, from: Data(json.utf8)) }

    private lazy var medicine: Entity = try! JSONDecoder().decode(Entity.self, from: Data(#"""
    {"id":"medicine","name":"薬","fields":[{"id":"name","name":"名前","type":"string"},{"id":"taken_on","name":"飲んだ日","type":"date"},
      {"id":"streak","name":"連続","type":"integer","computed":{"op":"streak","args":[{"ref":"taken_on"}]}},
      {"id":"last7","name":"7日","type":"integer","computed":{"op":"doneInDays","args":[{"ref":"taken_on"},7]}}]}
    """#.utf8))

    private func rec(_ id: String, last: String?, history: [String]?) -> Record {
        var data: [String: JSONValue] = ["name": .string(id)]
        if let last { data["taken_on"] = .string(last) }
        if let history { data["taken_on__dates"] = .array(history.map { .string($0) }) }
        return Record(id: id, data: data)
    }

    private func day(_ d: Date) -> String { FieldCoding.dateFormatter.string(from: d) }

    // MARK: - patch(押す・外す)

    func testCheckAppendsTodayAndKeepsLastDoneDate() {
        let r = rec("a", last: "2026-10-23", history: ["2026-10-22", "2026-10-23"])
        let p = DailyCheck.patch(r, field: "taken_on", on: true, today: at(2026, 10, 24))
        XCTAssertEqual(p["taken_on"], .string("2026-10-24"))
        XCTAssertEqual(p["taken_on__dates"], .array(["2026-10-22", "2026-10-23", "2026-10-24"]))
    }

    func testUncheckRemovesOnlyTodayAndRollsBackLastDone() {
        let r = rec("a", last: "2026-10-24", history: ["2026-10-23", "2026-10-24"])
        let p = DailyCheck.patch(r, field: "taken_on", on: false, today: at(2026, 10, 24))
        XCTAssertEqual(p["taken_on"], .string("2026-10-23"), "最後にやった日は残った最新日(今日ではない = 未チェック表示)")
        XCTAssertEqual(p["taken_on__dates"], .array(["2026-10-23"]))
        XCTAssertFalse(DailyCheck.isDone(p["taken_on"]!, today: at(2026, 10, 24)))
        // 最後の 1 日を外すと両方 null(キーごと消える)
        let only = rec("b", last: "2026-10-24", history: ["2026-10-24"])
        let q = DailyCheck.patch(only, field: "taken_on", on: false, today: at(2026, 10, 24))
        XCTAssertEqual(q["taken_on"], .null)
        XCTAssertEqual(q["taken_on__dates"], .null)
    }

    func testLegacyRecordWithoutHistorySeedsFromLastDoneDate() {
        // #132 の旧レコード(lastDoneDate だけ)
        let r = rec("a", last: "2026-10-23", history: nil)
        XCTAssertEqual(DailyCheck.dates(r, field: "taken_on"), ["2026-10-23"])
        let p = DailyCheck.patch(r, field: "taken_on", on: true, today: at(2026, 10, 24))
        XCTAssertEqual(p["taken_on__dates"], .array(["2026-10-23", "2026-10-24"]))
    }

    func testCheckTwiceIsIdempotentAndHistoryIsCapped() {
        let r = rec("a", last: "2026-10-24", history: ["2026-10-24"])
        let p = DailyCheck.patch(r, field: "taken_on", on: true, today: at(2026, 10, 24, 23, 59))
        XCTAssertEqual(p["taken_on__dates"], .array(["2026-10-24"]))
        let many = (1...500).map { day(cal.date(byAdding: .day, value: -$0, to: at(2026, 10, 24))!) }
        let big = rec("b", last: nil, history: many)
        guard case .array(let xs)? = DailyCheck.patch(big, field: "taken_on", on: true, today: at(2026, 10, 24))["taken_on__dates"] else {
            return XCTFail("history missing")
        }
        XCTAssertEqual(xs.count, DailyCheck.maxHistory)
        XCTAssertEqual(xs.last, .string("2026-10-24"), "古い方から捨てる")
    }

    // MARK: - streak / count

    func testStreakCountsFromTodayOrYesterday() {
        let days = ["2026-10-20", "2026-10-22", "2026-10-23", "2026-10-24"]
        XCTAssertEqual(DailyCheck.streak(days, today: at(2026, 10, 24)), 3)
        // 今日まだ: 昨日までの連続を切らない
        XCTAssertEqual(DailyCheck.streak(Array(days.dropLast()), today: at(2026, 10, 24)), 2)
        // 昨日も抜けたら 0
        XCTAssertEqual(DailyCheck.streak(["2026-10-22"], today: at(2026, 10, 24)), 0)
        XCTAssertEqual(DailyCheck.streak([], today: at(2026, 10, 24)), 0)
    }

    func testStreakAcrossMonthBoundaryAndMidnight() {
        let days = ["2026-10-30", "2026-10-31", "2026-11-01"]
        XCTAssertEqual(DailyCheck.streak(days, today: at(2026, 11, 1, 0, 0)), 3, "0:00 ちょうどでも今日扱い")
        XCTAssertEqual(DailyCheck.streak(days, today: at(2026, 10, 31, 23, 59)), 2)
        XCTAssertEqual(DailyCheck.streak(days, today: at(2026, 11, 2, 0, 1)), 3, "日付が変わった直後(今日未チェック)も昨日までの 3")
        XCTAssertEqual(DailyCheck.streak(days, today: at(2026, 11, 3)), 0)
    }

    func testCountWithinLastDays() {
        let days = ["2026-10-17", "2026-10-18", "2026-10-20", "2026-10-24", "2026-10-25"]
        XCTAssertEqual(DailyCheck.count(days, lastDays: 7, today: at(2026, 10, 24)), 3, "10/18〜10/24。未来日(10/25)は数えない")
        XCTAssertEqual(DailyCheck.count(days, lastDays: 1, today: at(2026, 10, 24)), 1)
        XCTAssertEqual(DailyCheck.count(days, lastDays: 0, today: at(2026, 10, 24)), 0)
    }

    // MARK: - Expr

    func testExprStreakAndDoneInDaysOnRowAndAggregate() throws {
        let rows = [
            rec("a", last: "2026-10-24", history: ["2026-10-21", "2026-10-22", "2026-10-23", "2026-10-24"]),
            rec("b", last: "2026-10-23", history: nil), // 旧形式・今日まだ
            rec("c", last: nil, history: nil),
        ]
        let entity = medicine
        let ev = ExprEvaluator(entities: [entity], records: { _ in rows }, now: at(2026, 10, 24, 9))
        let row = { (i: Int) in ExprEvaluator.Row(entity: entity, record: rows[i]) }
        XCTAssertEqual(ev.fieldValue("streak", row: row(0)), .number(4))
        XCTAssertEqual(ev.fieldValue("streak", row: row(1)), .number(1))
        XCTAssertEqual(ev.fieldValue("streak", row: row(2)), .number(0))
        XCTAssertEqual(ev.fieldValue("last7", row: row(0)), .number(4))
        // 全体: computed の合計 / 最長
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"sum","entity":"medicine","field":"last7"}"#)), .number(5))
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"max","entity":"medicine","field":"streak"}"#)), .number(4))
        // 第 1 引数が ref でない / 行が無い(stat.value)/ 日数が無いは評価できない
        XCTAssertNil(ev.evaluate(try expr(#"{"op":"streak","args":["2026-10-24"]}"#), row: row(0)))
        XCTAssertNil(ev.evaluate(try expr(#"{"op":"streak","args":[{"ref":"taken_on"}]}"#)))
        XCTAssertNil(ev.evaluate(try expr(#"{"op":"doneInDays","args":[{"ref":"taken_on"}]}"#), row: row(0)))
    }

    func testUncheckThenExprDropsToday() throws {
        let r = rec("a", last: "2026-10-24", history: ["2026-10-23", "2026-10-24"])
        var after = r
        for (k, v) in DailyCheck.patch(r, field: "taken_on", on: false, today: at(2026, 10, 24)) {
            after.data[k] = v == .null ? nil : v
        }
        let entity = medicine
        let ev = ExprEvaluator(entities: [entity], records: { _ in [after] }, now: at(2026, 10, 24))
        let row = ExprEvaluator.Row(entity: entity, record: after)
        XCTAssertEqual(ev.fieldValue("streak", row: row), .number(1), "今日を外すと昨日までの 1")
        XCTAssertEqual(ev.fieldValue("last7", row: row), .number(1))
    }
}
