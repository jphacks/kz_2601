import Foundation
#if os(iOS)
import UIKit
#endif

// 端末の電池・充電・省電力モード(#1989・親 #1835)。式・スキーマ・AI の決まりにはまだ出さない(#1775 の決定待ち)。
// 値は端末の中だけで使い、サーバーには送らない。

/// 端末の電源の状態。
public struct DevicePowerState: Equatable, Sendable {
    public enum Charge: String, Equatable, Sendable {
        /// 分からない(シミュレータ・監視が無効・Mac など)
        case unknown
        case unplugged
        case charging
        case full
    }

    /// 残り 0〜1。分からない時は nil(UIDevice の -1 もここに落とす)
    public var batteryLevel: Double?
    public var charge: Charge
    public var isLowPowerMode: Bool

    public init(batteryLevel: Double?, charge: Charge, isLowPowerMode: Bool) {
        self.batteryLevel = batteryLevel.map { min(max($0, 0), 1) }
        self.charge = charge
        self.isLowPowerMode = isLowPowerMode
    }

    /// 電源につながっている(満充電も含む)
    public var isCharging: Bool { charge == .charging || charge == .full }
    public var isFull: Bool { charge == .full }

    public static let unknown = DevicePowerState(batteryLevel: nil, charge: .unknown, isLowPowerMode: false)

    /// OS の生の値から作る。`rawLevel` が 0〜1 の外(シミュレータ・監視無効の -1)なら不明。
    /// 充電の状態が unknown なら残りも信用しない(UIDevice は監視無効の時 unknown と -1 を返す)。
    public static func from(rawLevel: Float, charge: Charge, isLowPowerMode: Bool) -> DevicePowerState {
        let level: Double? = (charge == .unknown || !(0...1).contains(rawLevel)) ? nil : Double(rawLevel)
        return DevicePowerState(batteryLevel: level, charge: charge, isLowPowerMode: isLowPowerMode)
    }
}

/// 監視を止めるための札。`cancel()` を呼ぶか手放すと止まる。
@MainActor
public final class DevicePowerObservation {
    private var onCancel: (@MainActor () -> Void)?

    init(onCancel: @escaping @MainActor () -> Void) { self.onCancel = onCancel }

    public func cancel() {
        let f = onCancel
        onCancel = nil
        f?()
    }

    deinit {
        // deinit は MainActor 外から来ることがあるので、止める処理は main に回す
        if let f = onCancel { Task { @MainActor in f() } }
    }
}

/// 端末の電源の状態を読む口。
@MainActor
public protocol DevicePowerSource: AnyObject {
    /// 今の値(監視していなくても読める)
    var current: DevicePowerState { get }
    /// 変わるたびに呼ぶ(登録した時に今の値を 1 回呼ぶ)。札を cancel すると止まる。
    func observe(_ onChange: @escaping @MainActor (DevicePowerState) -> Void) -> DevicePowerObservation
}

public extension DevicePowerSource {
    /// `observe` の AsyncStream 版。ストリームを捨てると監視も止まる。
    func updates() -> AsyncStream<DevicePowerState> {
        AsyncStream { continuation in
            let observation = observe { continuation.yield($0) }
            continuation.onTermination = { _ in
                Task { @MainActor in observation.cancel() }
            }
        }
    }
}

// MARK: - 実体

/// OS から読む部分(テストで差し替える)。
@MainActor
struct DevicePowerReadings {
    var isMonitoringEnabled: () -> Bool
    var setMonitoringEnabled: (Bool) -> Void
    var rawLevel: () -> Float
    var charge: () -> DevicePowerState.Charge
    var isLowPowerMode: () -> Bool
    /// 変わった時に届く通知の名前
    var notificationNames: [Notification.Name]
}

/// OS の値を読む実体。電池の監視(`isBatteryMonitoringEnabled`)は見ている間だけ有効にし、最後の監視が止まったら元の値に戻す。
@MainActor
public final class LiveDevicePowerSource: DevicePowerSource {
    private let readings: DevicePowerReadings
    private let center: NotificationCenter
    private var subscribers: [UUID: @MainActor (DevicePowerState) -> Void] = [:]
    private var tokens: [NSObjectProtocol] = []
    /// 監視を始める前の値(戻す用)。監視していない時は nil
    private var monitoringBefore: Bool?
    private var last: DevicePowerState?

    public convenience init() {
        self.init(readings: .system, center: .default)
    }

    init(readings: DevicePowerReadings, center: NotificationCenter) {
        self.readings = readings
        self.center = center
    }

    public var current: DevicePowerState {
        if monitoringBefore != nil { return read() }
        // 監視が無効だと UIDevice は -1/unknown を返すので、読む間だけ有効にして戻す
        let before = readings.isMonitoringEnabled()
        if !before { readings.setMonitoringEnabled(true) }
        defer { if !before { readings.setMonitoringEnabled(false) } }
        return read()
    }

    public func observe(_ onChange: @escaping @MainActor (DevicePowerState) -> Void) -> DevicePowerObservation {
        let id = UUID()
        if subscribers.isEmpty { start() }
        subscribers[id] = onChange
        let state = read()
        last = state
        onChange(state)
        return DevicePowerObservation { [weak self] in self?.remove(id) }
    }

