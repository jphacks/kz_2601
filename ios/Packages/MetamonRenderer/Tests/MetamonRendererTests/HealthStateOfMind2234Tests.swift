import XCTest
#if canImport(HealthKit)
import HealthKit
#endif
@testable import MetamonRenderer

/// ヘルスケアの気分(State of Mind・#2234)の読み書きの口。valence の範囲・ラベルの変換・iOS 18 未満。
final class HealthStateOfMind2234Tests: XCTestCase {
    func testValenceRange() throws {
        XCTAssertTrue(HealthMood.isValidValence(-1))
        XCTAssertTrue(HealthMood.isValidValence(0))
        XCTAssertTrue(HealthMood.isValidValence(1))
        XCTAssertFalse(HealthMood.isValidValence(1.0001))
        XCTAssertFalse(HealthMood.isValidValence(-1.5))
        XCTAssertFalse(HealthMood.isValidValence(.nan))
        XCTAssertFalse(HealthMood.isValidValence(.infinity))
        let bad = HealthMood(date: Date(), kind: .dailyMood, valence: 2)
        XCTAssertThrowsError(try bad.validated()) { XCTAssertEqual($0 as? HealthMoodError, .valenceOutOfRange) }
        XCTAssertFalse((HealthMoodError.valenceOutOfRange.errorDescription ?? "").isEmpty)
    }

    func testValidatedDropsDuplicateLabelsKeepingOrder() throws {
        let mood = HealthMood(date: Date(), kind: .momentaryEmotion, valence: 0.5, labels: [.happy, .calm, .happy])
        XCTAssertEqual(try mood.validated().labels, [.happy, .calm])
    }

    func testParseLabelsFromStrings() {
        XCTAssertEqual(HealthMoodLabel.parse(["happy", " Calm ", "unknown", "HAPPY", "worried"]), [.happy, .calm, .worried])
        XCTAssertEqual(HealthMoodLabel.parse([]), [])
        XCTAssertEqual(HealthMoodLabel.allCases.count, 38)
    }

    func testOlderThanIOS18IsUnsupported() {
        XCTAssertFalse(HealthMood.isSupported(osMajorVersion: 17))
        XCTAssertTrue(HealthMood.isSupported(osMajorVersion: 18))
        XCTAssertTrue(HealthMood.isSupported(osMajorVersion: 26))
    }

    func testFilterByRangeAndKindOldestFirst() {
        let base = Date(timeIntervalSince1970: 1_800_000_000)
        let moods = [
            HealthMood(date: base.addingTimeInterval(300), kind: .dailyMood, valence: 0.1),
            HealthMood(date: base.addingTimeInterval(100), kind: .momentaryEmotion, valence: 0.2),
            HealthMood(date: base.addingTimeInterval(900), kind: .momentaryEmotion, valence: 0.3),
        ]
        let all = healthMoods(moods, from: base, to: base.addingTimeInterval(600), kinds: [])
        XCTAssertEqual(all.map(\.valence), [0.2, 0.1])
        let daily = healthMoods(moods, from: base, to: base.addingTimeInterval(1000), kinds: [.dailyMood])
        XCTAssertEqual(daily.map(\.valence), [0.1])
    }

    func testMockReadsAndValidatesWrites() async throws {
        let mock = MockHealthData()
        let now = Date()
        let today = Calendar.current.startOfDay(for: now)
        let moods = try await mock.moods(from: today, to: today.addingTimeInterval(86_400), kinds: [])
        XCTAssertEqual(moods.count, 3)
        XCTAssertEqual(moods.map(\.date), moods.map(\.date).sorted())
        try await mock.saveMood(HealthMood(date: now, kind: .dailyMood, valence: 0.2, labels: [.calm]))
        do {
            try await mock.saveMood(HealthMood(date: now, kind: .dailyMood, valence: -3))
            XCTFail("範囲外は書かない")
        } catch {
            XCTAssertEqual(error as? HealthMoodError, .valenceOutOfRange)
        }
    }

    #if os(iOS)
    @available(iOS 18, *)
    func testLabelsRoundTripThroughHealthKit() throws {
        for label in HealthMoodLabel.allCases {
            XCTAssertEqual(HealthMoodLabel(label.healthKit), label, label.rawValue)
        }
        // HealthKit の名前と同じ(`HKStateOfMind.Label` は Int なので、すべて別の値になっているかで確かめる)
        XCTAssertEqual(Set(HealthMoodLabel.allCases.map(\.healthKit.rawValue)).count, HealthMoodLabel.allCases.count)
    }

    @available(iOS 18, *)
    func testSampleConversionRoundTrip() throws {
        let mood = HealthMood(date: Date(timeIntervalSince1970: 1_800_000_000), kind: .dailyMood, valence: -0.4, labels: [.stressed, .drained])
        let back = HealthMood(stateOfMind: mood.stateOfMind)
        XCTAssertEqual(back, mood)
    }
    #endif
}
