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
    // MARK: 睡眠の集計(#1986)

    private var tokyo: Calendar {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return cal
    }

    private func t(_ s: String) -> Date { ISO8601DateFormatter().date(from: s + "+09:00")! }

    private func sleep(_ a: String, _ b: String, _ stage: HealthSleepSample.Stage = .asleep) -> HealthSleepSample {
        HealthSleepSample(start: t(a), end: t(b), stage: stage)
    }

    private var week: [Date] { healthDayStarts(days: 7, now: t("2026-10-25T12:00:00"), calendar: tokyo) }

    func testSleepOverlapFromWatchAndPhoneIsCountedOnce() {
        let samples = [
            sleep("2026-10-25T00:00:00", "2026-10-25T06:00:00"), // Apple Watch
            sleep("2026-10-25T01:00:00", "2026-10-25T07:00:00"), // iPhone(重なる)
            sleep("2026-10-25T02:00:00", "2026-10-25T03:00:00"), // 中に含まれる
        ]
        let out = sleepMinutesByWakeDay(samples, dayStarts: week, calendar: tokyo)
        XCTAssertEqual(out, [HealthDailyValue(day: tokyo.startOfDay(for: t("2026-10-25T00:00:00")), value: 7 * 60)])
    }

    func testSleepAcrossMidnightGoesToWakeDay() {
        let samples = [
            sleep("2026-10-23T23:00:00", "2026-10-23T23:50:00"),
            sleep("2026-10-23T23:50:00", "2026-10-24T00:10:00", .awake), // 夜中に起きた
            sleep("2026-10-24T00:10:00", "2026-10-24T06:30:00"),
        ]
        let out = sleepMinutesByWakeDay(samples, dayStarts: week, calendar: tokyo)
        XCTAssertEqual(out.count, 1)
        XCTAssertEqual(out[0].day, tokyo.startOfDay(for: t("2026-10-24T06:30:00")))
        XCTAssertEqual(out[0].value, 50 + 380) // 起きていた 20 分は数えない
    }

    func testSleepExcludesInBed() {
        let samples = [
            sleep("2026-10-24T22:30:00", "2026-10-25T07:00:00", .inBed),
            sleep("2026-10-24T23:00:00", "2026-10-25T06:00:00"),
        ]
        let out = sleepMinutesByWakeDay(samples, dayStarts: week, calendar: tokyo)
        XCTAssertEqual(out.map(\.value), [7 * 60])
    }

    func testSleepDaysWithoutDataAreOmittedAndOutOfRangeDropped() {
        let samples = [
            sleep("2026-10-17T23:00:00", "2026-10-18T06:00:00"), // 範囲(10/19〜25)より前に起きた
            sleep("2026-10-20T23:00:00", "2026-10-21T05:00:00"),
            sleep("2026-10-21T14:00:00", "2026-10-21T14:30:00"), // 昼寝は別の眠り・同じ日に足す
            sleep("2026-10-23T23:00:00", "2026-10-24T06:00:00", .inBed), // inBed だけの日は記録なし
        ]
        let out = sleepMinutesByWakeDay(samples, dayStarts: week, calendar: tokyo)
        XCTAssertEqual(out, [HealthDailyValue(day: tokyo.startOfDay(for: t("2026-10-21T05:00:00")), value: 6 * 60 + 30)])
        XCTAssertEqual(sleepMinutesByWakeDay([], dayStarts: week, calendar: tokyo), [])
    }

    func testMockDailyHasSevenDaysForEveryMetric() async throws {
        let mock = MockHealthData()
        for metric in HealthMetric.allCases {
            let values = try await mock.daily(metric, days: 7)
            XCTAssertEqual(values.count, 7, "\(metric)")
            XCTAssertTrue(Calendar.current.isDateInToday(values.last!.day))
            XCTAssertTrue(values.allSatisfy { $0.value > 0 })
        }
        let today = try await mock.daily(.sleep, days: 1)
        XCTAssertEqual(today.map(\.value), [431])
    }

    func testLiveDailyIsUnavailableOffIOS() async {
        #if !os(iOS)
        do {
            _ = try await LiveHealthData().daily(.sleep, days: 7)
            XCTFail("macOS では使えない")
        } catch {
            XCTAssertEqual(error as? HealthDataError, .unavailable)
        }
        #endif
    }
}