    /// 監視中か(テスト用)
    var isObserving: Bool { monitoringBefore != nil }

    private func read() -> DevicePowerState {
        .from(rawLevel: readings.rawLevel(), charge: readings.charge(), isLowPowerMode: readings.isLowPowerMode())
    }

    private func start() {
        monitoringBefore = readings.isMonitoringEnabled()
        readings.setMonitoringEnabled(true)
        tokens = readings.notificationNames.map { name in
            // 省電力モードの通知は main 以外で届くことがあるので main に寄せる
            center.addObserver(forName: name, object: nil, queue: nil) { [weak self] _ in
                if Thread.isMainThread {
                    MainActor.assumeIsolated { self?.refresh() }
                } else {
                    Task { @MainActor in self?.refresh() }
                }
            }
        }
    }

    private func remove(_ id: UUID) {
        guard subscribers.removeValue(forKey: id) != nil, subscribers.isEmpty else { return }
        tokens.forEach(center.removeObserver)
        tokens = []
        if let before = monitoringBefore { readings.setMonitoringEnabled(before) }
        monitoringBefore = nil
        last = nil
    }

    func refresh() {
        guard monitoringBefore != nil else { return }
        let state = read()
        guard state != last else { return }
        last = state
        for f in subscribers.values { f(state) }
    }
}

extension DevicePowerReadings {
    static var system: DevicePowerReadings {
        #if os(iOS)
        DevicePowerReadings(
            isMonitoringEnabled: { UIDevice.current.isBatteryMonitoringEnabled },
            setMonitoringEnabled: { UIDevice.current.isBatteryMonitoringEnabled = $0 },
            rawLevel: { UIDevice.current.batteryLevel },
            charge: { DevicePowerState.Charge(UIDevice.current.batteryState) },
            isLowPowerMode: { ProcessInfo.processInfo.isLowPowerModeEnabled },
            notificationNames: [
                UIDevice.batteryLevelDidChangeNotification,
                UIDevice.batteryStateDidChangeNotification,
                .NSProcessInfoPowerStateDidChange,
            ]
        )
        #else
        // Mac は電池を読まない(#1989 の対象外)。省電力モードだけ OS から読む
        DevicePowerReadings(
            isMonitoringEnabled: { false },
            setMonitoringEnabled: { _ in },
            rawLevel: { -1 },
            charge: { .unknown },
            isLowPowerMode: { ProcessInfo.processInfo.isLowPowerModeEnabled },
            notificationNames: [.NSProcessInfoPowerStateDidChange]
        )
        #endif
    }
}

#if os(iOS)
extension DevicePowerState.Charge {
    init(_ state: UIDevice.BatteryState) {
        switch state {
        case .unplugged: self = .unplugged
        case .charging: self = .charging
        case .full: self = .full
        case .unknown: self = .unknown
        @unknown default: self = .unknown
        }
    }
}
#endif

// MARK: - 偽物

/// テスト・`METAMON_MOCK=1`・DEBUG で使う偽物。`state` を書き換えると監視している側に届く。
@MainActor
public final class FakeDevicePowerSource: DevicePowerSource {
    public var state: DevicePowerState {
        didSet { if state != oldValue { subscribers.values.forEach { $0(state) } } }
    }
    private var subscribers: [UUID: @MainActor (DevicePowerState) -> Void] = [:]

    public init(_ state: DevicePowerState = DevicePowerState(batteryLevel: 0.8, charge: .unplugged, isLowPowerMode: false)) {
        self.state = state
    }

    /// 環境変数で初期値を決める: `METAMON_BATTERY_LEVEL`(0〜1・無ければ不明)・`METAMON_BATTERY_STATE`(unknown/unplugged/charging/full)・`METAMON_LOW_POWER=1`
    public convenience init(environment: [String: String]) {
        let charge = environment["METAMON_BATTERY_STATE"].flatMap(DevicePowerState.Charge.init(rawValue:))
        let level = environment["METAMON_BATTERY_LEVEL"].flatMap(Double.init)
        let state = DevicePowerState(batteryLevel: level, charge: charge ?? (level == nil ? .unknown : .unplugged),
                                     isLowPowerMode: environment["METAMON_LOW_POWER"] == "1")
        self.init(state)
    }

    public var current: DevicePowerState { state }

    /// 見ている数(テスト用)
    public var observerCount: Int { subscribers.count }

    public func observe(_ onChange: @escaping @MainActor (DevicePowerState) -> Void) -> DevicePowerObservation {
        let id = UUID()
        subscribers[id] = onChange
        onChange(state)
        return DevicePowerObservation { [weak self] in self?.subscribers[id] = nil }
    }
}

// MARK: - 選ぶ

public enum DevicePowerSources {
    /// `METAMON_MOCK=1` なら偽物、それ以外は OS の実体。
    @MainActor
    public static func make(environment: [String: String] = ProcessInfo.processInfo.environment) -> DevicePowerSource {
        environment["METAMON_MOCK"] == "1" ? FakeDevicePowerSource(environment: environment) : LiveDevicePowerSource()
    }
}
