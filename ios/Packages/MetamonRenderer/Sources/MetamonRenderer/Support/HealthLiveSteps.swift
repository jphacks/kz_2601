// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import Foundation
#if os(iOS)
import CoreMotion
import HealthKit
#endif

/// `healthStat` の `live`(#3694)が使う、開いている間の歩数の更新の口。
/// - 歩数計(CoreMotion)は購読してからの累計歩数を流す(`from` 以降・減らない)。
/// - ヘルスケア(HealthKit)は歩数に変化があった時に合図を流す(歩数計の無い端末・Apple Watch の同期)。
/// 既定の一発読み(`HealthDataSource.steps`)はそのままで、`live: true` のカードが画面に出ている間だけ使う。
public protocol HealthLiveSource: Sendable {
    /// `from` 以降の歩数計の累計を流す。使えない端末では空の流れ(すぐ終わる)。
    func pedometerUpdates(from: Date) -> AsyncStream<Double>
    /// ヘルスケアの歩数が変わった合図。使えない端末では空の流れ。
    func stepCountChanges() -> AsyncStream<Void>
}

/// 画面に出す歩数 = 基準値(一発読み)+ 購読してからの歩数計の増分。純粋な計算(テストはここを見る)。
public struct HealthLiveSteps: Equatable, Sendable {
    /// 一発読みの今日の歩数(HealthKit か歩数計)
    public private(set) var baseline: Double
    /// 購読してからの歩数計の累計(減らない)
    public private(set) var delta: Double = 0

    public init(baseline: Double) { self.baseline = max(baseline, 0) }

    public var value: Double { baseline + delta }

    /// 基準値を取り直した(ヘルスケアの合図・日付が変わった)。増分は基準値に含まれるので 0 に戻す。
    public mutating func rebase(_ baseline: Double) {
        self.baseline = max(baseline, 0)
        delta = 0
    }

    /// 歩数計の累計(購読してからの歩数)。負や NaN は 0 として扱う。
    public mutating func pedometer(cumulative: Double) {
        delta = cumulative.isFinite ? max(cumulative, 0) : 0
    }
}

/// `live` の購読の状態機械。`start()` で歩数計とヘルスケアの合図を購読し、`stop()` で両方やめる。
/// ヘルスケアの合図と日付の切り替わりでは基準値を取り直し、歩数計を購読し直す(増分の二重数えを防ぐ)。
/// 値の変化は `onChange` で(MainActor)。HealthKit に依存しないので偽の源でテストできる。
@MainActor
public final class HealthLiveStepsMonitor {
    public typealias Baseline = @Sendable () async throws -> Double

    private let source: any HealthLiveSource
    private let refresh: Baseline
    private let calendar: Calendar
    public private(set) var steps: HealthLiveSteps
    public private(set) var isRunning = false
    /// 画面に出す値が変わった時(基準値の取り直しも含む)
    public var onChange: ((Double) -> Void)?
    private var pedometerTask: Task<Void, Never>?
    private var observerTask: Task<Void, Never>?
    private var day = Date.distantPast

    /// - Parameters:
    ///   - baseline: 一発読みで出した今日の歩数
    ///   - source: 歩数計とヘルスケアの合図
    ///   - refresh: 基準値を取り直す(合図が来た時・日付が変わった時)
    public init(baseline: Double, source: any HealthLiveSource, calendar: Calendar = .current, refresh: @escaping Baseline) {
        self.steps = HealthLiveSteps(baseline: baseline)
        self.source = source
        self.calendar = calendar
        self.refresh = refresh
    }

    public func start(now: Date = Date()) {
        guard !isRunning else { return }
        isRunning = true
        day = calendar.startOfDay(for: now)
        subscribePedometer(from: now)
        observerTask = Task { [weak self, source] in
            for await _ in source.stepCountChanges() {
                guard !Task.isCancelled, let self else { return }
                await self.rebase()
            }
        }
    }

    public func stop() {
        pedometerTask?.cancel()
        pedometerTask = nil
        observerTask?.cancel()
        observerTask = nil
        isRunning = false
    }

