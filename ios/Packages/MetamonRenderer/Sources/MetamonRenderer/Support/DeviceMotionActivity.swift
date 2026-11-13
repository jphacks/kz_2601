// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(CoreMotion を持ち込まない・SensorData と同じ)。
#if !APP_STORE_REVIEW
import Foundation
#if os(iOS)
import CoreMotion
#endif

/// 今の動き(歩く・走る・自転車・乗り物・止まっている)を読む口(#2225・親 #1822)。
/// `DeviceCalendarSource` と同じ形: protocol + 実体(`LiveDeviceMotionActivity`)+ 偽物(`FakeDeviceMotionActivity`)。
/// DSL の部品・式・AI の決まり・`supportedFeatures` の申告はここでは足さない(決定待ち)。
public protocol DeviceMotionActivitySource: Sendable {
    /// この端末に動きの判定があるか(シミュレータ・M シリーズの無い機種は無い)
    var isAvailable: Bool { get }
    /// 許可の状態。`requesting: true` なら未決定のときに OS の許可ダイアログを出す。
    func authorization(requesting: Bool) async -> DeviceMotionActivityAccess
    /// 今の動き。まだ判定が出ていなければ nil(許可が無ければ `notAuthorized`)。
    func current() async throws -> DeviceMotionActivityReading?
    /// `from..<to` の履歴(始まりの早い順)。期間は `DeviceMotionActivityLimits` で切り、件数も上限で切る。
    func history(from: Date, to: Date) async throws -> [DeviceMotionActivityReading]
    /// 動きが変わるたびに流す。購読を取り消すと判定を止める。
    func updates() -> AsyncStream<DeviceMotionActivityReading>
}

/// DSL 側で扱う動きの種類。`CMMotionActivity` の複数のフラグを 1 つに決めた結果。
public enum DeviceMotionActivityKind: String, Sendable, CaseIterable {
    case walking
    case running
    case cycling
    case automotive
    case stationary
    /// 判定できない・信頼度が足りない
    case unknown
}

/// `CMMotionConfidence` を自前の値にしたもの。
public enum DeviceMotionConfidence: String, Sendable, CaseIterable, Comparable {
    case low
    case medium
    case high

    private var rank: Int {
        switch self {
        case .low: 0
        case .medium: 1
        case .high: 2
        }
    }

    public static func < (a: Self, b: Self) -> Bool { a.rank < b.rank }
}

/// モーションとフィットネスの許可の状態。
public enum DeviceMotionActivityAccess: String, Sendable, Equatable {
    case notDetermined
    case denied
    case restricted
    case authorized

    public var canRead: Bool { self == .authorized }
}

public enum DeviceMotionActivityError: LocalizedError, Equatable {
    case notAuthorized(DeviceMotionActivityAccess)
    /// この端末に動きの判定が無い
    case unavailable

    public var errorDescription: String? {
        switch self {
        case .notAuthorized: L("モーションとフィットネスへのアクセスが許可されていません")
        case .unavailable: L("この端末では使えません")
        }
    }
}

/// `CMMotionActivity` 相当の入力。`CMMotionActivity` は自分で作れないので、
/// フラグだけを持つこの型に移してから純関数で種類を決める(ここが単体テストの対象)。
public struct DeviceMotionActivityFlags: Sendable, Equatable {
    public var stationary: Bool
    public var walking: Bool
    public var running: Bool
    public var automotive: Bool
    public var cycling: Bool
    /// OS が「判定できない」と言っている
    public var unknown: Bool
    public var confidence: DeviceMotionConfidence
    /// この動きが始まった時刻
    public var start: Date

    public init(stationary: Bool = false, walking: Bool = false, running: Bool = false,
                automotive: Bool = false, cycling: Bool = false, unknown: Bool = false,
                confidence: DeviceMotionConfidence = .high, start: Date = Date()) {
        self.stationary = stationary
        self.walking = walking
        self.running = running
        self.automotive = automotive
        self.cycling = cycling
        self.unknown = unknown
        self.confidence = confidence
        self.start = start
    }
}

