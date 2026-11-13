// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(HealthData.swift と同じ)。
#if !APP_STORE_REVIEW
import Foundation
#if os(iOS)
import HealthKit
#endif

/// ヘルスケアのワークアウト 1 件(`HKWorkout`・#2236)。HealthKit に依存しない形で持ち、部品・DSL にはまだつながない(#1665 の決定待ち)。
public struct HealthWorkout: Sendable, Equatable {
    /// 種類(よく使う 15 種 + その他)。文字は HealthKit の名前に寄せる
    public enum Activity: String, CaseIterable, Sendable {
        case walking, running, cycling, swimming, hiking
        case yoga, pilates, dance, elliptical, rowing
        case stairClimbing, functionalStrengthTraining, traditionalStrengthTraining, highIntensityIntervalTraining, coreTraining
        case other
    }

    public var activity: Activity
    public var start: Date
    /// 秒
    public var duration: TimeInterval
    /// km。距離の無いワークアウト(ヨガ・筋トレ等)は nil
    public var distanceKm: Double?
    /// 消費カロリー(アクティブ)kcal。記録が無ければ nil
    public var energyKcal: Double?
    /// 出どころ(「Apple Watch」・アプリ名など)
    public var source: String

    public init(activity: Activity, start: Date, duration: TimeInterval, distanceKm: Double? = nil, energyKcal: Double? = nil, source: String = "") {
        self.activity = activity
        self.start = start
        self.duration = duration
        self.distanceKm = distanceKm
        self.energyKcal = energyKcal
        self.source = source
    }
}

/// 期間(始まりが `from..<to`)で絞り、古い順に並べる
func healthWorkouts(_ workouts: [HealthWorkout], from: Date, to: Date) -> [HealthWorkout] {
    workouts.filter { $0.start >= from && $0.start < to }.sorted { $0.start < $1.start }
}

/// 0 以下・非数の量は「無い」とする(HealthKit は距離の無いワークアウトに 0 を返すことがある)
func healthPositive(_ v: Double?) -> Double? {
    guard let v, v.isFinite, v > 0 else { return nil }
    return v
}

/// `MockHealthData` のワークアウト: 今日の朝のランニングと昨日のヨガ
public extension MockHealthData {
    static func workoutSamples(now: Date = Date()) -> [HealthWorkout] {
        let today = Calendar.current.startOfDay(for: now)
        return [
            HealthWorkout(activity: .yoga, start: today.addingTimeInterval(-6 * 3600), duration: 30 * 60, energyKcal: 95, source: "Apple Watch"),
            HealthWorkout(activity: .running, start: today.addingTimeInterval(7 * 3600), duration: 32 * 60, distanceKm: 5.2, energyKcal: 340, source: "Apple Watch"),
        ]
    }

    func workouts(from: Date, to: Date) async throws -> [HealthWorkout] {
        healthWorkouts(Self.workoutSamples(), from: from, to: to)
    }
}

#if os(iOS)
extension HealthWorkout.Activity {
    /// HealthKit の種類から(15 種以外は `other`)
    init(_ type: HKWorkoutActivityType) {
        switch type {
        case .walking: self = .walking
        case .running: self = .running
        case .cycling, .handCycling: self = .cycling
        case .swimming: self = .swimming
        case .hiking: self = .hiking
        case .yoga: self = .yoga
        case .pilates: self = .pilates
        case .socialDance, .cardioDance: self = .dance
        case .elliptical: self = .elliptical
        case .rowing: self = .rowing
        case .stairClimbing, .stairs, .stepTraining: self = .stairClimbing
        case .functionalStrengthTraining: self = .functionalStrengthTraining
        case .traditionalStrengthTraining: self = .traditionalStrengthTraining
        case .highIntensityIntervalTraining: self = .highIntensityIntervalTraining
        case .coreTraining: self = .coreTraining
        default: self = .other
        }
    }
}

extension HealthWorkout {
    /// 距離として足す量(先に見つかったものを使う)
    static let distanceTypes: [HKQuantityTypeIdentifier] = [
        .distanceWalkingRunning, .distanceCycling, .distanceSwimming, .distanceWheelchair, .distanceDownhillSnowSports,
    ]

    init(_ w: HKWorkout) {
        let km = Self.distanceTypes.lazy
            .compactMap { w.statistics(for: HKQuantityType($0))?.sumQuantity()?.doubleValue(for: .meterUnit(with: .kilo)) }
            .first { $0 > 0 }
        let kcal = w.statistics(for: HKQuantityType(.activeEnergyBurned))?.sumQuantity()?.doubleValue(for: .kilocalorie())
        self.init(activity: Activity(w.workoutActivityType), start: w.startDate, duration: w.duration,
                  distanceKm: healthPositive(km), energyKcal: healthPositive(kcal),
                  source: w.sourceRevision.source.name)
    }
}
#endif

/// 実機: HealthKit の `HKWorkout`(読むだけ・許可は `HealthMetric` と同じシートでまとめて求める)
public extension LiveHealthData {
    func workouts(from: Date, to: Date) async throws -> [HealthWorkout] {
        #if os(iOS)
        try await authorizeMetrics()
        let descriptor = HKSampleQueryDescriptor(
            predicates: [.workout(HKQuery.predicateForSamples(withStart: from, end: to))],
            sortDescriptors: [],
            limit: 500
        )
        let samples = try await descriptor.result(for: store)
        return healthWorkouts(samples.map(HealthWorkout.init), from: from, to: to)
        #else
        throw HealthDataError.unavailable
        #endif
    }
}
#endif