    /// 歩数計の累計を 1 つ受ける。日付が変わっていたら基準値を取り直す(昨日の増分を今日に足さない)。
    func apply(pedometer cumulative: Double, now: Date = Date()) {
        guard isRunning else { return }
        if calendar.startOfDay(for: now) != day {
            Task { await rebase(now: now) }
            return
        }
        steps.pedometer(cumulative: cumulative)
        onChange?(steps.value)
    }

    /// 基準値を取り直し、歩数計を購読し直す。取り直せない時(許可が無い等)は今の値のまま。
    func rebase(now: Date = Date()) async {
        guard isRunning else { return }
        guard let value = try? await refresh(), isRunning else { return }
        day = calendar.startOfDay(for: now)
        steps.rebase(value)
        subscribePedometer(from: now)
        onChange?(steps.value)
    }

    private func subscribePedometer(from: Date) {
        pedometerTask?.cancel()
        pedometerTask = Task { [weak self, source] in
            for await n in source.pedometerUpdates(from: from) {
                guard !Task.isCancelled, let self else { return }
                self.apply(pedometer: n)
            }
        }
    }
}

/// 歩数計もヘルスケアも無い(Mac・テストの既定)。
public struct UnavailableHealthLiveSource: HealthLiveSource {
    public init() {}
    public func pedometerUpdates(from: Date) -> AsyncStream<Double> { AsyncStream { $0.finish() } }
    public func stepCountChanges() -> AsyncStream<Void> { AsyncStream { $0.finish() } }
}

/// `METAMON_MOCK=1` / プレビュー用: `interval` ごとに 1 歩ずつ増える。
public struct MockHealthLiveSource: HealthLiveSource {
    public var interval: Duration
    public init(interval: Duration = .seconds(1.5)) { self.interval = interval }

    public func pedometerUpdates(from: Date) -> AsyncStream<Double> {
        let interval = self.interval
        return AsyncStream { cont in
            let task = Task {
                var n = 0.0
                while !Task.isCancelled {
                    try? await Task.sleep(for: interval)
                    guard !Task.isCancelled else { break }
                    n += 1
                    cont.yield(n)
                }
                cont.finish()
            }
            cont.onTermination = { _ in task.cancel() }
        }
    }

    public func stepCountChanges() -> AsyncStream<Void> { AsyncStream { $0.finish() } }
}

/// 実機: CoreMotion の `CMPedometer.startUpdates` と HealthKit の `HKObserverQuery`(歩数)。
/// 流れの収集をやめる(画面を離れる・背面に回る)と更新を止める。値はログに出さない。
public final class LiveHealthLiveSource: HealthLiveSource, @unchecked Sendable {
    #if os(iOS)
    private let store = HKHealthStore()
    #endif

    public init() {}

    public func pedometerUpdates(from: Date) -> AsyncStream<Double> {
        #if os(iOS)
        guard CMPedometer.isStepCountingAvailable() else { return AsyncStream { $0.finish() } }
        switch CMPedometer.authorizationStatus() {
        case .denied, .restricted: return AsyncStream { $0.finish() }
        default: break
        }
        let pedometer = PedometerBox()
        return AsyncStream { cont in
            pedometer.value.startUpdates(from: from) { data, error in
                guard error == nil, let data else { return }
                cont.yield(data.numberOfSteps.doubleValue)
            }
            cont.onTermination = { _ in pedometer.value.stopUpdates() }
        }
        #else
        return AsyncStream { $0.finish() }
        #endif
    }

    public func stepCountChanges() -> AsyncStream<Void> {
        #if os(iOS)
        guard HKHealthStore.isHealthDataAvailable() else { return AsyncStream { $0.finish() } }
        let store = self.store
        return AsyncStream { cont in
            let query = HKObserverQuery(sampleType: HKQuantityType(.stepCount), predicate: nil) { _, completion, error in
                if error == nil { cont.yield(()) }
                completion()
            }
            let box = QueryBox(query)
            store.execute(query)
            cont.onTermination = { _ in store.stop(box.value) }
        }
        #else
        return AsyncStream { $0.finish() }
        #endif
    }

    #if os(iOS)
    private final class PedometerBox: @unchecked Sendable {
        let value = CMPedometer()
    }

    private final class QueryBox: @unchecked Sendable {
        let value: HKObserverQuery
        init(_ value: HKObserverQuery) { self.value = value }
    }
    #endif
}
#endif