/// 1 回分の読み取り結果。
public struct DeviceMotionActivityReading: Sendable, Equatable {
    /// DSL に渡す種類。信頼度 low は `.unknown` にしてある。
    public var kind: DeviceMotionActivityKind
    /// 信頼度を無視してフラグだけで決めた種類(実機での確認・デバッグ用)
    public var rawKind: DeviceMotionActivityKind
    public var confidence: DeviceMotionConfidence
    public var start: Date

    public init(kind: DeviceMotionActivityKind, rawKind: DeviceMotionActivityKind,
                confidence: DeviceMotionConfidence, start: Date) {
        self.kind = kind
        self.rawKind = rawKind
        self.confidence = confidence
        self.start = start
    }
}

/// 読む期間・件数の上限。超えた分は切る(エラーにしない)。
public enum DeviceMotionActivityLimits {
    /// CoreMotion が持っている履歴はおよそ 7 日
    public static let daysBack = 7
    /// 1 回で返す件数
    public static let maxItems = 200
    /// 今の動きを待つ秒数(これを過ぎたら nil)
    public static let currentTimeout: TimeInterval = 3

    /// `from..<to` を「今 - daysBack 日」〜「今」に収める。空なら nil。
    public static func clamp(from: Date, to: Date, now: Date = Date()) -> DateInterval? {
        guard let lower = Calendar.current.date(byAdding: .day, value: -daysBack, to: now) else { return nil }
        let start = max(from, lower)
        let end = min(to, now)
        guard start < end else { return nil }
        return DateInterval(start: start, end: end)
    }
}

// MARK: - 変換の純関数(単体テストの対象)

/// `CMMotionActivity` は複数のフラグが同時に立つ(電車で歩くと walking と automotive が両方 true)。
/// どれを 1 つ選ぶかをここで決める。
public enum DeviceMotionActivityMapping {
    /// 強い順。自分の体の動きを、乗り物・止まっているより先に取る。
    /// - 自転車・走る・歩くは「体を動かしている」ので、同時に立ったら乗り物より先(電車で歩く → walking)。
    /// - 乗り物は止まっているより先(信号待ちで automotive と stationary が両方立つ → automotive)。
    /// - 自転車 → 走る → 歩くの順は、速い(= 特定しにくい)方を先に取る。自転車は誤って立ちにくい。
    public static let priority: [DeviceMotionActivityKind] = [.cycling, .running, .walking, .automotive, .stationary]

    /// 信頼度を見ずにフラグだけで決める。どれも立っていない・`unknown` だけなら `.unknown`。
    public static func rawKind(from flags: DeviceMotionActivityFlags) -> DeviceMotionActivityKind {
        priority.first { isSet($0, in: flags) } ?? .unknown
    }

    /// DSL に渡す種類。信頼度 low は当てにならない(止まっていても walking が立つ)ので `.unknown` にする。
    public static func kind(from flags: DeviceMotionActivityFlags) -> DeviceMotionActivityKind {
        flags.confidence == .low ? .unknown : rawKind(from: flags)
    }

    public static func reading(from flags: DeviceMotionActivityFlags) -> DeviceMotionActivityReading {
        DeviceMotionActivityReading(kind: kind(from: flags), rawKind: rawKind(from: flags),
                                    confidence: flags.confidence, start: flags.start)
    }

    /// 期間で絞り、始まりの早い順に並べて上限で切る。
    public static func history(_ all: [DeviceMotionActivityReading], in range: DateInterval) -> [DeviceMotionActivityReading] {
        let hits = all.filter { range.start <= $0.start && $0.start < range.end }
        let sorted = hits.sorted { $0.start < $1.start }
        return Array(sorted.prefix(DeviceMotionActivityLimits.maxItems))
    }

    private static func isSet(_ kind: DeviceMotionActivityKind, in flags: DeviceMotionActivityFlags) -> Bool {
        switch kind {
        case .cycling: flags.cycling
        case .running: flags.running
        case .walking: flags.walking
        case .automotive: flags.automotive
        case .stationary: flags.stationary
        case .unknown: false
        }
    }
}

