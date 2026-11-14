// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
#if os(iOS)
import CoreMotion
import HealthKit
#endif

/// `healthStat` / `Field.health`(#167)が読む・書く端末のヘルスデータ。
/// 歩数は CoreMotion の歩数計(capability 不要・直近 7 日)を優先し、使えない端末(シミュレータ等)はヘルスケアの歩数を読む。
/// 体重はヘルスケア(HealthKit)。睡眠・安静時心拍・距離・消費カロリー・エクササイズ・マインドフルネスは読むだけ(`daily`・#1986)。
/// 値はログ・クラッシュ報告に出さない(要配慮の情報)。ホストは `HealthData.source` を差し替えられる(`METAMON_MOCK=1` は固定値)。
public protocol HealthDataSource: Sendable {
    /// 今日を最後とする直近 `days` 日の歩数(古い順・1 日 1 件)。
    func steps(days: Int) async throws -> [HealthDailyValue]
    /// 直近 `days` 日の体重 kg(古い順・記録のある日だけ・その日の最新値)。
    func weights(days: Int) async throws -> [HealthDailyValue]
    /// 体重 kg をヘルスケアに 1 件書き込む。
    func saveWeight(kg: Double, at date: Date) async throws
    /// 直近 `days` 日の `metric`(古い順・記録のある日だけ・1 日 1 件)。単位は `HealthMetric.unit`。
    func daily(_ metric: HealthMetric, days: Int) async throws -> [HealthDailyValue]
    /// `from`〜`to` の気分(State of Mind・古い順)。`kind` を渡すとその種類だけ。iOS 18 から。
    func moods(from: Date, to: Date, kind: HealthMoodKind?) async throws -> [HealthMoodEntry]
    /// 気分をヘルスケアに 1 件書き込む。iOS 18 から。
    func saveMood(_ entry: HealthMoodEntry) async throws
}

public extension HealthDataSource {
    /// 読む口を持たないデータ源は「使えません」(既存の差し替え先を壊さない)。
    func daily(_ metric: HealthMetric, days: Int) async throws -> [HealthDailyValue] {
        throw HealthDataError.unavailable
    }

    /// 気分の口を持たないデータ源は「使えません」。
    func moods(from: Date, to: Date, kind: HealthMoodKind?) async throws -> [HealthMoodEntry] {
        throw HealthDataError.unavailable
    }

    /// 気分の口を持たないデータ源は「使えません」。
    func saveMood(_ entry: HealthMoodEntry) async throws {
        throw HealthDataError.unavailable
    }
}

/// ヘルスケアから読むだけの値(#1986)。部品・DSL にはまだつながない(#1663 の決定待ち)。
public enum HealthMetric: String, CaseIterable, Sendable {
    /// 睡眠(core / deep / REM / unspecified の合計・起きた日の値)。分
    case sleep
    /// 安静時心拍(その日の平均)。拍/分
    case restingHeartRate
    /// 歩行と走行の距離(合計)。km
    case walkingRunningDistance
    /// 消費カロリー(アクティブ・合計)。kcal
    case activeEnergy
    /// エクササイズの時間(合計)。分
    case exerciseTime
    /// マインドフルネスの時間(合計)。分
    case mindfulness

    public enum Unit: String, Sendable { case minutes, beatsPerMinute, kilometers, kilocalories }

    public var unit: Unit {
        switch self {
        case .sleep, .exerciseTime, .mindfulness: .minutes
        case .restingHeartRate: .beatsPerMinute
        case .walkingRunningDistance: .kilometers
        case .activeEnergy: .kilocalories
        }
    }
}

/// 気分(State of Mind・#2234)の種類。HealthKit の `HKStateOfMind.Kind` と 1 対 1。
public enum HealthMoodKind: String, CaseIterable, Sendable {
    /// その時の感情(「いま どう感じている?」)
    case momentaryEmotion
    /// 1 日の気分(「今日は どう感じていた?」)
    case dailyMood
}

/// 気分に付ける言葉。HealthKit の `HKStateOfMind.Label`(iOS 18 の 38 種)と 1 対 1。
public enum HealthMoodLabel: String, CaseIterable, Sendable {
    case amazed, amused, angry, annoyed, anxious, ashamed, brave, calm, confident, content
    case disappointed, discouraged, disgusted, drained, embarrassed, excited, frustrated
    case grateful, guilty, happy, hopeful, hopeless, indifferent, irritated, jealous, joyful
    case lonely, overwhelmed, passionate, peaceful, proud, relieved, sad, satisfied, scared
    case stressed, surprised, worried
}

