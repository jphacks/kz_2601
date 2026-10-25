// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
#if os(iOS)
import CoreMotion
import HealthKit
#endif

/// `healthStat` / `Field.health`(#167)が読む・書く端末のヘルスデータ。
/// 歩数は CoreMotion の歩数計(capability 不要・直近 7 日)を優先し、使えない端末(シミュレータ等)はヘルスケアの歩数を読む。
/// 体重はヘルスケア(HealthKit)。ホストは `HealthData.source` を差し替えられる(`METAMON_MOCK=1` は固定値)。
public protocol HealthDataSource: Sendable {
    /// 今日を最後とする直近 `days` 日の歩数(古い順・1 日 1 件)。
    func steps(days: Int) async throws -> [HealthDailyValue]
    /// 直近 `days` 日の体重 kg(古い順・記録のある日だけ・その日の最新値)。
    func weights(days: Int) async throws -> [HealthDailyValue]
    /// 体重 kg をヘルスケアに 1 件書き込む。
    func saveWeight(kg: Double, at date: Date) async throws
}

public struct HealthDailyValue: Sendable, Equatable, Identifiable {
    public var day: Date
    public var value: Double
    public var id: Date { day }
    public init(day: Date, value: Double) {
        self.day = day
        self.value = value
    }
}

public enum HealthDataError: LocalizedError, Equatable {
    /// この端末では使えない(シミュレータの歩数計・iPad のヘルスケア等)
    case unavailable
    /// 許可されていない(モーションとフィットネス / ヘルスケア)
    case denied

    public var errorDescription: String? {
        switch self {
        case .unavailable: L("この端末ではヘルスデータを使えません")
        case .denied: L("ヘルスケアへのアクセスが許可されていません")
        }
    }
}

@MainActor
public enum HealthData {
    /// 部品とアプリ本体の保存処理が使う実体。アプリ起動時に 1 回だけ差し替える。
    public static var source: any HealthDataSource = LiveHealthData()
}

/// 1 日の境目(端末のタイムゾーン)で `days` 日分の 0:00 を古い順に返す。
func healthDayStarts(days: Int, now: Date = Date(), calendar: Calendar = .current) -> [Date] {
    let today = calendar.startOfDay(for: now)
    return (0..<max(days, 1)).reversed().compactMap { calendar.date(byAdding: .day, value: -$0, to: today) }
}

/// `METAMON_MOCK=1` / プレビュー用の固定値。今日の歩数 6,240・体重は 7 日で 62.4 → 61.8 kg。
public struct MockHealthData: HealthDataSource {
    public static let stepSeries: [Double] = [7120, 9340, 4210, 8800, 10230, 5600, 6240]
    public static let weightSeries: [Double] = [62.4, 62.3, 62.3, 62.1, 62.0, 61.9, 61.8]

    public init() {}

    public func steps(days: Int) async throws -> [HealthDailyValue] {
        Self.series(Self.stepSeries, days: days)
    }

    public func weights(days: Int) async throws -> [HealthDailyValue] {
        Self.series(Self.weightSeries, days: days)
    }

    public func saveWeight(kg: Double, at date: Date) async throws {}

    static func series(_ values: [Double], days: Int) -> [HealthDailyValue] {
        let starts = healthDayStarts(days: days)
        let tail = Array(values.suffix(starts.count))
        let pad = starts.count - tail.count
        return starts.enumerated().compactMap { i, d in i < pad ? nil : HealthDailyValue(day: d, value: tail[i - pad]) }
    }
}

/// 実機: CoreMotion(歩数)+ HealthKit(歩数の代替・体重)。
public final class LiveHealthData: HealthDataSource, @unchecked Sendable {
    #if os(iOS)
    private let pedometer = CMPedometer()
    private let store = HKHealthStore()
    #endif

    public init() {}

    public func steps(days: Int) async throws -> [HealthDailyValue] {
        #if os(iOS)
        if CMPedometer.isStepCountingAvailable() {
            switch CMPedometer.authorizationStatus() {
            case .denied, .restricted: throw HealthDataError.denied
            default: return try await pedometerSteps(days: days)
            }
        }
        // 歩数計の無い端末(シミュレータ)はヘルスケアの歩数を読む
        return try await healthKitSteps(days: days)
        #else
        throw HealthDataError.unavailable
        #endif
    }

