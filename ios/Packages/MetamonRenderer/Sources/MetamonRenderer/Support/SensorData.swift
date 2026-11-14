// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(CoreMotion を持ち込まない)。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL
#if os(iOS)
import CoreMotion
#endif

/// `sensorValue`(#419)が読むセンサー。ホストは `SensorData.source` を差し替えられる(`METAMON_MOCK=1` は決まった波形)。
public enum SensorAvailability: Equatable, Sendable {
    case available
    /// この端末にセンサーが無い(シミュレータ・気圧計の無い機種)
    case unavailable
    /// モーションとフィットネスが許可されていない
    case denied
}

/// 1 回分の値。`motion` は device motion 由来の時だけ(水平器の泡に使う)。
public struct SensorFrame: Sendable {
    public var value: Double
    public var motion: MotionSample?
    public init(value: Double, motion: MotionSample? = nil) {
        self.value = value
        self.motion = motion
    }
}

/// 絶対高度(標高)1 回分。`accuracy`・`precision` は `CMAbsoluteAltitudeData` と同じ意味の推定誤差(m)。
public struct AbsoluteAltitudeSample: Sendable, Equatable {
    /// 平均海面からの高さ(m)
    public var altitude: Double
    public var accuracy: Double
    public var precision: Double

    public init(altitude: Double, accuracy: Double, precision: Double) {
        self.altitude = altitude
        self.accuracy = accuracy
        self.precision = precision
    }
}

/// 標高の精度。`poor` は値が荒すぎて使えない。
public enum AbsoluteAltitudeQuality: Sendable, Equatable {
    case good
    case fair
    case poor
}

/// 精度の決まり(純関数だけ。端末も OS も見ない)。
public enum AbsoluteAltitudeRule {
    /// ここまでなら十分細かい(m)
    public static let goodMeters: Double = 10
    /// ここを超えたら使わない(m)
    public static let usableMeters: Double = 50

    public static func quality(accuracy: Double, precision: Double) -> AbsoluteAltitudeQuality {
        // 負や NaN は「誤差が分からない」= 使えない(CoreMotion は読めない時に負を返す)。
        guard accuracy.isFinite, precision.isFinite, accuracy >= 0, precision >= 0 else { return .poor }
        let worst = Swift.max(accuracy, precision)
        if worst <= goodMeters { return .good }
        if worst <= usableMeters { return .fair }
        return .poor
    }

    public static func quality(of sample: AbsoluteAltitudeSample) -> AbsoluteAltitudeQuality {
        guard sample.altitude.isFinite else { return .poor }
        return quality(accuracy: sample.accuracy, precision: sample.precision)
    }

    public static func isUsable(_ sample: AbsoluteAltitudeSample) -> Bool { quality(of: sample) != .poor }

    /// 精度が悪い時・値が無い時は nil。保存や表示はこれを通す。
    public static func usableAltitude(_ sample: AbsoluteAltitudeSample?) -> Double? {
        guard let sample, isUsable(sample) else { return nil }
        return sample.altitude
    }
}

@MainActor
public protocol SensorSource: AnyObject {
    func availability(_ kind: SensorKind) -> SensorAvailability
    /// 値の流れ。購読を取り消すと(画面から消える・背面へ)センサーを止める。
    func frames(_ kind: SensorKind) -> AsyncStream<SensorFrame>
    /// 標高が読めるか(iOS 15 以降の対応端末のみ。使えない端末は `.unavailable`)。
    func absoluteAltitudeAvailability() -> SensorAvailability
    /// 1 回だけ読む(`timeout` で必ず止める)。使えない端末・時間切れでは nil。
    func absoluteAltitude(timeout: Duration) async -> AbsoluteAltitudeSample?
    /// 続けて読む。購読を取り消すと止める。
    func absoluteAltitudes() -> AsyncStream<AbsoluteAltitudeSample>
}

/// 標高は後から足した口(#2226)。使えない端末と同じ振る舞いを既定にする。
extension SensorSource {
    public func absoluteAltitudeAvailability() -> SensorAvailability { .unavailable }
    public func absoluteAltitude(timeout: Duration) async -> AbsoluteAltitudeSample? { nil }
    public func absoluteAltitudes() -> AsyncStream<AbsoluteAltitudeSample> { AsyncStream { $0.finish() } }

    /// 既定の待ち時間(数秒)で 1 回だけ読む。
    public func absoluteAltitude() async -> AbsoluteAltitudeSample? {
        await absoluteAltitude(timeout: .seconds(5))
    }
}

@MainActor
public enum SensorData {
    /// アプリ起動時に 1 回だけ差し替える。
    public static var source: any SensorSource = LiveSensorData()
}