@MainActor
public enum DeviceMotionActivity {
    /// 読む口の実体。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeDeviceMotionActivity.sample`)。
    public static var source: any DeviceMotionActivitySource = {
        #if os(iOS)
        LiveDeviceMotionActivity()
        #else
        FakeDeviceMotionActivity(isAvailable: false)
        #endif
    }()
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。許可の状態と流す動きを外から決められる。
public struct FakeDeviceMotionActivity: DeviceMotionActivitySource {
    public var isAvailable: Bool
    public var access: DeviceMotionActivityAccess
    /// `requesting: true` で未決定の時に、ダイアログで選ばれたことにする状態
    public var grantOnRequest: DeviceMotionActivityAccess
    public var all: [DeviceMotionActivityFlags]
    public var now: @Sendable () -> Date

    public init(isAvailable: Bool = true,
                access: DeviceMotionActivityAccess = .authorized,
                grantOnRequest: DeviceMotionActivityAccess = .authorized,
                all: [DeviceMotionActivityFlags] = [],
                now: @escaping @Sendable () -> Date = { Date() }) {
        self.isAvailable = isAvailable
        self.access = access
        self.grantOnRequest = grantOnRequest
        self.all = all
        self.now = now
    }

    public func authorization(requesting: Bool) async -> DeviceMotionActivityAccess {
        requesting && access == .notDetermined ? grantOnRequest : access
    }

    public func current() async throws -> DeviceMotionActivityReading? {
        try check()
        return all.max { $0.start < $1.start }.map(DeviceMotionActivityMapping.reading)
    }

    public func history(from: Date, to: Date) async throws -> [DeviceMotionActivityReading] {
        try check()
        guard let range = DeviceMotionActivityLimits.clamp(from: from, to: to, now: now()) else { return [] }
        return DeviceMotionActivityMapping.history(all.map(DeviceMotionActivityMapping.reading), in: range)
    }

    public func updates() -> AsyncStream<DeviceMotionActivityReading> {
        let readings = all.sorted { $0.start < $1.start }.map(DeviceMotionActivityMapping.reading)
        let allowed = isAvailable && access.canRead
        return AsyncStream { cont in
            if allowed { readings.forEach { cont.yield($0) } }
            cont.finish()
        }
    }

    private func check() throws {
        guard isAvailable else { throw DeviceMotionActivityError.unavailable }
        guard access.canRead else { throw DeviceMotionActivityError.notAuthorized(access) }
    }

    /// `METAMON_MOCK=1` の固定の中身(今を基準に作る)。
    public static var sample: FakeDeviceMotionActivity {
        let now = Date()
        return FakeDeviceMotionActivity(all: [
            DeviceMotionActivityFlags(stationary: true, confidence: .high, start: now.addingTimeInterval(-1800)),
            DeviceMotionActivityFlags(walking: true, confidence: .medium, start: now.addingTimeInterval(-1200)),
            DeviceMotionActivityFlags(walking: true, automotive: true, confidence: .medium, start: now.addingTimeInterval(-900)),
            DeviceMotionActivityFlags(automotive: true, confidence: .high, start: now.addingTimeInterval(-600)),
            DeviceMotionActivityFlags(walking: true, confidence: .high, start: now.addingTimeInterval(-60)),
        ], now: { now })
    }
}

// MARK: - 実体

#if os(iOS)
extension DeviceMotionConfidence {
    init(_ c: CMMotionActivityConfidence) {
        switch c {
        case .low: self = .low
        case .medium: self = .medium
        case .high: self = .high
        @unknown default: self = .low
        }
    }
}

extension DeviceMotionActivityAccess {
    init(_ status: CMAuthorizationStatus) {
        switch status {
        case .notDetermined: self = .notDetermined
        case .denied: self = .denied
        case .restricted: self = .restricted
        case .authorized: self = .authorized
        @unknown default: self = .restricted
        }
    }
}

extension DeviceMotionActivityFlags {
    init(_ a: CMMotionActivity) {
        self.init(stationary: a.stationary, walking: a.walking, running: a.running,
                  automotive: a.automotive, cycling: a.cycling, unknown: a.unknown,
                  confidence: DeviceMotionConfidence(a.confidence), start: a.startDate)
    }
}

/// `CMMotionActivityManager` から読む。manager は 1 つだけ持つ。
/// Info.plist に `NSMotionUsageDescription` が無いビルド(審査用の版など)では許可を求めずに `.restricted` を返す。
public final class LiveDeviceMotionActivity: DeviceMotionActivitySource, @unchecked Sendable {
    private let manager = CMMotionActivityManager()
    private let queue = OperationQueue()