    public func weights(days: Int) async throws -> [HealthDailyValue] {
        #if os(iOS)
        try await authorize()
        let type = HKQuantityType(.bodyMass)
        let starts = healthDayStarts(days: days)
        guard let from = starts.first else { return [] }
        let descriptor = HKSampleQueryDescriptor(
            predicates: [.quantitySample(type: type, predicate: HKQuery.predicateForSamples(withStart: from, end: nil))],
            sortDescriptors: [SortDescriptor(\.startDate, order: .reverse)],
            limit: 200
        )
        let samples = try await descriptor.result(for: store)
        var latestPerDay: [Date: Double] = [:]
        for s in samples { // 新しい順なので最初に見た値がその日の最新
            let day = Calendar.current.startOfDay(for: s.startDate)
            if latestPerDay[day] == nil { latestPerDay[day] = s.quantity.doubleValue(for: .gramUnit(with: .kilo)) }
        }
        return latestPerDay.sorted { $0.key < $1.key }.map { HealthDailyValue(day: $0.key, value: $0.value) }
        #else
        throw HealthDataError.unavailable
        #endif
    }

    public func saveWeight(kg: Double, at date: Date) async throws {
        #if os(iOS)
        try await authorize()
        let type = HKQuantityType(.bodyMass)
        guard store.authorizationStatus(for: type) == .sharingAuthorized else { throw HealthDataError.denied }
        let sample = HKQuantitySample(type: type, quantity: HKQuantity(unit: .gramUnit(with: .kilo), doubleValue: kg), start: date, end: date)
        try await store.save(sample)
        #else
        throw HealthDataError.unavailable
        #endif
    }

    #if os(iOS)
    /// 初回だけ許可ダイアログが出る(決定済みなら何もしない)。読み取りの拒否は OS が隠すので、結果が空になるだけ。
    private func authorize() async throws {
        guard HKHealthStore.isHealthDataAvailable() else { throw HealthDataError.unavailable }
        do {
            try await store.requestAuthorization(toShare: [HKQuantityType(.bodyMass)],
                                                 read: [HKQuantityType(.bodyMass), HKQuantityType(.stepCount)])
        } catch {
            // entitlement が無いビルド等。部品には「使えません」と出す
            throw HealthDataError.unavailable
        }
    }

    private func pedometerSteps(days: Int) async throws -> [HealthDailyValue] {
        let now = Date()
        var out: [HealthDailyValue] = []
        for start in healthDayStarts(days: days, now: now) {
            let end = min(Calendar.current.date(byAdding: .day, value: 1, to: start) ?? now, now)
            let n = try await pedometerCount(from: start, to: end)
            out.append(HealthDailyValue(day: start, value: n))
        }
        return out
    }

    private func pedometerCount(from: Date, to: Date) async throws -> Double {
        try await withCheckedThrowingContinuation { cont in
            pedometer.queryPedometerData(from: from, to: to) { data, error in
                if let error = error as NSError? {
                    if error.domain == CMErrorDomain && error.code == Int(CMErrorMotionActivityNotAuthorized.rawValue) {
                        cont.resume(throwing: HealthDataError.denied)
                    } else {
                        cont.resume(throwing: error)
                    }
                    return
                }
                cont.resume(returning: data?.numberOfSteps.doubleValue ?? 0)
            }
        }
    }

    private func healthKitSteps(days: Int) async throws -> [HealthDailyValue] {
        try await authorize()
        let starts = healthDayStarts(days: days)
        guard let from = starts.first else { return [] }
        let descriptor = HKStatisticsCollectionQueryDescriptor(
            predicate: .quantitySample(type: HKQuantityType(.stepCount), predicate: HKQuery.predicateForSamples(withStart: from, end: nil)),
            options: .cumulativeSum,
            anchorDate: from,
            intervalComponents: DateComponents(day: 1)
        )
        let collection = try await descriptor.result(for: store)
        var byDay: [Date: Double] = [:]
        collection.enumerateStatistics(from: from, to: Date()) { stats, _ in
            byDay[Calendar.current.startOfDay(for: stats.startDate)] = stats.sumQuantity()?.doubleValue(for: .count()) ?? 0
        }
        return starts.map { HealthDailyValue(day: $0, value: byDay[$0] ?? 0) }
    }
    #endif
}
#endif
