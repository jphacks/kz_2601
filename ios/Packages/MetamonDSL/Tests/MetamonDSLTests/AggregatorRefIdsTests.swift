import XCTest
@testable import MetamonDSL

/// 集計の ref の取り出し(Aggregator.refIds)と週の始まり(weekCalendar)(下書き stock7-ios-013)。
final class AggregatorRefIdsTests: XCTestCase {
    func testRefIdsFromSingleString() {
        XCTAssertEqual(Aggregator.refIds(.string("r1")), ["r1"])
        XCTAssertEqual(Aggregator.refIds(.string("")), [])
    }

    func testRefIdsFromArrayKeepsOrderAndDropsNonStringsAndEmpties() {
        XCTAssertEqual(Aggregator.refIds(.array([.string("a"), .string("b"), .string("a")])), ["a", "b", "a"], "重複はそのまま")
        XCTAssertEqual(Aggregator.refIds(.array([.string("a"), .number(1), .null, .bool(true), .string(""), .array([.string("x")]), .string("c")])), ["a", "c"])
    }

    func testRefIdsEmptyAndOtherTypes() {
        XCTAssertEqual(Aggregator.refIds(.array([])), [])
        XCTAssertEqual(Aggregator.refIds(.null), [])
        XCTAssertEqual(Aggregator.refIds(.number(3)), [])
        XCTAssertEqual(Aggregator.refIds(.bool(true)), [])
        XCTAssertEqual(Aggregator.refIds(.object(["id": .string("r1")])), [])
    }

    func testWeekCalendarStartsOnMondayWhateverTheBase() {
        for firstWeekday in 1...7 {
            var base = Calendar(identifier: .gregorian)
            base.firstWeekday = firstWeekday
            XCTAssertEqual(Aggregator.weekCalendar(base).firstWeekday, 2, "base=\(firstWeekday)")
        }
        XCTAssertEqual(Aggregator.weekCalendar().firstWeekday, 2)
    }

    func testWeekCalendarKeepsTheRestOfTheBase() {
        var base = Calendar(identifier: .gregorian)
        base.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        base.locale = Locale(identifier: "ja_JP")
        let cal = Aggregator.weekCalendar(base)
        XCTAssertEqual(cal.identifier, .gregorian)
        XCTAssertEqual(cal.timeZone, base.timeZone)
        XCTAssertEqual(cal.locale, base.locale)
        XCTAssertEqual(base.firstWeekday, 1, "元の暦は書き換えない(日曜始まりのまま)")
    }

    func testWeekIntervalRunsMondayToSunday() throws {
        var base = Calendar(identifier: .gregorian)
        base.timeZone = TimeZone(identifier: "UTC")!
        base.firstWeekday = 1
        let cal = Aggregator.weekCalendar(base)
        // 2026-11-15 は日曜: 週は 11/09(月)〜11/16(月・含まない)
        let sunday = try XCTUnwrap(cal.date(from: DateComponents(year: 2026, month: 11, day: 15, hour: 12)))
        let week = try XCTUnwrap(cal.dateInterval(of: .weekOfYear, for: sunday))
        XCTAssertEqual(cal.component(.weekday, from: week.start), 2, "週の頭は月曜")
        XCTAssertEqual(cal.dateComponents([.year, .month, .day], from: week.start), DateComponents(year: 2026, month: 11, day: 9))
        XCTAssertEqual(week.duration, 7 * 24 * 3600)
        // 翌日(月曜)は次の週
        let monday = try XCTUnwrap(cal.date(byAdding: .day, value: 1, to: sunday))
        XCTAssertEqual(cal.dateInterval(of: .weekOfYear, for: monday)?.start, week.end)
    }

    func testThisWeekFilterCountsSundayInTheSameWeekAsMonday() throws {
        var base = Calendar(identifier: .gregorian)
        base.timeZone = TimeZone(identifier: "UTC")!
        let f = Filter(field: "d", op: "thisWeek", value: nil)
        let now = try XCTUnwrap(base.date(from: DateComponents(year: 2026, month: 11, day: 11, hour: 9))) // 水
        let field = Field(id: "d", name: "日", type: .date)
        func hit(_ day: Int) -> Bool {
            Aggregator.matches(f, record: Record(id: "r", data: ["d": .string(String(format: "2026-11-%02dT10:00:00Z", day))]),
                               field: field, now: now, calendar: base)
        }
        XCTAssertTrue(hit(9), "月曜")
        XCTAssertTrue(hit(15), "日曜は同じ週")
        XCTAssertFalse(hit(8), "前の日曜は前の週")
        XCTAssertFalse(hit(16), "次の月曜は次の週")
    }
}
