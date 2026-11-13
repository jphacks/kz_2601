import XCTest
#if canImport(HealthKit)
import HealthKit
#endif
@testable import MetamonRenderer

/// ヘルスケアのワークアウト(#2236): 種類の変換・距離の無いワークアウト・期間の絞り込み。
final class HealthWorkouts2236Tests: XCTestCase {
    private let base = Date(timeIntervalSince1970: 1_800_000_000)

    func testPeriodFilterIsHalfOpenAndOldestFirst() {
        let ws = [
            HealthWorkout(activity: .running, start: base.addingTimeInterval(3600), duration: 600),
            HealthWorkout(activity: .yoga, start: base, duration: 900),
            HealthWorkout(activity: .cycling, start: base.addingTimeInterval(7200), duration: 1200),
        ]
        let got = healthWorkouts(ws, from: base, to: base.addingTimeInterval(7200))
        XCTAssertEqual(got.map(\.activity), [.yoga, .running]) // 終わりちょうどに始まったものは入らない
        XCTAssertEqual(healthWorkouts(ws, from: base.addingTimeInterval(10_000), to: base.addingTimeInterval(20_000)), [])
    }

    func testWorkoutWithoutDistance() {
        XCTAssertNil(healthPositive(nil))
        XCTAssertNil(healthPositive(0))
        XCTAssertNil(healthPositive(.nan))
        XCTAssertEqual(healthPositive(5.2), 5.2)
        let yoga = HealthWorkout(activity: .yoga, start: base, duration: 1800, distanceKm: healthPositive(0), energyKcal: 90)
        XCTAssertNil(yoga.distanceKm)
        XCTAssertEqual(yoga.energyKcal, 90)
    }

    func testMockHasRunAndYoga() async throws {
        let today = Calendar.current.startOfDay(for: Date())
        let got = try await MockHealthData().workouts(from: today.addingTimeInterval(-86_400), to: today.addingTimeInterval(86_400))
        XCTAssertEqual(got.map(\.activity), [.yoga, .running])
        XCTAssertNil(got[0].distanceKm)
        XCTAssertEqual(got[1].distanceKm, 5.2)
    }

    func testActivityNamesAreSixteen() {
        XCTAssertEqual(HealthWorkout.Activity.allCases.count, 16) // 15 種 + その他
        XCTAssertEqual(HealthWorkout.Activity.highIntensityIntervalTraining.rawValue, "highIntensityIntervalTraining")
    }

    #if os(iOS)
    func testActivityTypeConversion() {
        XCTAssertEqual(HealthWorkout.Activity(.running), .running)
        XCTAssertEqual(HealthWorkout.Activity(.walking), .walking)
        XCTAssertEqual(HealthWorkout.Activity(.handCycling), .cycling)
        XCTAssertEqual(HealthWorkout.Activity(.socialDance), .dance)
        XCTAssertEqual(HealthWorkout.Activity(.cardioDance), .dance)
        XCTAssertEqual(HealthWorkout.Activity(.stairs), .stairClimbing)
        XCTAssertEqual(HealthWorkout.Activity(.highIntensityIntervalTraining), .highIntensityIntervalTraining)
        XCTAssertEqual(HealthWorkout.Activity(.archery), .other)
        XCTAssertEqual(HealthWorkout.Activity(.other), .other)
    }
    #endif
}
