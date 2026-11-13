import XCTest
@testable import MetamonDSL

/// chart の日付の束ね方(dateBucket)と、大小比較の filter の並び(filterOrder)。Android・デスクトップの Aggregator と同じ結果。
final class AggregatorBucketTests: XCTestCase {
    private let en = Locale(identifier: "en_US")

    /// "YYYY-MM-DD" を端末の TZ のその日にする(dateBucket と同じ TZ)。
    private func day(_ s: String) throws -> Date { try XCTUnwrap(FieldCoding.date(from: .string(s))) }

    func testDayKeyAndLabels() throws {
        let d = try day("2026-03-04")
        XCTAssertEqual(Aggregator.dateBucket(d, "day").0, "2026-03-04")
        XCTAssertEqual(Aggregator.dateBucket(d, "day").1, "3/4", "locale 無しは日本語")
        XCTAssertEqual(Aggregator.dateBucket(d, "day", locale: Locale(identifier: "ja_JP")).1, "3/4")
        XCTAssertEqual(Aggregator.dateBucket(d, "day", locale: en).1, "3/4")
    }

    func testMonthKeyAndLabels() throws {
        let d = try day("2026-11-30")
        XCTAssertEqual(Aggregator.dateBucket(d, "month").0, "2026-11")
        XCTAssertEqual(Aggregator.dateBucket(d, "month").1, "11月")
        XCTAssertEqual(Aggregator.dateBucket(d, "month", locale: en).1, "Nov")
        XCTAssertEqual(Aggregator.dateBucket(try day("2026-01-01"), "month").0, "2026-01", "月は 2 桁")
    }

    /// [d] を含む週の始まりの日(端末の firstWeekday・西暦)の "YYYY-MM-DD"。期待値を本体と別の数え方で出す。
    private func weekStart(_ d: Date) -> String {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = FieldCoding.dateFormatter.timeZone
        let back = (cal.component(.weekday, from: d) - Calendar.current.firstWeekday + 7) % 7
        let noon = cal.date(byAdding: .hour, value: 12, to: d)!
        return FieldCoding.dateFormatter.string(from: cal.date(byAdding: .day, value: -back, to: noon)!)
    }

    private func monthDay(_ key: String) -> String {
        let p = key.split(separator: "-").map { Int($0)! }
        return "\(p[1])/\(p[2])"
    }

    /// 週の始まりは端末の設定(Calendar.current.firstWeekday)に従う。
    func testWeekStartsOnTheDeviceFirstWeekday() throws {
        let wed = try day("2026-01-07")
        let expected = weekStart(wed)
        if Calendar.current.firstWeekday == 1 { XCTAssertEqual(expected, "2026-01-04") }
        if Calendar.current.firstWeekday == 2 { XCTAssertEqual(expected, "2026-01-05") }
        let (key, label) = Aggregator.dateBucket(wed, "week")
        XCTAssertEqual(key, expected)
        XCTAssertEqual(label, "\(monthDay(expected))週")
        XCTAssertEqual(Aggregator.dateBucket(wed, "week", locale: en).1, monthDay(expected))
        // 週の始まりの日と、その週の他の日も同じキー
        let startDay = try day(expected)
        XCTAssertEqual(Aggregator.dateBucket(startDay, "week").0, expected)
        XCTAssertEqual(Aggregator.dateBucket(startDay.addingTimeInterval(6 * 86_400 + 3_600), "week").0, expected)
        XCTAssertNotEqual(Aggregator.dateBucket(startDay.addingTimeInterval(-3_600), "week").0, expected, "前の日は前の週")
    }

    func testWeekCrossesTheYear() throws {
        // 2026-01-01 は木曜。どの曜日始まりでも、その週の始まりは前の年
        let d = try day("2026-01-01")
        let (key, label) = Aggregator.dateBucket(d, "week")
        XCTAssertEqual(key, weekStart(d))
        XCTAssertTrue(key.hasPrefix("2025-12-"), key)
        XCTAssertEqual(label, "\(monthDay(key))週")
    }

    /// year は束ね方に無い(Android と同じ)。知らない値と同じく日で束ねる。
    func testYearAndUnknownBucketFallBackToDay() throws {
        let d = try day("2026-03-04")
        XCTAssertEqual(Aggregator.dateBucket(d, "year").0, "2026-03-04")
        XCTAssertEqual(Aggregator.dateBucket(d, "year").1, "3/4")
        XCTAssertEqual(Aggregator.dateBucket(d, "decade", locale: en).1, "3/4")
    }

    // MARK: - filterOrder

    func testFilterOrderNumbers() {
        XCTAssertEqual(Aggregator.filterOrder(.number(1), .number(2)), .orderedAscending)
        XCTAssertEqual(Aggregator.filterOrder(.number(2), .number(2)), .orderedSame)
        XCTAssertEqual(Aggregator.filterOrder(.string("1,200"), .number(1000)), .orderedDescending, "カンマ付きの数の文字列も数")
        XCTAssertEqual(Aggregator.filterOrder(.string("10"), .string("9")), .orderedDescending, "数の文字列同士は文字でなく数で比べる")
    }

    func testFilterOrderDates() {
        XCTAssertEqual(Aggregator.filterOrder(.string("2026-01-02"), .string("2026-01-10")), .orderedAscending)
        XCTAssertEqual(Aggregator.filterOrder(.string("2026-01-02"), .string("2026-01-02")), .orderedSame)
        XCTAssertEqual(Aggregator.filterOrder(.string("2026-01-02T00:00:00Z"), .string("2026-01-01T23:00:00Z")), .orderedDescending)
    }

    func testFilterOrderReturnsNilForTextAndMixedTypes() {
        XCTAssertNil(Aggregator.filterOrder(.string("りんご"), .string("みかん")), "文字は比べない")
        XCTAssertNil(Aggregator.filterOrder(.string("2026-01-02"), .number(5)), "日付と数")
        XCTAssertNil(Aggregator.filterOrder(.bool(true), .number(1)))
        XCTAssertNil(Aggregator.filterOrder(.number(1), .null))
        XCTAssertNil(Aggregator.filterOrder(.string("2026-02-30"), .string("2026-03-01")), "あり得ない日付は読まない")
    }
}
