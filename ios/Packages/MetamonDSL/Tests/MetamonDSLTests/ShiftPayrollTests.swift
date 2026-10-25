import XCTest
@testable import MetamonDSL

/// #229: シフトの勤務時間(休憩・夜勤)・今月の合計・給料。実 API の生成(ai-eval shift_break)と同じ形の entity で確かめる。
final class ShiftPayrollTests: XCTestCase {
    private var cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = .current // FieldCoding.dateFormatter(端末 TZ)と揃える
        return c
    }()

    private func at(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 0, _ min: Int = 0) -> Date {
        cal.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: min))!
    }

    private func entity(_ json: String) throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    /// 生成された DSL(shift_break)の shift entity: date + 開始/終了 "HH:mm" + 時給 + 休憩(分)。
    /// 休憩はレシピどおり coalesce(break_min, 0)(後から足した休憩欄が空の古い記録も計算できる)。
    private func shiftWithBreak() throws -> Entity {
        try entity(#"""
        {"id":"shift","name":"シフト","fields":[
          {"id":"date","name":"日付","type":"date","defaultValue":"now","required":true},
          {"id":"start","name":"開始時刻","type":"string","defaultValue":"09:00","required":true},
          {"id":"end","name":"終了時刻","type":"string","defaultValue":"18:00","required":true},
          {"id":"wage","name":"時給","type":"currency","unit":"円","defaultValue":1100,"required":true},
          {"id":"break_min","name":"休憩時間","type":"integer","unit":"分","defaultValue":60},
          {"id":"hours","name":"勤務時間","type":"number","unit":"時間","computed":{"op":"max","args":[{"op":"sub","args":[{"op":"hoursBetween","args":[{"ref":"start"},{"ref":"end"}]},{"op":"div","args":[{"op":"coalesce","args":[{"ref":"break_min"},0]},60]}]},0]}},
          {"id":"pay","name":"給料","type":"currency","unit":"円","computed":{"op":"round","args":[{"op":"mul","args":[{"ref":"hours"},{"ref":"wage"}]}]}}]}
        """#)
    }

    private func r(_ id: String, _ date: String, _ start: String, _ end: String, brk: Double? = nil, wage: Double = 1100) -> Record {
        var d: [String: JSONValue] = ["date": .string(date), "start": .string(start), "end": .string(end), "wage": .number(wage)]
        if let brk { d["break_min"] = .number(brk) }
        return Record(id: id, data: d)
    }

    private func decode(_ json: String) throws -> Expr { try JSONDecoder().decode(Expr.self, from: Data(json.utf8)) }

    func testPerShiftHoursWithBreakAndOvernight() throws {
        let rows = [
            r("day", "2026-10-03", "09:00", "18:00", brk: 60),     // 9h − 1h = 8
            r("quarter", "2026-10-04", "17:00", "22:00", brk: 45),  // 5h − 0.75h = 4.25
            r("night", "2026-10-10", "22:00", "06:00", brk: 60),    // 日付またぎ 8h − 1h = 7
            r("nobreak", "2026-10-11", "10:00", "13:30"),           // 休憩未入力(休憩欄を足す前の記録)= 引かない(3.5)
            r("short", "2026-10-12", "10:00", "10:30", brk: 60),    // 休憩が勤務より長い → 0(負にしない)
        ]
        let ev = ExprEvaluator(entities: [try shiftWithBreak()], records: { _ in rows }, now: at(2026, 10, 25, 12), calendar: cal)
        let view = ev.viewRecords(for: "shift")
        XCTAssertEqual(view.map { $0["hours"].doubleValue }, [8, 4.25, 7, 3.5, 0])
        XCTAssertEqual(view.map { $0["pay"].doubleValue }, [8800, 4675, 7700, 3850, 0])
    }

    func testMonthlyTotalsFilterThisMonth() throws {
        let rows = [
            r("a", "2026-10-03", "09:00", "18:00", brk: 60),  // 8
            r("b", "2026-10-31", "22:00", "05:30", brk: 30),  // 月末の夜勤 7.5 − 0.5 = 7(日付の field が 10 月なので今月)
            r("c", "2026-09-30", "09:00", "17:00", brk: 0),   // 先月 → 入らない
            r("d", "2026-11-01", "09:00", "12:00", brk: 0),   // 来月 → 入らない
        ]
        let ev = ExprEvaluator(entities: [try shiftWithBreak()], records: { _ in rows }, now: at(2026, 10, 25, 12), calendar: cal)
        let hours = try decode(#"{"agg":"sum","entity":"shift","field":"hours","filter":{"field":"date","op":"thisMonth","value":null}}"#)
        let pay = try decode(#"{"agg":"sum","entity":"shift","field":"pay","filter":{"field":"date","op":"thisMonth","value":null}}"#)
        XCTAssertEqual(ev.evaluate(hours)?.number, 15)
        XCTAssertEqual(ev.evaluate(pay)?.number, 16500)
        // stat(aggregate モード)の経路: 合成済みレコード → filter → Aggregator.aggregate
        let month = Aggregator.weekCalendar(cal)
        let shown = ev.viewRecords(for: "shift").filter {
            Aggregator.matches(Filter(field: "date", op: "thisMonth"), record: $0, field: try? shiftWithBreak().field("date"), now: at(2026, 10, 25, 12), calendar: month)
        }
        XCTAssertEqual(Aggregator.aggregate("sum", field: "hours", over: shown), 15)
        XCTAssertEqual(Aggregator.aggregate("sum", field: "pay", over: shown), 16500)
    }

    func testBreakWithoutCoalesceLeavesOldRecordsBlank() throws {
        // coalesce を付けないと、休憩欄を足す前の記録は sub が nil → 勤務時間・給料が「—」(プロンプトで coalesce を指示する理由)
        let e = try entity(#"""
        {"id":"shift","name":"シフト","fields":[
          {"id":"start","name":"開始","type":"string"},{"id":"end","name":"終了","type":"string"},
          {"id":"break_min","name":"休憩","type":"integer","unit":"分"},
          {"id":"hours","name":"勤務時間","type":"number","computed":{"op":"sub","args":[{"op":"hoursBetween","args":[{"ref":"start"},{"ref":"end"}]},{"op":"div","args":[{"ref":"break_min"},60]}]}}]}
        """#)
        let rows = [Record(id: "old", data: ["start": "09:00", "end": "12:00"])]
        XCTAssertNil(ExprEvaluator(entities: [e], records: { _ in rows }, calendar: cal).viewRecords(for: "shift")[0]["hours"].doubleValue)
    }

    func testPayFromConstantWage() throws {
        // 時給が定数(式の中の 1100)
        let e = try entity(#"""
        {"id":"shift","name":"シフト","fields":[
          {"id":"start","name":"出勤","type":"string"},{"id":"end","name":"退勤","type":"string"},
          {"id":"hours","name":"勤務時間","type":"number","unit":"時間","computed":{"op":"hoursBetween","args":[{"ref":"start"},{"ref":"end"}]}},
          {"id":"pay","name":"給料","type":"currency","computed":{"op":"round","args":[{"op":"mul","args":[{"ref":"hours"},1100]}]}}]}
        """#)
        let rows = [Record(id: "1", data: ["start": "18:15", "end": "23:00"])]
        let view = ExprEvaluator(entities: [e], records: { _ in rows }, calendar: cal).viewRecords(for: "shift")
        XCTAssertEqual(view[0]["hours"].doubleValue, 4.75)
        XCTAssertEqual(view[0]["pay"].doubleValue, 5225)
    }

    func testDatetimeFieldsAndBreakAsTimes() throws {
        // datetime の出勤・退勤(datePicker includeTime / timePicker on datetime)+ 休憩を開始・終了の時刻で持つ形
        let e = try entity(#"""
        {"id":"shift","name":"シフト","fields":[
          {"id":"clock_in","name":"出勤","type":"datetime"},{"id":"clock_out","name":"退勤","type":"datetime"},
          {"id":"break_start","name":"休憩開始","type":"string"},{"id":"break_end","name":"休憩終了","type":"string"},
          {"id":"hours","name":"勤務時間","type":"number","unit":"時間","computed":{"op":"sub","args":[
            {"op":"hoursBetween","args":[{"ref":"clock_in"},{"ref":"clock_out"}]},
            {"op":"coalesce","args":[{"op":"hoursBetween","args":[{"ref":"break_start"},{"ref":"break_end"}]},0]}]}}]}
        """#)
        let iso = { (d: Date) in JSONValue.string(FieldCoding.datetimeFormatter.string(from: d)) }
        let rows = [
            Record(id: "1", data: ["clock_in": iso(at(2026, 10, 24, 22, 0)), "clock_out": iso(at(2026, 10, 25, 7, 0)),
                                   "break_start": "02:00", "break_end": "03:00"]),
            Record(id: "2", data: ["clock_in": iso(at(2026, 10, 24, 9, 0)), "clock_out": iso(at(2026, 10, 24, 17, 30))]),
        ]
        let view = ExprEvaluator(entities: [e], records: { _ in rows }, calendar: cal).viewRecords(for: "shift")
        XCTAssertEqual(view.map { $0["hours"].doubleValue }, [8, 8.5], "夜勤 9h − 休憩 1h / 休憩なしは coalesce で 0")
    }

    func testHoursAndPayDisplay() throws {
        let e = try shiftWithBreak()
        // 1 桁に丸めると 7.25 時間が「7.3 時間」になり給料(7.25 × 1100)と合わなかった
        XCTAssertEqual(FieldCoding.display(7.25, field: e.field("hours")), "7.25 時間")
        XCTAssertEqual(FieldCoding.display(8, field: e.field("hours")), "8 時間")
        XCTAssertEqual(FieldCoding.display(7.5, field: e.field("hours")), "7.5 時間")
        XCTAssertEqual(Aggregator.format(15.25, as: nil, unit: "時間"), "15.25 時間")
        XCTAssertEqual(Aggregator.format(1 / 3, as: nil, unit: "時間"), "0.33 時間")
        // #150/#183: 通貨の unit「円」は ¥ と二重にしない
        XCTAssertEqual(FieldCoding.display(16500, field: e.field("pay")), FieldCoding.currency(16500))
        XCTAssertFalse(FieldCoding.display(16500, field: e.field("pay")).contains("円"))
    }

    func testPlainNumber() {
        XCTAssertEqual(FieldCoding.plainNumber(12.5), "12.5")
        XCTAssertEqual(FieldCoding.plainNumber(7.25), "7.25")
        XCTAssertEqual(FieldCoding.plainNumber(7.999), "8")
        XCTAssertEqual(FieldCoding.plainNumber(-0.75), "-0.75")
        XCTAssertEqual(FieldCoding.plainNumber(3), "3")
    }
}
