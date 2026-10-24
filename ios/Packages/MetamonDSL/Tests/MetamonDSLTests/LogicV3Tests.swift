import XCTest
@testable import MetamonDSL

/// #133 agg latest/earliest / #134 {"ref":"a.b"} lookup / #132 list.item.resets daily。
final class LogicV3Tests: XCTestCase {
    private var cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = .current
        return c
    }()

    private func at(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 12, _ min: Int = 0) -> Date {
        cal.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: min))!
    }

    private func entity(_ json: String) throws -> Entity { try JSONDecoder().decode(Entity.self, from: Data(json.utf8)) }
    private func expr(_ json: String) throws -> Expr { try JSONDecoder().decode(Expr.self, from: Data(json.utf8)) }

    private lazy var body: Entity = try! entity(#"""
    {"id":"body","name":"体重","fields":[{"id":"date","name":"日付","type":"date"},{"id":"weight","name":"体重","type":"number","unit":"kg"},
      {"id":"note","name":"メモ","type":"string"}]}
    """#)

    // MARK: - latest / earliest

    func testLatestPicksNewestByDate() throws {
        let rows = [
            Record(id: "a", data: ["date": "2026-10-20", "weight": 70], createdAt: "2026-10-20T08:00:00Z"),
            Record(id: "b", data: ["date": "2026-10-23", "weight": 68.5], createdAt: "2026-10-21T08:00:00Z"), // 後から過去日を入れても日付で並ぶ
            Record(id: "c", data: ["date": "2026-10-22", "weight": 69], createdAt: "2026-10-24T08:00:00Z"),
        ]
        let ev = ExprEvaluator(entities: [body], records: { _ in rows })
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"latest","entity":"body","field":"weight","by":"date"}"#)), .number(68.5))
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"earliest","entity":"body","field":"weight","by":"date"}"#)), .number(70))
        // by 省略 = 最初の日付 field
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"latest","entity":"body","field":"weight"}"#)), .number(68.5))
        // 増減 = latest − earliest
        XCTAssertEqual(ev.evaluate(try expr(#"{"op":"sub","args":[{"agg":"latest","entity":"body","field":"weight"},{"agg":"earliest","entity":"body","field":"weight"}]}"#)), .number(-1.5))
        // 日付 field の latest は日付のまま(最後に水をやった日)
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"latest","entity":"body","field":"date"}"#))?.text, "2026-10-23")
    }

    func testLatestSameDayUsesCreationOrder() throws {
        let rows = [
            Record(id: "late", data: ["date": "2026-10-24", "weight": 66], createdAt: "2026-10-24T20:00:00Z"),
            Record(id: "early", data: ["date": "2026-10-24", "weight": 67], createdAt: "2026-10-24T07:00:00Z"),
        ]
        XCTAssertEqual(Aggregator.pick("latest", by: "date", entity: body, over: rows)?.id, "late")
        XCTAssertEqual(Aggregator.pick("earliest", by: "date", entity: body, over: rows)?.id, "early")
        // createdAt も同じなら配列の後ろ(= 後から入った)を新しいとみなす
        let tie = [Record(id: "1", data: ["date": "2026-10-24"]), Record(id: "2", data: ["date": "2026-10-24"])]
        XCTAssertEqual(Aggregator.pick("latest", by: "date", entity: body, over: tie)?.id, "2")
    }

    func testLatestEmptyAndMissing() throws {
        let ev = ExprEvaluator(entities: [body], records: { _ in [] })
        XCTAssertNil(ev.evaluate(try expr(#"{"agg":"latest","entity":"body","field":"weight"}"#)), "0 件は空(「—」)")
        // 日付が空のレコードは並べられないので除く・値が空なら nil
        let rows = [Record(id: "x", data: ["weight": 50]), Record(id: "y", data: ["date": "2026-10-01"])]
        let ev2 = ExprEvaluator(entities: [body], records: { _ in rows })
        XCTAssertNil(ev2.evaluate(try expr(#"{"agg":"latest","entity":"body","field":"weight"}"#)))
        XCTAssertNil(Aggregator.pickText(try JSONDecoder().decode(Component.self, from: Data(#"{"type":"stat","entity":"body","aggregate":"latest","field":"weight"}"#.utf8)), entity: body, records: []))
    }

    func testStatPickText() throws {
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"stat","label":"今の体重","entity":"body","aggregate":"latest","field":"weight","by":"date"}"#.utf8))
        XCTAssertEqual(c.by, "date")
        let rows = [Record(id: "a", data: ["date": "2026-10-20", "weight": 70]), Record(id: "b", data: ["date": "2026-10-23", "weight": 68.5])]
        XCTAssertEqual(Aggregator.pickText(c, entity: body, records: rows), "68.5 kg")
        var note = c
        note.field = "note"
        XCTAssertNil(Aggregator.pickText(note, entity: body, records: rows), "値が空")
    }

    func testAggByRoundTrips() throws {
        let e = try expr(#"{"agg":"latest","entity":"body","field":"weight","by":"date"}"#)
        let back = try JSONDecoder().decode(Expr.self, from: JSONEncoder().encode(e))
        XCTAssertEqual(back, e)
        guard case .agg(let a) = e else { return XCTFail() }
        XCTAssertEqual(a.by, "date")
    }

    // MARK: - lookup

    func testLookupReadsReferencedRecord() throws {
        let chores = try entity(#"{"id":"chores","name":"家事","fields":[{"id":"name","name":"名前","type":"string"},{"id":"points","name":"点","type":"integer"},{"id":"double","name":"2倍","type":"integer","computed":{"op":"mul","args":[{"ref":"points"},2]}}]}"#)
        let log = try entity(#"{"id":"log","name":"記録","fields":[{"id":"chore","name":"家事","type":"ref","refEntity":"chores"},{"id":"pts","name":"点","type":"integer","computed":{"ref":"chore.points"}}]}"#)
        let data: [String: [Record]] = [
            "chores": [Record(id: "c1", data: ["name": "皿洗い", "points": 3])],
            "log": [Record(id: "l1", data: ["chore": "c1"]), Record(id: "l2", data: ["chore": "gone"]), Record(id: "l3", data: [:])],
        ]
        let ev = ExprEvaluator(entities: [chores, log], records: { data[$0] ?? [] })
        let row = { (id: String) in ExprEvaluator.Row(entity: log, record: data["log"]!.first { $0.id == id }!) }
        XCTAssertEqual(ev.evaluate(.ref("chore.points"), row: row("l1")), .number(3))
        XCTAssertEqual(ev.evaluate(.ref("chore.name"), row: row("l1")), .string("皿洗い"))
        XCTAssertEqual(ev.evaluate(.ref("chore.double"), row: row("l1")), .number(6), "参照先の computed も読める")
        XCTAssertEqual(ev.evaluate(.ref("chore.id"), row: row("l1")), .string("c1"))
        XCTAssertNil(ev.evaluate(.ref("chore.points"), row: row("l2")), "参照先が消えている = 空")
        XCTAssertNil(ev.evaluate(.ref("chore.points"), row: row("l3")), "未選択 = 空")
        XCTAssertNil(ev.evaluate(.ref("chore.ghost"), row: row("l1")), "無い field")
        XCTAssertNil(ev.evaluate(.ref("chore.points.x"), row: row("l1")), "2 段は読まない")
        XCTAssertNil(ev.evaluate(.ref("pts.points"), row: row("l1")), "ref でない field")
        // 記録の computed として合成 → 合計
        XCTAssertEqual(ev.viewRecords(for: "log").first?["pts"], .number(3))
        XCTAssertEqual(ev.evaluate(try expr(#"{"agg":"sum","entity":"log","field":"pts"}"#)), .number(3))
    }

    // MARK: - daily toggle

    func testDailyCheckDateBoundary() {
        let today = at(2026, 10, 24, 23, 59)
        XCTAssertTrue(DailyCheck.isDone("2026-10-24", today: today, calendar: cal))
        XCTAssertFalse(DailyCheck.isDone("2026-10-23", today: today, calendar: cal), "昨日チェックした = 今日は未チェック")
        XCTAssertFalse(DailyCheck.isDone("2026-10-24", today: at(2026, 10, 25, 0, 0), calendar: cal), "0:00 を過ぎたら戻る")
        XCTAssertFalse(DailyCheck.isDone(.null, today: today, calendar: cal))
        XCTAssertFalse(DailyCheck.isDone("x", today: today, calendar: cal))
        XCTAssertTrue(DailyCheck.isDone(.string(FieldCoding.datetimeFormatter.string(from: at(2026, 10, 24, 8))), today: today, calendar: cal), "RFC3339 も読む")
        XCTAssertEqual(DailyCheck.stored(true, today: today), "2026-10-24")
        XCTAssertEqual(DailyCheck.stored(false, today: today), .null)
    }

    func testDebugTodayOverride() {
        let real = at(2026, 10, 24, 21, 30)
        let d = DSLClock.debugNow(real, env: ["METAMON_DEBUG_TODAY": "2026-10-25"], calendar: cal)
        XCTAssertEqual(d.map { FieldCoding.dateFormatter.string(from: $0) }, "2026-10-25")
        XCTAssertEqual(d.map { cal.component(.hour, from: $0) }, 21, "時刻は実時刻のまま")
        XCTAssertNil(DSLClock.debugNow(real, env: [:], calendar: cal))
        XCTAssertNil(DSLClock.debugNow(real, env: ["METAMON_DEBUG_TODAY": "tomorrow"], calendar: cal))
    }

    func testMedicineExampleClearsToggleDefault() throws {
        let dsl = try ExamplesFixture.load("medicine")
        XCTAssertEqual(dsl.dailyToggleFields, ["medicine": ["taken_on"]])
        let f = try XCTUnwrap(dsl.entity("medicine")?.field("taken_on"))
        XCTAssertEqual(FieldCoding.defaultValue(for: f), .null, "追加した瞬間に「済み」にしない")
        XCTAssertNil(FieldCoding.normalize(FieldCoding.defaultValue(for: f), for: f), "送信もしない")
        // 毎日リセットでない date field の既定値(今日)はそのまま
        let shopping = try ExamplesFixture.load("shopping")
        XCTAssertNotEqual(FieldCoding.defaultValue(for: try XCTUnwrap(shopping.entity("groceries")?.field("added"))), .null)
        // 今日飲んだ数 = count filter today
        let rows = [Record(id: "1", data: ["taken_on": "2026-10-24"]), Record(id: "2", data: ["taken_on": "2026-10-23"]), Record(id: "3", data: [:])]
        let ev = ExprEvaluator(entities: dsl.entities, records: { _ in rows }, now: at(2026, 10, 24), calendar: cal)
        let stat = try XCTUnwrap(dsl.screen("home")?.allComponents.first { $0.label == "今日飲んだ(回)" }?.valueExpr)
        XCTAssertEqual(ev.evaluate(stat), .number(1))
    }
}