/// 実機: CoreMotion の device motion(10 Hz)と CMAltimeter(更新間隔は OS 任せ・約 1 Hz)。
/// 部品が複数あっても manager は 1 つで、購読が 0 になったら止める。
@MainActor
public final class LiveSensorData: SensorSource {
    private var motionSubs: [UUID: (MotionSample) -> Void] = [:]
    private var altitudeSubs: [UUID: (AltitudeSample) -> Void] = [:]
    private var absoluteSubs: [UUID: (AbsoluteAltitudeSample) -> Void] = [:]
    #if os(iOS)
    private let motion = CMMotionManager()
    private let altimeter = CMAltimeter()
    #endif

    public init() {}

    public func availability(_ kind: SensorKind) -> SensorAvailability {
        #if os(iOS)
        if kind.usesAltimeter {
            guard CMAltimeter.isRelativeAltitudeAvailable() else { return .unavailable }
            switch CMAltimeter.authorizationStatus() {
            case .denied, .restricted: return .denied
            default: return .available
            }
        }
        return motion.isDeviceMotionAvailable ? .available : .unavailable
        #else
        return .unavailable
        #endif
    }

    public func absoluteAltitudeAvailability() -> SensorAvailability {
        #if os(iOS)
        guard CMAltimeter.isAbsoluteAltitudeAvailable() else { return .unavailable }
        switch CMAltimeter.authorizationStatus() {
        case .denied, .restricted: return .denied
        default: return .available
        }
        #else
        return .unavailable
        #endif
    }

    /// 1 回だけ読む。値が来ても時間切れでも取り消されても、抜ける時に必ず `stopAbsoluteAltitudeUpdates` を呼ぶ。
    public func absoluteAltitude(timeout: Duration) async -> AbsoluteAltitudeSample? {
        #if os(iOS)
        guard absoluteAltitudeAvailability() == .available else { return nil }
        // 続けて読む購読を止めてしまわないよう、1 回読みは別インスタンスで動かす。
        let reader = CMAltimeter()
        let box = AbsoluteAltitudeBox()
        defer { reader.stopAbsoluteAltitudeUpdates() }
        reader.startAbsoluteAltitudeUpdates(to: .main) { data, _ in
            guard let data else { return }
            let s = LiveSensorData.sample(from: data)
            MainActor.assumeIsolated { box.sample = s }
        }
        let deadline = ContinuousClock.now + timeout
        while box.sample == nil, ContinuousClock.now < deadline, !Task.isCancelled {
            try? await Task.sleep(for: .milliseconds(50))
        }
        return box.sample
        #else
        return nil
        #endif
    }

    public func absoluteAltitudes() -> AsyncStream<AbsoluteAltitudeSample> {
        AsyncStream { cont in
            let id = UUID()
            absoluteSubs[id] = { cont.yield($0) }
            updateRunning()
            cont.onTermination = { _ in
                Task { @MainActor [weak self] in
                    self?.absoluteSubs[id] = nil
                    self?.updateRunning()
                }
            }
        }
    }

    public func frames(_ kind: SensorKind) -> AsyncStream<SensorFrame> {
        AsyncStream { cont in
            let id = UUID()
            if kind.usesAltimeter {
                altitudeSubs[id] = { a in if let v = kind.value(from: a) { cont.yield(SensorFrame(value: v)) } }
            } else {
                motionSubs[id] = { m in if let v = kind.value(from: m) { cont.yield(SensorFrame(value: v, motion: m)) } }
            }
            updateRunning()
            cont.onTermination = { _ in
                Task { @MainActor [weak self] in
                    self?.motionSubs[id] = nil
                    self?.altitudeSubs[id] = nil
                    self?.updateRunning()
                }
            }
        }
    }

    private func updateRunning() {
        #if os(iOS)
        if motionSubs.isEmpty {
            if motion.isDeviceMotionActive { motion.stopDeviceMotionUpdates() }
        } else if !motion.isDeviceMotionActive, motion.isDeviceMotionAvailable {
            motion.deviceMotionUpdateInterval = 1 / SensorThrottle.maxUIHz
            motion.startDeviceMotionUpdates(to: .main) { [weak self] dm, _ in
                guard let dm else { return }
                let s = MotionSample(
                    pitch: dm.attitude.pitch, roll: dm.attitude.roll,
                    gravity: Vector3(dm.gravity.x, dm.gravity.y, dm.gravity.z),
                    userAcceleration: Vector3(dm.userAcceleration.x, dm.userAcceleration.y, dm.userAcceleration.z),
                    rotationRate: Vector3(dm.rotationRate.x, dm.rotationRate.y, dm.rotationRate.z)
                )
                MainActor.assumeIsolated { self?.motionSubs.values.forEach { $0(s) } }
            }
        }
        if altitudeSubs.isEmpty {
            if altimeterRunning {
                altimeter.stopRelativeAltitudeUpdates()
                altimeterRunning = false
            }
        } else if !altimeterRunning, CMAltimeter.isRelativeAltitudeAvailable() {
            altimeterRunning = true
            altimeter.startRelativeAltitudeUpdates(to: .main) { [weak self] data, _ in
                guard let data else { return }
                let s = AltitudeSample(pressureKPa: data.pressure.doubleValue, relativeAltitude: data.relativeAltitude.doubleValue)
                MainActor.assumeIsolated { self?.altitudeSubs.values.forEach { $0(s) } }
            }
        }
        if absoluteSubs.isEmpty {
            if absoluteRunning {
                altimeter.stopAbsoluteAltitudeUpdates()
                absoluteRunning = false
            }
        } else if !absoluteRunning, CMAltimeter.isAbsoluteAltitudeAvailable() {
            absoluteRunning = true
            altimeter.startAbsoluteAltitudeUpdates(to: .main) { [weak self] data, _ in
                guard let data else { return }
                let s = LiveSensorData.sample(from: data)
                MainActor.assumeIsolated { self?.absoluteSubs.values.forEach { $0(s) } }
            }
        }
        #endif
    }

