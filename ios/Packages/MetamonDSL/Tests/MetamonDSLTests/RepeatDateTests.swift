import XCTest
@testable import MetamonDSL

/// `RepeatDate`(毎月・毎年くり返す日付・#266)。月末の寄せ方・次の回・タイムゾーン。
final class RepeatDateTests: XCTestCase {
    private func calendar(_ tz: String) -> Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: tz)!
        return c
    }

    private func day(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 0, _ cal: Calendar) -> Date {
        cal.date(from: DateComponents(year: y, month: m, day: d, hour: h))!
    }

    private func ymd(_ d: Date?, _ cal: Calendar) -> [Int]? {
        d.map { cal.dateComponents([.year, .month, .day], from: $0) }.map { [$0.year!, $0.month!, $0.day!] }
    }

    func testMonths() {
        XCTAssertEqual(RepeatDate.months("monthly"), 1)
        XCTAssertEqual(RepeatDate.months("yearly"), 12)
        XCTAssertNil(RepeatDate.months("weekly"))
        XCTAssertNil(RepeatDate.months(""))
    }

    func testMonthlyFromJan31ClampsToMonthEndAndReturns() {
        let cal = calendar("Asia/Tokyo")
        let anchor = day(2025, 1, 31, 9, cal)
        XCTAssertEqual(ymd(RepeatDate.occurrence(anchor, months: 1, index: 1, calendar: cal), cal), [2025, 2, 28])
        XCTAssertEqual(ymd(RepeatDate.occurrence(anchor, months: 1, index: 2, calendar: cal), cal), [2025, 3, 31])
        // 閏年は 2/29
        let leap = day(2024, 1, 31, 9, cal)
        XCTAssertEqual(ymd(RepeatDate.occurrence(leap, months: 1, index: 1, calendar: cal), cal), [2024, 2, 29])
        // 時刻はそのまま
        let t = RepeatDate.occurrence(anchor, months: 1, index: 1, calendar: cal)!
        XCTAssertEqual(cal.component(.hour, from: t), 9)
    }

    func testYearlyFromFeb29ClampsInCommonYear() {
        let cal = calendar("Asia/Tokyo")
        let anchor = day(2024, 2, 29, 0, cal)
        XCTAssertEqual(ymd(RepeatDate.occurrence(anchor, months: 12, index: 1, calendar: cal), cal), [2025, 2, 28])
        XCTAssertEqual(ymd(RepeatDate.occurrence(anchor, months: 12, index: 4, calendar: cal), cal), [2028, 2, 29])
    }

    func testMonthsBetween() {
        let cal = calendar("Asia/Tokyo")
        XCTAssertEqual(RepeatDate.monthsBetween(day(2025, 1, 31, 0, cal), day(2025, 3, 1, 0, cal), calendar: cal), 2)
        XCTAssertEqual(RepeatDate.monthsBetween(day(2024, 11, 5, 0, cal), day(2025, 2, 1, 0, cal), calendar: cal), 3)
        XCTAssertEqual(RepeatDate.monthsBetween(day(2025, 3, 1, 0, cal), day(2025, 1, 1, 0, cal), calendar: cal), -2)
    }

    func testNextWhenAnchorIsTodayOrLater() {
        let cal = calendar("Asia/Tokyo")
        let today = day(2025, 5, 10, 15, cal)
        // 今日ちょうど(時刻は落とす)
        XCTAssertEqual(RepeatDate.next(day(2025, 5, 10, 8, cal), unit: "monthly", today: today, calendar: cal), day(2025, 5, 10, 0, cal))
        // 未来の起点はそのまま起点の日
        XCTAssertEqual(RepeatDate.next(day(2025, 8, 1, 20, cal), unit: "yearly", today: today, calendar: cal), day(2025, 8, 1, 0, cal))
    }

    func testNextFromPastAnchor() {
        let cal = calendar("Asia/Tokyo")
        // 1/31 の毎月・今日 2/15 → 2/28
        XCTAssertEqual(RepeatDate.next(day(2025, 1, 31, 0, cal), unit: "monthly", today: day(2025, 2, 15, 0, cal), calendar: cal),
                       day(2025, 2, 28, 0, cal))
        // 今日 3/1 → 3/31(元の日に戻る)
        XCTAssertEqual(RepeatDate.next(day(2025, 1, 31, 0, cal), unit: "monthly", today: day(2025, 3, 1, 0, cal), calendar: cal),
                       day(2025, 3, 31, 0, cal))
        // 今月の回が過ぎていれば来月
        XCTAssertEqual(RepeatDate.next(day(2025, 1, 5, 0, cal), unit: "monthly", today: day(2025, 4, 6, 0, cal), calendar: cal),
                       day(2025, 5, 5, 0, cal))
        // 2/29 の毎年・今日 2025/1/1 → 2025/2/28
        XCTAssertEqual(RepeatDate.next(day(2024, 2, 29, 0, cal), unit: "yearly", today: day(2025, 1, 1, 0, cal), calendar: cal),
                       day(2025, 2, 28, 0, cal))
        // 毎年・今年の回が過ぎていれば来年
        XCTAssertEqual(RepeatDate.next(day(2020, 3, 3, 0, cal), unit: "yearly", today: day(2025, 3, 4, 0, cal), calendar: cal),
                       day(2026, 3, 3, 0, cal))
    }

    func testNextUnknownUnitIsNil() {
        let cal = calendar("Asia/Tokyo")
        XCTAssertNil(RepeatDate.next(day(2025, 1, 1, 0, cal), unit: "daily", today: day(2025, 2, 1, 0, cal), calendar: cal))
    }

    func testNextUsesCalendarTimeZone() {
        // 2025-01-31 00:30 JST = 2025-01-30 15:30 UTC。暦の日はカレンダーのタイムゾーンで決まる
        let tokyo = calendar("Asia/Tokyo"), utc = calendar("UTC")
        let anchor = day(2025, 1, 31, 0, tokyo).addingTimeInterval(30 * 60)
        let today = day(2025, 2, 10, 12, utc)
        XCTAssertEqual(ymd(RepeatDate.next(anchor, unit: "monthly", today: today, calendar: tokyo), tokyo), [2025, 2, 28])
        XCTAssertEqual(ymd(RepeatDate.next(anchor, unit: "monthly", today: today, calendar: utc), utc), [2025, 2, 28])
        // UTC では起点が 1/30 なので、今日 3/1 なら 3/30(JST では 3/31)
        let march = day(2025, 3, 1, 12, utc)
        XCTAssertEqual(ymd(RepeatDate.next(anchor, unit: "monthly", today: march, calendar: utc), utc), [2025, 3, 30])
        XCTAssertEqual(ymd(RepeatDate.next(anchor, unit: "monthly", today: march, calendar: tokyo), tokyo), [2025, 3, 31])
    }
}
