import XCTest
@testable import MetamonDSL

/// 日付の厳しい読み方(`FieldCoding.strictDatetime`・`strictDay`)と表示(`displayDate`)を関数ごとに。
final class FieldCodingDateTests: XCTestCase {
    private func utc(_ y: Int, _ mo: Int, _ d: Int, _ h: Int, _ mi: Int, _ s: Int = 0) -> Date {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal.date(from: DateComponents(year: y, month: mo, day: d, hour: h, minute: mi, second: s))!
    }

    /// 端末のタイムゾーン(strictDay・displayDate が使う)での日時。
    private func local(_ y: Int, _ mo: Int, _ d: Int, _ h: Int = 0, _ mi: Int = 0) -> Date {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = FieldCoding.dateFormatter.timeZone
        return cal.date(from: DateComponents(year: y, month: mo, day: d, hour: h, minute: mi))!
    }

    // MARK: - strictDatetime

    func testStrictDatetimeZuluAndOffset() {
        XCTAssertEqual(FieldCoding.strictDatetime("2026-11-13T10:00:00Z"), utc(2026, 11, 13, 10, 0))
        XCTAssertEqual(FieldCoding.strictDatetime("2026-11-13T19:00:00+09:00"), utc(2026, 11, 13, 10, 0))
        XCTAssertEqual(FieldCoding.strictDatetime("2026-11-13T05:30:00-04:30"), utc(2026, 11, 13, 10, 0))
        // 日をまたぐオフセット
        XCTAssertEqual(FieldCoding.strictDatetime("2026-11-14T01:00:00+09:00"), utc(2026, 11, 13, 16, 0))
    }

    func testStrictDatetimeFractionalSeconds() throws {
        let d = try XCTUnwrap(FieldCoding.strictDatetime("2026-11-13T10:00:00.5Z"))
        XCTAssertEqual(d.timeIntervalSince(utc(2026, 11, 13, 10, 0)), 0.5, accuracy: 1e-6)
        let e = try XCTUnwrap(FieldCoding.strictDatetime("2026-11-13T19:00:59.123+09:00"))
        XCTAssertEqual(e.timeIntervalSince(utc(2026, 11, 13, 10, 0, 59)), 0.123, accuracy: 1e-6)
    }

    func testStrictDatetimeRejectsBadForms() {
        for bad in [
            "2026-11-13T10:00Z", // 秒が無い
            "2026-11-13T10:00:00", // オフセットが無い
            "2026-11-13 10:00:00Z", // T が無い
            "2026-02-30T10:00:00Z", // あり得ない日
            "2026-11-13T24:00:00Z", // 24 時
            "2026-11-13T10:60:00Z",
            "2026-11-13",
            "",
        ] {
            XCTAssertNil(FieldCoding.strictDatetime(bad), bad)
        }
    }

    // MARK: - strictDay

    func testStrictDayIsLocalStartOfDay() {
        XCTAssertEqual(FieldCoding.strictDay("2026-11-13"), local(2026, 11, 13))
        XCTAssertEqual(FieldCoding.strictDay("2024-02-29"), local(2024, 2, 29)) // 閏日
        XCTAssertEqual(FieldCoding.strictDay("2026-1-5"), local(2026, 1, 5)) // 月・日は 1 桁も可
    }

    func testStrictDayRejectsOtherForms() {
        for bad in ["2025-02-29", "2026-13-01", "2026-11-31", "2026/11/13", "20261113", "2026-11-13T00:00:00Z",
                    " 2026-11-13", "2026-11", ""] {
            XCTAssertNil(FieldCoding.strictDay(bad), bad)
        }
    }

    // MARK: - displayDate

    private let ja = Locale(identifier: "ja_JP")
    private let en = Locale(identifier: "en_US")

    func testDisplayDateJapanese() {
        let now = local(2026, 11, 13, 12) // 金曜
        // 今日・昨日も同じ書式(「今日」とは書かない)
        XCTAssertEqual(FieldCoding.displayDate(local(2026, 11, 13, 14, 30), withTime: false, locale: ja, now: now), "11/13(金)")
        XCTAssertEqual(FieldCoding.displayDate(local(2026, 11, 13, 14, 30), withTime: true, locale: ja, now: now), "11/13(金) 14:30")
        XCTAssertEqual(FieldCoding.displayDate(local(2026, 11, 12, 9, 5), withTime: true, locale: ja, now: now), "11/12(木) 9:05")
        // 今年の別の月
        XCTAssertEqual(FieldCoding.displayDate(local(2026, 1, 1), withTime: false, locale: ja, now: now), "1/1(木)")
        // 去年は年を付ける
        XCTAssertEqual(FieldCoding.displayDate(local(2025, 12, 31), withTime: false, locale: ja, now: now), "2025/12/31(水)")
        XCTAssertEqual(FieldCoding.displayDate(local(2025, 12, 31, 23, 59), withTime: true, locale: ja, now: now), "2025/12/31(水) 23:59")
    }

    func testDisplayDateEnglish() {
        let now = local(2026, 11, 13, 12)
        let today = FieldCoding.displayDate(local(2026, 11, 13, 14, 30), withTime: false, locale: en, now: now)
        XCTAssertTrue(today.contains("Fri") && today.contains("Nov") && today.contains("13"), today)
        XCTAssertFalse(today.contains("2026"), today)
        XCTAssertFalse(today.contains("2:30"), today)

        let withTime = FieldCoding.displayDate(local(2026, 11, 13, 14, 30), withTime: true, locale: en, now: now)
        XCTAssertTrue(withTime.contains("Nov") && withTime.contains("2:30") && withTime.contains("PM"), withTime)

        let lastYear = FieldCoding.displayDate(local(2025, 12, 31), withTime: false, locale: en, now: now)
        XCTAssertTrue(lastYear.contains("Wed") && lastYear.contains("Dec") && lastYear.contains("31") && lastYear.contains("2025"), lastYear)
    }
}