    #if os(iOS)
    nonisolated static func sample(from data: CMAbsoluteAltitudeData) -> AbsoluteAltitudeSample {
        AbsoluteAltitudeSample(altitude: data.altitude, accuracy: data.accuracy, precision: data.precision)
    }

    private var altimeterRunning = false
    private var absoluteRunning = false
    #endif
}

#if os(iOS)
/// 1 回読みの受け皿(コールバックは main で来る)。
@MainActor
private final class AbsoluteAltitudeBox {
    var sample: AbsoluteAltitudeSample?
}
#endif

/// `METAMON_MOCK=1` / シミュレータの確認用: 10 Hz のゆっくりした波(傾きは 0〜40° を往復してしきい値 30° をまたぐ)。
@MainActor
public final class MockSensorData: SensorSource {
    public init() {}

    public func availability(_ kind: SensorKind) -> SensorAvailability { .available }

    public func absoluteAltitudeAvailability() -> SensorAvailability { .available }

    public func absoluteAltitude(timeout: Duration) async -> AbsoluteAltitudeSample? { Self.absoluteAltitude(at: 0) }

    public func absoluteAltitudes() -> AsyncStream<AbsoluteAltitudeSample> {
        AsyncStream { cont in
            let task = Task { @MainActor in
                let start = Date()
                while !Task.isCancelled {
                    cont.yield(Self.absoluteAltitude(at: Date().timeIntervalSince(start)))
                    try? await Task.sleep(for: .seconds(1))
                }
                cont.finish()
            }
            cont.onTermination = { _ in task.cancel() }
        }
    }

    /// 経過 `t` 秒の標高(テストでも使う)。
    nonisolated static func absoluteAltitude(at t: Double) -> AbsoluteAltitudeSample {
        AbsoluteAltitudeSample(altitude: 40 + 2 * sin(t / 30), accuracy: 5, precision: 3)
    }

    public func frames(_ kind: SensorKind) -> AsyncStream<SensorFrame> {
        AsyncStream { cont in
            let task = Task { @MainActor in
                let start = Date()
                while !Task.isCancelled {
                    let t = Date().timeIntervalSince(start)
                    cont.yield(Self.frame(kind, at: t))
                    try? await Task.sleep(for: .milliseconds(100))
                }
                cont.finish()
            }
            cont.onTermination = { _ in task.cancel() }
        }
    }

    /// 経過 `t` 秒の値(テストでも使う)。
    nonisolated static func frame(_ kind: SensorKind, at t: Double) -> SensorFrame {
        let tilt = 20 + 20 * sin(t * 2 * .pi / 12) // 0...40°・12 秒周期
        let r = tilt * .pi / 180
        let m = MotionSample(
            pitch: r * 0.8, roll: r * 0.6,
            gravity: Vector3(sin(r) * 0.6, -sin(r) * 0.8, -cos(r)),
            userAcceleration: Vector3(0.05 * sin(t * 3), 0.03 * cos(t * 5), 0.02),
            rotationRate: Vector3(0.2 * sin(t), 0.1, 0)
        )
        let a = AltitudeSample(pressureKPa: 101.32 - 0.003 * sin(t / 10), relativeAltitude: 1.5 * sin(t / 20))
        if kind.usesAltimeter { return SensorFrame(value: kind.value(from: a) ?? 0) }
        return SensorFrame(value: kind.value(from: m) ?? 0, motion: m)
    }
}

/// センサーの無い端末と同じ振る舞い(プレビュー・テスト用)。
@MainActor
public final class UnavailableSensorData: SensorSource {
    public init() {}
    public func availability(_ kind: SensorKind) -> SensorAvailability { .unavailable }
    public func frames(_ kind: SensorKind) -> AsyncStream<SensorFrame> { AsyncStream { $0.finish() } }
}
#endif
