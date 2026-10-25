import XCTest
@testable import MetamonRenderer

/// `healthStat`(#167)のデータ源。モックの固定値と日付の区切り。
final class HealthDataTests: XCTestCase {
    func testDayStartsAreOldestFirstAndEndToday() {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        let now = ISO8601DateFormatter().date(from: "2026-10-25T09:30:00+09:00")!
        let starts = healthDayStarts(days: 7, now: now, calendar: cal)
        XCTAssertEqual(starts.count, 7)
        XCTAssertEqual(starts.last, cal.startOfDay(for: now))
        XCTAssertEqual(cal.dateComponents([.day], from: starts.first!, to: starts.last!).day, 6)
        XCTAssertEqual(healthDayStarts(days: 0, now: now, calendar: cal).count, 1)
    }

    func testMockReturnsFixedSeries() async throws {
        let mock = MockHealthData()
        let today = try await mock.steps(days: 1)
        XCTAssertEqual(today.map(\.value), [6240])
        let week = try await mock.steps(days: 7)
        XCTAssertEqual(week.map(\.value), MockHealthData.stepSeries)
        let weights = try await mock.weights(days: 30) // 7 日分しか無い時は古い日を詰めない
        XCTAssertEqual(weights.count, 7)
        XCTAssertEqual(weights.last?.value, 61.8)
        XCTAssertTrue(Calendar.current.isDateInToday(weights.last!.day))
    }

    func testErrorMessages() {
        XCTAssertFalse((HealthDataError.denied.errorDescription ?? "").isEmpty)
        XCTAssertNotEqual(HealthDataError.denied.errorDescription, HealthDataError.unavailable.errorDescription)
    }
}