/// 気分 1 件。`valence` は -1(とても不快)〜+1(とても快)。
/// 中身は要配慮の情報なので `description` にも出さない(ログ・クラッシュ報告に漏らさないため)。
public struct HealthMoodEntry: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var date: Date
    public var kind: HealthMoodKind
    public var valence: Double
    public var labels: [HealthMoodLabel]
    public var id: Date { date }

    public init(date: Date, kind: HealthMoodKind, valence: Double, labels: [HealthMoodLabel] = []) {
        self.date = date
        self.kind = kind
        self.valence = valence
        self.labels = labels
    }

    public var description: String { "HealthMoodEntry(非表示)" }
    public var debugDescription: String { description }
}

/// valence が書き込める範囲(-1〜+1)に入っているか。NaN・無限は入っていないとみなす。
func healthValenceIsInRange(_ valence: Double) -> Bool {
    valence.isFinite && valence >= -1 && valence <= 1
}

/// 文字列から気分の言葉へ。知らない言葉は落とし、重なりは最初の 1 つだけ残す(並びは渡された順)。
func healthMoodLabels(fromRawValues raws: [String]) -> [HealthMoodLabel] {
    var seen: Set<HealthMoodLabel> = []
    return raws.compactMap(HealthMoodLabel.init(rawValue:)).filter { seen.insert($0).inserted }
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
    /// iOS 18 より前の OS では気分(State of Mind)を使えない
    case requiresIOS18
    /// 値が書き込める範囲の外(気分の valence は -1〜+1)
    case outOfRange

    public var errorDescription: String? {
        switch self {
        case .unavailable: L("この端末ではヘルスデータを使えません")
        case .denied: L("ヘルスケアへのアクセスが許可されていません")
        case .requiresIOS18: L("気分の記録は iOS 18 以降で使えます")
        case .outOfRange: L("気分の値が範囲の外です")
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

/// 睡眠の 1 区間(HealthKit の `sleepAnalysis` を写したもの。HealthKit に依存しない形でテストする)。
struct HealthSleepSample: Sendable, Equatable {
    enum Stage: Sendable, Equatable { case inBed, awake, asleep }
    var start: Date
    var end: Date
    var stage: Stage
}

/// 区間の和集合(古い順・重なりと接する区間をまとめる)。Apple Watch と iPhone の重なる記録を二重に数えないため。
func healthUnion(_ intervals: [(start: Date, end: Date)]) -> [(start: Date, end: Date)] {
    var out: [(start: Date, end: Date)] = []
    for i in intervals.filter({ $0.end > $0.start }).sorted(by: { $0.start < $1.start }) {
        if let last = out.last, i.start <= last.end {
            out[out.count - 1].end = max(last.end, i.end)
        } else {
            out.append(i)
        }
    }
    return out
}

/// 眠っていた時間(分)を起きた日ごとに集計する。
/// - 眠っていた段階だけ数える(inBed・awake は数えない)。重なる区間は 1 回だけ数える。
/// - 間が `sessionGap` 未満の区間は同じ眠り(夜中に起きた等)とみなし、最後に起きた時刻の日に入れる。
/// - 返すのは `dayStarts`(古い順の 0:00)の範囲で、眠りの記録がある日だけ。
func sleepMinutesByWakeDay(_ samples: [HealthSleepSample], dayStarts: [Date],
                           sessionGap: TimeInterval = 3 * 3600,
                           calendar: Calendar = .current) -> [HealthDailyValue] {
    let blocks = healthUnion(samples.filter { $0.stage == .asleep }.map { ($0.start, $0.end) })
    var byDay: [Date: TimeInterval] = [:]
    var session: (end: Date, seconds: TimeInterval)?
    func close() {
        guard let s = session else { return }
        byDay[calendar.startOfDay(for: s.end), default: 0] += s.seconds
    }
    for b in blocks {
        if let s = session, b.start.timeIntervalSince(s.end) < sessionGap {
            session = (b.end, s.seconds + b.end.timeIntervalSince(b.start))
        } else {
            close()
            session = (b.end, b.end.timeIntervalSince(b.start))
        }
    }
    close()
    let wanted = Set(dayStarts)
    return byDay.filter { wanted.contains($0.key) }
        .sorted { $0.key < $1.key }
        .map { HealthDailyValue(day: $0.key, value: $0.value / 60) }
}

/// `METAMON_MOCK=1` / プレビュー用の固定値。今日の歩数 6,240・体重は 7 日で 62.4 → 61.8 kg。
public struct MockHealthData: HealthDataSource {
    public static let stepSeries: [Double] = [7120, 9340, 4210, 8800, 10230, 5600, 6240]
    public static let weightSeries: [Double] = [62.4, 62.3, 62.3, 62.1, 62.0, 61.9, 61.8]
    /// `daily(_:days:)` の固定の並び(7 日・古い順)。単位は `HealthMetric.unit`。
    public static let dailySeries: [HealthMetric: [Double]] = [
        .sleep: [412, 388, 455, 401, 362, 498, 431],
        .restingHeartRate: [58, 60, 57, 59, 61, 56, 58],
        .walkingRunningDistance: [5.1, 6.8, 3.0, 6.3, 7.4, 4.0, 4.5],
        .activeEnergy: [320, 410, 240, 380, 455, 290, 335],
        .exerciseTime: [22, 35, 12, 30, 41, 18, 25],
        .mindfulness: [5, 3, 10, 5, 8, 15, 5],
    ]

    public init() {}

    public func steps(days: Int) async throws -> [HealthDailyValue] {
        Self.series(Self.stepSeries, days: days)
    }

    public func weights(days: Int) async throws -> [HealthDailyValue] {
        Self.series(Self.weightSeries, days: days)
    }

    public func saveWeight(kg: Double, at date: Date) async throws {}

    public func daily(_ metric: HealthMetric, days: Int) async throws -> [HealthDailyValue] {
        Self.series(Self.dailySeries[metric] ?? [], days: days)
    }

    /// 固定の気分(古い順)。今から何時間前かで置く。
    static let moodOffsets: [(hoursAgo: Double, kind: HealthMoodKind, valence: Double, labels: [HealthMoodLabel])] = [
        (50, .dailyMood, -0.2, [.anxious, .drained]),
        (26, .momentaryEmotion, 0.6, [.happy, .grateful]),
        (3, .momentaryEmotion, 0.25, [.calm]),
        (1, .dailyMood, 0.4, [.content, .satisfied]),
    ]

    public func moods(from: Date, to: Date, kind: HealthMoodKind?) async throws -> [HealthMoodEntry] {
        let now = Date()
        return Self.moodOffsets
            .map { HealthMoodEntry(date: now.addingTimeInterval(-$0.hoursAgo * 3600), kind: $0.kind, valence: $0.valence, labels: $0.labels) }
            .filter { $0.date >= from && $0.date <= to && (kind == nil || $0.kind == kind) }
    }

    public func saveMood(_ entry: HealthMoodEntry) async throws {
        guard healthValenceIsInRange(entry.valence) else { throw HealthDataError.outOfRange }
    }

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
            // \.startDate のキーパスは Sendable でない(Swift 6 で error)。同じ並び(startDate の降順)を NSSortDescriptor から作る
            sortDescriptors: SortDescriptor(NSSortDescriptor(key: HKSampleSortIdentifierStartDate, ascending: false), comparing: HKQuantitySample.self).map { [$0] } ?? [],
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

    public func daily(_ metric: HealthMetric, days: Int) async throws -> [HealthDailyValue] {
        #if os(iOS)
        let starts = healthDayStarts(days: days)
        guard let from = starts.first else { return [] }
        switch metric {
        case .sleep:
            let type = HKCategoryType(.sleepAnalysis)
            try await authorizeMetrics()
            // 前の晩に寝始めた眠りも拾う(起きた日が範囲内なら数える)
            let since = Calendar.current.date(byAdding: .day, value: -1, to: from) ?? from
            let samples = try await categorySamples(type, since: since)
            return sleepMinutesByWakeDay(samples.map { HealthSleepSample(start: $0.startDate, end: $0.endDate, stage: Self.sleepStage($0.value)) },
                                         dayStarts: starts)
        case .mindfulness:
            let type = HKCategoryType(.mindfulSession)
            try await authorizeMetrics()
            let blocks = healthUnion(try await categorySamples(type, since: from).map { ($0.startDate, $0.endDate) })
            var byDay: [Date: Double] = [:]
            for b in blocks { byDay[Calendar.current.startOfDay(for: b.start), default: 0] += b.end.timeIntervalSince(b.start) / 60 }
            return starts.compactMap { d in byDay[d].map { HealthDailyValue(day: d, value: $0) } }
        case .restingHeartRate:
            return try await statistics(.restingHeartRate, unit: .count().unitDivided(by: .minute()), average: true, starts: starts)
        case .walkingRunningDistance:
            return try await statistics(.distanceWalkingRunning, unit: .meterUnit(with: .kilo), average: false, starts: starts)
        case .activeEnergy:
            return try await statistics(.activeEnergyBurned, unit: .kilocalorie(), average: false, starts: starts)
        case .exerciseTime:
            return try await statistics(.appleExerciseTime, unit: .minute(), average: false, starts: starts)
        }
        #else
        throw HealthDataError.unavailable
        #endif
    }

    public func moods(from: Date, to: Date, kind: HealthMoodKind?) async throws -> [HealthMoodEntry] {
        #if os(iOS)
        guard #available(iOS 18.0, *) else { throw HealthDataError.requiresIOS18 }
        return try await readMoods(from: from, to: to, kind: kind)
        #else
        throw HealthDataError.unavailable
        #endif
    }

    public func saveMood(_ entry: HealthMoodEntry) async throws {
        #if os(iOS)
        guard #available(iOS 18.0, *) else { throw HealthDataError.requiresIOS18 }
        guard healthValenceIsInRange(entry.valence) else { throw HealthDataError.outOfRange }
        try await writeMood(entry)
        #else
        throw HealthDataError.unavailable
        #endif
    }

    #if os(iOS)
    /// HealthMetric で読む値(すべて)。許可は 1 枚のシートでまとめて求める。
    private static let metricReadTypes: Set<HKObjectType> = [
        HKCategoryType(.sleepAnalysis),
        HKCategoryType(.mindfulSession),
        HKQuantityType(.restingHeartRate),
        HKQuantityType(.distanceWalkingRunning),
        HKQuantityType(.activeEnergyBurned),
        HKQuantityType(.appleExerciseTime),
    ]

    /// 読むだけの値の許可。HealthMetric の全種類を 1 回で求める(書き込みの許可は求めない)。
    private func authorizeMetrics() async throws {
        guard HKHealthStore.isHealthDataAvailable() else { throw HealthDataError.unavailable }
        do {
            try await store.requestAuthorization(toShare: [], read: Self.metricReadTypes)
        } catch {
            throw HealthDataError.unavailable
        }
    }

    private static func sleepStage(_ raw: Int) -> HealthSleepSample.Stage {
        switch HKCategoryValueSleepAnalysis(rawValue: raw) {
        case .inBed: .inBed
        case .awake: .awake
        case .asleepUnspecified, .asleepCore, .asleepDeep, .asleepREM: .asleep
        default: .awake // 知らない値は眠りに数えない
        }
    }

    private func categorySamples(_ type: HKCategoryType, since: Date) async throws -> [HKCategorySample] {
        let descriptor = HKSampleQueryDescriptor(
            predicates: [.categorySample(type: type, predicate: HKQuery.predicateForSamples(withStart: since, end: nil))],
            sortDescriptors: [],
            limit: HKObjectQueryNoLimit
        )
        return try await descriptor.result(for: store)
    }

    /// 1 日ごとの合計 / 平均(ソースの重なりは HealthKit がまとめる)。記録の無い日は返さない。
    private func statistics(_ id: HKQuantityTypeIdentifier, unit: HKUnit, average: Bool, starts: [Date]) async throws -> [HealthDailyValue] {
        guard let from = starts.first else { return [] }
        let type = HKQuantityType(id)
        try await authorizeMetrics()
        let descriptor = HKStatisticsCollectionQueryDescriptor(
            predicate: .quantitySample(type: type, predicate: HKQuery.predicateForSamples(withStart: from, end: nil)),
            options: average ? .discreteAverage : .cumulativeSum,
            anchorDate: from,
            intervalComponents: DateComponents(day: 1)
        )
        let collection = try await descriptor.result(for: store)
        var byDay: [Date: Double] = [:]
        collection.enumerateStatistics(from: from, to: Date()) { stats, _ in
            let q = average ? stats.averageQuantity() : stats.sumQuantity()
            if let q { byDay[Calendar.current.startOfDay(for: stats.startDate)] = q.doubleValue(for: unit) }
        }
        return starts.compactMap { d in byDay[d].map { HealthDailyValue(day: d, value: $0) } }
    }

    /// 気分を読む許可だけを求める(書き込みの許可は求めない)。
    @available(iOS 18.0, *)
    private func authorizeMoodRead() async throws {
        guard HKHealthStore.isHealthDataAvailable() else { throw HealthDataError.unavailable }
        do {
            try await store.requestAuthorization(toShare: [], read: [HKObjectType.stateOfMindType()])
        } catch {
            throw HealthDataError.unavailable
        }
    }

    /// 気分を書く許可だけを求める(読みの許可は求めない)。拒否されていれば `denied`。
    @available(iOS 18.0, *)
    private func authorizeMoodWrite() async throws {
        guard HKHealthStore.isHealthDataAvailable() else { throw HealthDataError.unavailable }
        let type = HKObjectType.stateOfMindType()
        do {
            try await store.requestAuthorization(toShare: [type], read: [])
        } catch {
            throw HealthDataError.unavailable
        }
        guard store.authorizationStatus(for: type) == .sharingAuthorized else { throw HealthDataError.denied }
    }

    @available(iOS 18.0, *)
    private func readMoods(from: Date, to: Date, kind: HealthMoodKind?) async throws -> [HealthMoodEntry] {
        try await authorizeMoodRead()
        let descriptor = HKSampleQueryDescriptor(
            predicates: [.stateOfMind(HKQuery.predicateForSamples(withStart: from, end: to))],
            // \.startDate のキーパスは Sendable でない(Swift 6 で error)。同じ並び(startDate の昇順)を NSSortDescriptor から作る
            sortDescriptors: SortDescriptor(NSSortDescriptor(key: HKSampleSortIdentifierStartDate, ascending: true), comparing: HKStateOfMind.self).map { [$0] } ?? [],
            limit: HKObjectQueryNoLimit
        )
        return try await descriptor.result(for: store).compactMap { sample in
            let k = HealthMoodKind(sample.kind)
            guard kind == nil || k == kind else { return nil }
            return HealthMoodEntry(date: sample.startDate, kind: k, valence: sample.valence,
                                   labels: sample.labels.compactMap(HealthMoodLabel.init))
        }
    }

    @available(iOS 18.0, *)
    private func writeMood(_ entry: HealthMoodEntry) async throws {
        try await authorizeMoodWrite()
        let sample = HKStateOfMind(date: entry.date, kind: entry.kind.healthKitKind, valence: entry.valence,
                                   labels: entry.labels.map(\.healthKitLabel), associations: [])
        try await store.save(sample)
    }

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

#if os(iOS)
@available(iOS 18.0, *)
extension HealthMoodKind {
    init(_ kind: HKStateOfMind.Kind) {
        switch kind {
        case .dailyMood: self = .dailyMood
        case .momentaryEmotion: self = .momentaryEmotion
        @unknown default: self = .momentaryEmotion
        }
    }

    var healthKitKind: HKStateOfMind.Kind {
        switch self {
        case .momentaryEmotion: .momentaryEmotion
        case .dailyMood: .dailyMood
        }
    }
}

@available(iOS 18.0, *)
extension HealthMoodLabel {
    /// 知らない言葉(将来の OS が足した値)は落とす。
    init?(_ label: HKStateOfMind.Label) {
        guard let match = Self.allCases.first(where: { $0.healthKitLabel == label }) else { return nil }
        self = match
    }

    var healthKitLabel: HKStateOfMind.Label {
        switch self {
        case .amazed: .amazed
        case .amused: .amused
        case .angry: .angry
        case .annoyed: .annoyed
        case .anxious: .anxious
        case .ashamed: .ashamed
        case .brave: .brave
        case .calm: .calm
        case .confident: .confident
        case .content: .content
        case .disappointed: .disappointed
        case .discouraged: .discouraged
        case .disgusted: .disgusted
        case .drained: .drained
        case .embarrassed: .embarrassed
        case .excited: .excited
        case .frustrated: .frustrated
        case .grateful: .grateful
        case .guilty: .guilty
        case .happy: .happy
        case .hopeful: .hopeful
        case .hopeless: .hopeless
        case .indifferent: .indifferent
        case .irritated: .irritated
        case .jealous: .jealous
        case .joyful: .joyful
        case .lonely: .lonely
        case .overwhelmed: .overwhelmed
        case .passionate: .passionate
        case .peaceful: .peaceful
        case .proud: .proud
        case .relieved: .relieved
        case .sad: .sad
        case .satisfied: .satisfied
        case .scared: .scared
        case .stressed: .stressed
        case .surprised: .surprised
        case .worried: .worried
        }
    }
}
#endif
#endif
