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
        XCTAssertEqual(out.map(\.value), [Double(7 * 60)])
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

    // MARK: 気分(State of Mind・#2234)

    func testValenceRange() {
        XCTAssertTrue(healthValenceIsInRange(0))
        XCTAssertTrue(healthValenceIsInRange(-1))
        XCTAssertTrue(healthValenceIsInRange(1))
        XCTAssertTrue(healthValenceIsInRange(-0.75))
        XCTAssertFalse(healthValenceIsInRange(-1.0001))
        XCTAssertFalse(healthValenceIsInRange(1.5))
        XCTAssertFalse(healthValenceIsInRange(.nan))
        XCTAssertFalse(healthValenceIsInRange(.infinity))
    }

    func testMoodLabelConversionDropsUnknownAndDuplicates() {
        XCTAssertEqual(healthMoodLabels(fromRawValues: ["happy", "grateful"]), [.happy, .grateful])
        XCTAssertEqual(healthMoodLabels(fromRawValues: ["calm", "きげんがいい", "calm", "sad"]), [.calm, .sad])
        XCTAssertEqual(healthMoodLabels(fromRawValues: []), [])
        XCTAssertEqual(healthMoodLabels(fromRawValues: ["Happy"]), []) // 大文字小文字は区別する
        XCTAssertEqual(HealthMoodLabel.allCases.count, 38)
        XCTAssertEqual(Set(HealthMoodLabel.allCases.map(\.rawValue)).count, 38)
        XCTAssertEqual(healthMoodLabels(fromRawValues: HealthMoodLabel.allCases.map(\.rawValue)), HealthMoodLabel.allCases)
    }

    func testMoodKindRawValuesMatchHealthKitNames() {
        XCTAssertEqual(HealthMoodKind.allCases.map(\.rawValue), ["momentaryEmotion", "dailyMood"])
    }

    func testMoodEntryDoesNotLeakTheMoodInItsDescription() {
        let entry = HealthMoodEntry(date: t("2026-10-25T09:00:00"), kind: .dailyMood, valence: -0.8, labels: [.lonely, .sad])
        for text in [entry.description, entry.debugDescription, "\(entry)"] {
            XCTAssertFalse(text.contains("lonely"))
            XCTAssertFalse(text.contains("sad"))
            XCTAssertFalse(text.contains("0.8"))
            XCTAssertFalse(text.contains("dailyMood"))
        }
    }

    func testMockMoodsFilterByPeriodAndKind() async throws {
        let mock = MockHealthData()
        let now = Date()
        let all = try await mock.moods(from: now.addingTimeInterval(-7 * 86400), to: now, kind: nil)
        XCTAssertEqual(all.count, MockHealthData.moodOffsets.count)
        XCTAssertEqual(all.map(\.date), all.map(\.date).sorted()) // 古い順
        XCTAssertTrue(all.allSatisfy { healthValenceIsInRange($0.valence) })

        let daily = try await mock.moods(from: now.addingTimeInterval(-7 * 86400), to: now, kind: .dailyMood)
        XCTAssertEqual(daily.map(\.kind), [.dailyMood, .dailyMood])
        let momentary = try await mock.moods(from: now.addingTimeInterval(-7 * 86400), to: now, kind: .momentaryEmotion)
        XCTAssertEqual(momentary.map(\.kind), [.momentaryEmotion, .momentaryEmotion])

        let lastHours = try await mock.moods(from: now.addingTimeInterval(-4 * 3600), to: now, kind: nil)
        XCTAssertEqual(lastHours.count, 2)
        let empty = try await mock.moods(from: now.addingTimeInterval(86400), to: now.addingTimeInterval(2 * 86400), kind: nil)
        XCTAssertTrue(empty.isEmpty)
    }

    func testSaveMoodRejectsValenceOutOfRange() async {
        let mock = MockHealthData()
        do {
            try await mock.saveMood(HealthMoodEntry(date: Date(), kind: .momentaryEmotion, valence: 0.5, labels: [.calm]))
        } catch {
            XCTFail("範囲内は通る: \(error)")
        }
        for bad in [1.2, -2.0, Double.nan] {
            do {
                try await mock.saveMood(HealthMoodEntry(date: Date(), kind: .momentaryEmotion, valence: bad))
                XCTFail("範囲の外は断る: \(bad)")
            } catch {
                XCTAssertEqual(error as? HealthDataError, .outOfRange)
            }
        }
    }

    func testMoodErrorMessagesAreDistinct() {
        let all: [HealthDataError] = [.unavailable, .denied, .requiresIOS18, .outOfRange]
        let messages = all.map { $0.errorDescription ?? "" }
        XCTAssertFalse(messages.contains(""))
        XCTAssertEqual(Set(messages).count, all.count)
    }

    /// 気分の口を実装していない差し替え先は「使えません」を返す(既存のホストを壊さない)。
    func testDataSourceWithoutMoodSupportIsUnavailable() async {
        struct StepsOnly: HealthDataSource {
            func steps(days: Int) async throws -> [HealthDailyValue] { [] }
            func weights(days: Int) async throws -> [HealthDailyValue] { [] }
            func saveWeight(kg: Double, at date: Date) async throws {}
        }
        let source = StepsOnly()
        do {
            _ = try await source.moods(from: .distantPast, to: Date(), kind: nil)
            XCTFail("使えないはず")
        } catch {
            XCTAssertEqual(error as? HealthDataError, .unavailable)
        }
        do {
            try await source.saveMood(HealthMoodEntry(date: Date(), kind: .dailyMood, valence: 0))
            XCTFail("使えないはず")
        } catch {
            XCTAssertEqual(error as? HealthDataError, .unavailable)
        }
    }

    /// iOS 18 未満 / iOS でない所では読み書きできない。
    func testLiveMoodsRequireIOS18() async {
        let live = LiveHealthData()
        let expected: HealthDataError
        #if os(iOS)
        if #available(iOS 18.0, *) { return } // iOS 18 以降は許可ダイアログが要るのでここでは試さない
        expected = .requiresIOS18
        #else
        expected = .unavailable
        #endif
        do {
            _ = try await live.moods(from: .distantPast, to: Date(), kind: nil)
            XCTFail("iOS 18 未満では使えない")
        } catch {
            XCTAssertEqual(error as? HealthDataError, expected)
        }
        do {
            try await live.saveMood(HealthMoodEntry(date: Date(), kind: .dailyMood, valence: 0))
            XCTFail("iOS 18 未満では使えない")
        } catch {
            XCTAssertEqual(error as? HealthDataError, expected)
        }
    }
}