    public init() {
        queue.maxConcurrentOperationCount = 1
        queue.qualityOfService = .utility
    }

    public var isAvailable: Bool { CMMotionActivityManager.isActivityAvailable() }

    static func hasUsageDescription(in info: [String: Any]?) -> Bool {
        guard let text = info?["NSMotionUsageDescription"] as? String else { return false }
        return !text.isEmpty
    }

    public func authorization(requesting: Bool) async -> DeviceMotionActivityAccess {
        let current = DeviceMotionActivityAccess(CMMotionActivityManager.authorizationStatus())
        guard requesting, current == .notDetermined else { return current }
        guard Self.hasUsageDescription(in: Bundle.main.infoDictionary) else { return .restricted }
        // 許可ダイアログを出す口は無いので、履歴を 1 回だけ引いて OS に出させる。
        _ = try? await history(from: Date().addingTimeInterval(-60), to: Date())
        return DeviceMotionActivityAccess(CMMotionActivityManager.authorizationStatus())
    }

    public func current() async throws -> DeviceMotionActivityReading? {
        try check()
        let once = ResumeOnce<DeviceMotionActivityReading?>()
        return await withCheckedContinuation { cont in
            once.attach(cont)
            manager.startActivityUpdates(to: queue) { [weak self] activity in
                guard let activity else { return }
                self?.manager.stopActivityUpdates()
                once.resume(DeviceMotionActivityMapping.reading(from: DeviceMotionActivityFlags(activity)))
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + DeviceMotionActivityLimits.currentTimeout) { [weak self] in
                guard once.isPending else { return }
                self?.manager.stopActivityUpdates()
                once.resume(nil)
            }
        }
    }

    public func history(from: Date, to: Date) async throws -> [DeviceMotionActivityReading] {
        try check()
        guard let range = DeviceMotionActivityLimits.clamp(from: from, to: to) else { return [] }
        let activities: [CMMotionActivity] = try await withCheckedThrowingContinuation { cont in
            manager.queryActivityStarting(from: range.start, to: range.end, to: queue) { list, error in
                if let error { cont.resume(throwing: error) } else { cont.resume(returning: list ?? []) }
            }
        }
        let readings = activities.map { DeviceMotionActivityMapping.reading(from: DeviceMotionActivityFlags($0)) }
        return DeviceMotionActivityMapping.history(readings, in: range)
    }

    public func updates() -> AsyncStream<DeviceMotionActivityReading> {
        AsyncStream { cont in
            guard isAvailable, DeviceMotionActivityAccess(CMMotionActivityManager.authorizationStatus()).canRead else {
                cont.finish()
                return
            }
            manager.startActivityUpdates(to: queue) { activity in
                guard let activity else { return }
                cont.yield(DeviceMotionActivityMapping.reading(from: DeviceMotionActivityFlags(activity)))
            }
            cont.onTermination = { [weak self] _ in self?.manager.stopActivityUpdates() }
        }
    }

    private func check() throws {
        guard isAvailable else { throw DeviceMotionActivityError.unavailable }
        let access = DeviceMotionActivityAccess(CMMotionActivityManager.authorizationStatus())
        guard access.canRead else { throw DeviceMotionActivityError.notAuthorized(access) }
    }
}

/// `CheckedContinuation` を 1 回だけ再開する入れ物(判定が来た時と待ち時間切れの両方から呼ばれる)。
private final class ResumeOnce<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<T, Never>?
    private var done = false

    func attach(_ cont: CheckedContinuation<T, Never>) {
        lock.lock()
        defer { lock.unlock() }
        continuation = cont
    }

    var isPending: Bool {
        lock.lock()
        defer { lock.unlock() }
        return !done
    }

    func resume(_ value: T) {
        lock.lock()
        let cont = done ? nil : continuation
        done = true
        continuation = nil
        lock.unlock()
        cont?.resume(returning: value)
    }
}
#endif
#endif
