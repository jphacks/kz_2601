import XCTest
@testable import MetamonDSL

/// #108 list.item.toggle / #109 thisWeek・thisYear / hoursBetween(監査 a03)。
final class LogicV2Tests: XCTestCase {
    /// テストの暦は日本(ja_JP の既定は日曜始まり = 月曜固定が効いているかを見る)。
    private var cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.locale = Locale(identifier: "ja_JP")
        c.timeZone = .current // FieldCoding.dateFormatter(端末 TZ)と揃える
        return c
    }()

    private func at(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 0, _ min: Int = 0, _ s: Int = 0) -> Date {
        cal.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: min, second: s))!
    }

    private func rec(_ id: String, _ value: String) -> Record { Record(id: id, data: ["date": .string(value)]) }
    private func day(_ d: Date) -> String { FieldCoding.dateFormatter.string(from: d) }
    private func iso(_ d: Date) -> String { FieldCoding.datetimeFormatter.string(from: d) }

    private func matching(_ op: String, now: Date, _ records: [Record]) -> [String] {
        records.filter { Aggregator.matches(Filter(field: "date", op: op), record: $0, field: nil, now: now, calendar: cal) }.map(\.id)
    }

    // MARK: - thisWeek(月曜始まり)

    func testThisWeekStartsMonday() {
        XCTAssertEqual(cal.firstWeekday, 1, "前提: ja_JP の既定は日曜始まり")
        // 2026-10-19 = 月曜、10-25 = 日曜
        let records = [
            rec("prevSun", iso(at(2026, 10, 18, 23, 59, 59))),
            rec("mon0", iso(at(2026, 10, 19, 0, 0, 0))),
            rec("monDate", "2026-10-19"),
            rec("sun", "2026-10-25"),
            rec("sunLate", iso(at(2026, 10, 25, 23, 59, 59))),
            rec("nextMon", iso(at(2026, 10, 26, 0, 0, 0))),
        ]
        // 週の途中(土曜)から見ても、日曜の夜から見ても同じ月〜日
        for now in [at(2026, 10, 24, 12), at(2026, 10, 25, 23, 30), at(2026, 10, 19, 0, 0)] {
            XCTAssertEqual(matching("thisWeek", now: now, records), ["mon0", "monDate", "sun", "sunLate"], "now=\(now)")
        }
        // 日曜(10-25)から見た「今週」に前の日曜(10-18)は入らない(日曜始まりだと逆になる)
        XCTAssertFalse(matching("thisWeek", now: at(2026, 10, 25, 9), records).contains("prevSun"))
        // 月曜 0:00 ちょうどに切り替わる
        XCTAssertEqual(matching("thisWeek", now: at(2026, 10, 26, 0, 0), records), ["nextMon"])
    }

    func testThisWeekAcrossNewYear() {
        // 2026-12-28(月)〜 2027-01-03(日)が 1 週
        let records = [rec("dec27", "2026-12-27"), rec("dec28", "2026-12-28"), rec("dec31", "2026-12-31"),
                       rec("jan1", "2027-01-01"), rec("jan3", "2027-01-03"), rec("jan4", "2027-01-04")]
        XCTAssertEqual(matching("thisWeek", now: at(2027, 1, 1, 10), records), ["dec28", "dec31", "jan1", "jan3"])
        XCTAssertEqual(matching("thisWeek", now: at(2026, 12, 28, 0), records), ["dec28", "dec31", "jan1", "jan3"])
    }

    // MARK: - thisYear

    func testThisYearBoundary() {
        let records = [rec("lastNye", iso(at(2025, 12, 31, 23, 59, 59))), rec("jan1", iso(at(2026, 1, 1, 0, 0, 0))),
                       rec("mid", "2026-06-15"), rec("nye", "2026-12-31"), rec("next", "2027-01-01")]
        XCTAssertEqual(matching("thisYear", now: at(2026, 1, 1, 0, 0), records), ["jan1", "mid", "nye"])
        XCTAssertEqual(matching("thisYear", now: at(2026, 12, 31, 23, 59), records), ["jan1", "mid", "nye"])
        XCTAssertEqual(matching("thisYear", now: at(2027, 1, 1, 0, 0), records), ["next"])
    }

    func testPeriodFiltersIgnoreNonDates() {
        let records = [Record(id: "x", data: ["date": "そのうち"]), Record(id: "y", data: [:])]
        for op in ["today", "thisWeek", "thisMonth", "thisYear"] {
            XCTAssertEqual(matching(op, now: at(2026, 10, 24), records), [], op)
        }
    }

    func testEvaluatorAggUsesSameWeek() throws {
        // agg の filter も同じ判定(ExprEvaluator は now / calendar を渡す)
        let records = ["e": [rec("1", "2026-10-19"), rec("2", "2026-10-18"), rec("3", "2026-10-25")]]
        let ev = ExprEvaluator(entities: [], records: { records[$0] ?? [] }, now: at(2026, 10, 25, 20), calendar: cal)
        let e = try JSONDecoder().decode(Expr.self, from: Data(#"{"agg":"count","entity":"e","filter":{"field":"date","op":"thisWeek","value":null}}"#.utf8))
        XCTAssertEqual(ev.evaluate(e)?.number, 2)
    }

    // MARK: - hoursBetween

    private func hours(_ a: String, _ b: String) throws -> Double? {
        let json = #"{"op":"hoursBetween","args":[\#(a),\#(b)]}"#
        let e = try JSONDecoder().decode(Expr.self, from: Data(json.utf8))
        return ExprEvaluator(entities: [], records: { _ in [] }, calendar: cal).evaluate(e)?.number
    }

    func testHoursBetween() throws {
        XCTAssertEqual(try hours(#""09:00""#, #""18:00""#), 9)
        XCTAssertEqual(try hours(#""09:15""#, #""17:45""#), 8.5)
        XCTAssertEqual(try hours(#""9:30""#, #""12:00""#), 2.5, "H:mm も可")
        XCTAssertEqual(try hours(#""10:00:00""#, #""10:30:00""#), 0.5, "HH:mm:ss も可")
        XCTAssertEqual(try hours(#""10:00""#, #""10:00""#), 0, "同じ時刻は 0(24 ではない)")
        XCTAssertEqual(try hours(#""00:00""#, #""24:00""#), 24, "24:00 は終端として可")
    }

    func testHoursBetweenOvernight() throws {
        XCTAssertEqual(try hours(#""22:00""#, #""05:00""#), 7, "日付またぎは +24h")
        XCTAssertEqual(try hours(#""23:30""#, #""00:15""#), 0.75)
        XCTAssertEqual(try hours(#""18:00""#, #""17:59""#), 23 + 59.0 / 60)
    }

    func testHoursBetweenDatetimeUsesTimeOfDay() throws {
        let start = iso(at(2026, 10, 24, 22, 0))
        let end = iso(at(2026, 10, 25, 6, 30))
        XCTAssertEqual(try hours("\"\(start)\"", "\"\(end)\""), 8.5)
    }

    func testHoursBetweenInvalidIsNil() throws {
        XCTAssertNil(try hours(#""25:00""#, #""10:00""#))
        XCTAssertNil(try hours(#""9時""#, #""10:00""#))
        XCTAssertNil(try hours(#""10:60""#, #""11:00""#))
        XCTAssertNil(try hours(#""2026-10-24""#, #""10:00""#), "日付だけでは時刻が無い")
        XCTAssertNil(try hours("9", #""10:00""#), "数値は受けない")
    }

    func testShiftPayComputed() throws {
        // a03: 時給 × (終了 − 開始)
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"shift","name":"シフト","fields":[
          {"id":"start","name":"開始","type":"string"},{"id":"end","name":"終了","type":"string"},
          {"id":"wage","name":"時給","type":"currency"},
          {"id":"hours","name":"時間","type":"number","computed":{"op":"hoursBetween","args":[{"ref":"start"},{"ref":"end"}]}},
          {"id":"pay","name":"給料","type":"currency","computed":{"op":"round","args":[{"op":"mul","args":[{"ref":"hours"},{"ref":"wage"}]}]}}]}
        """#.utf8))
        let rows = [Record(id: "1", data: ["start": "17:00", "end": "22:30", "wage": 1100]),
                    Record(id: "2", data: ["start": "22:00", "end": "05:00", "wage": 1375])]
        let ev = ExprEvaluator(entities: [entity], records: { _ in rows }, calendar: cal)
        let view = ev.viewRecords(for: "shift")
        XCTAssertEqual(view.map { $0["hours"].doubleValue }, [5.5, 7])
        XCTAssertEqual(view.map { $0["pay"].doubleValue }, [6050, 9625])
    }

    // MARK: - schema と実装の一致

    private func schemaDefs() throws -> [String: Any] {
        let data = try Data(contentsOf: ExamplesFixture.schemaURL)
        let root = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        return try XCTUnwrap(root["$defs"] as? [String: Any])
    }

    private func enumOf(_ defs: [String: Any], _ def: String, _ prop: String) throws -> Set<String> {
        let d = try XCTUnwrap(defs[def] as? [String: Any])
        let p = try XCTUnwrap((d["properties"] as? [String: Any])?[prop] as? [String: Any])
        return Set(try XCTUnwrap(p["enum"] as? [String]))
    }

    func testSchemaOpsMatchEvaluator() throws {
        let defs = try schemaDefs()
        XCTAssertEqual(try enumOf(defs, "ExprOp", "op"), ExprEvaluator.supportedOps, "schema の ExprOp.op と iOS の評価器がずれている")
        XCTAssertEqual(try enumOf(defs, "Filter", "op"), Aggregator.filterOps, "schema の Filter.op と Aggregator がずれている")
    }

    // MARK: - list.item.toggle

    func testShoppingExampleDecodesToggle() throws {
        let dsl = try ExamplesFixture.load("shopping")
        let lists = dsl.screen("home")?.components.flatMap { $0.children ?? [] }.filter { $0.type == .list } ?? []
        XCTAssertEqual(lists.count, 2)
        XCTAssertEqual(lists.map { $0.item?.toggle }, ["done", "done"])
        XCTAssertEqual(dsl.entity("groceries")?.field("done")?.type, .boolean)
    }
}
