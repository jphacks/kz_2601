import XCTest
@testable import MetamonDSL

/// 式の時刻の扱い(timeOfDay・clockString と、それを使う addMinutes)。hoursBetween は LogicV2Tests にある。
final class ExprEvaluatorClockTests: XCTestCase {
    private var utc: Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "UTC")!
        return c
    }

    private func addMinutes(_ time: String, _ minutes: Double) throws -> ExprValue? {
        let json = #"{"op":"addMinutes","args":["\#(time)",\#(minutes)]}"#
        let e = try JSONDecoder().decode(Expr.self, from: Data(json.utf8))
        return ExprEvaluator(entities: [], records: { _ in [] }, calendar: utc).evaluate(e)
    }

    private func text(_ time: String, _ minutes: Double) throws -> String? {
        guard case .string(let s)? = try addMinutes(time, minutes) else { return nil }
        return s
    }

    // MARK: - timeOfDay

    func testTimeOfDayAcceptedForms() {
        XCTAssertEqual(ExprEvaluator.timeOfDay("00:00"), 0)
        XCTAssertEqual(ExprEvaluator.timeOfDay("9:05"), 9 * 3600 + 5 * 60)
        XCTAssertEqual(ExprEvaluator.timeOfDay("23:59:59"), 86_399, "秒の端")
        XCTAssertEqual(ExprEvaluator.timeOfDay(" 12:30 "), 12 * 3600 + 30 * 60, "前後の空白は許す")
    }

    func testTimeOfDayRejectsBrokenForms() {
        for bad in ["", "24:00", "12:60", "12:30:60", "12", "1:2:3:4", "ab:cd", "12:3x", "123:00", "12::00", ":30", "-1:30", "１２:３０"] {
            XCTAssertNil(ExprEvaluator.timeOfDay(bad), bad)
        }
    }

    // MARK: - clockString

    func testClockStringFormat() {
        XCTAssertEqual(ExprEvaluator.clockString(0), "00:00")
        XCTAssertEqual(ExprEvaluator.clockString(9 * 3600 + 5 * 60), "09:05")
        XCTAssertEqual(ExprEvaluator.clockString(86_399), "23:59:59", "秒があれば秒まで出す")
    }

    func testClockStringWrapsAroundDay() {
        XCTAssertEqual(ExprEvaluator.clockString(86_400), "00:00", "24 時は 0 時")
        XCTAssertEqual(ExprEvaluator.clockString(86_400 + 90 * 60), "01:30")
        XCTAssertEqual(ExprEvaluator.clockString(-60), "23:59", "負の値は前の日の終わりから")
        XCTAssertEqual(ExprEvaluator.clockString(-86_400 - 3600), "23:00")
    }

    // MARK: - addMinutes(時刻の文字列)

    func testAddMinutesOnClockString() throws {
        XCTAssertEqual(try text("09:00", 90), "10:30")
        XCTAssertEqual(try text("9:00", 0), "09:00", "H:mm は HH:mm にそろう")
        XCTAssertEqual(try text("10:00", -30), "09:30")
    }

    func testAddMinutesWrapsAroundMidnight() throws {
        XCTAssertEqual(try text("23:30", 45), "00:15", "0 時をまたぐ")
        XCTAssertEqual(try text("00:10", -20), "23:50")
        XCTAssertEqual(try text("12:00", 1440), "12:00", "ちょうど 1 日")
    }

    func testAddMinutesKeepsSecondsWhenPresent() throws {
        XCTAssertEqual(try text("10:00:30", 1), "10:01:30")
        XCTAssertEqual(try text("10:00:00", 1), "10:01", "秒が 0 なら出さない")
    }

    func testAddMinutesRoundsFractionalMinutes() throws {
        XCTAssertEqual(try text("10:00", 1.4), "10:01")
        XCTAssertEqual(try text("10:00", 1.6), "10:02")
    }

    func testAddMinutesRejectsBrokenInput() throws {
        XCTAssertNil(try addMinutes("25:00", 10))
        XCTAssertNil(try addMinutes("abc", 10))
        let tooMany = Double(ExprEvaluator.maxAddMinutes) + 1
        XCTAssertNil(try addMinutes("10:00", tooMany), "上限を超える分は nil")
        XCTAssertNil(try addMinutes("10:00", -tooMany))
    }

    // MARK: - addMinutes(日時は経過時間)

    func testAddMinutesOnDatetimeKeepsDate() throws {
        let start = Date(timeIntervalSince1970: 1_800_000_000)
        let e = try JSONDecoder().decode(Expr.self, from: Data(#"{"op":"addMinutes","args":[{"op":"now","args":[]},90]}"#.utf8))
        let v = ExprEvaluator(entities: [], records: { _ in [] }, now: start, calendar: utc).evaluate(e)
        guard case .date(let d)? = v else { return XCTFail("日時が返るはず: \(String(describing: v))") }
        XCTAssertEqual(d.timeIntervalSince(start), 90 * 60, accuracy: 0.001, "日付をまたいでも経過時間で足す")
    }
}
