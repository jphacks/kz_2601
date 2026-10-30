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

@MainActor
public protocol SensorSource: AnyObject {
    func availability(_ kind: SensorKind) -> SensorAvailability
    /// 値の流れ。購読を取り消すと(画面から消える・背面へ)センサーを止める。
    func frames(_ kind: SensorKind) -> AsyncStream<SensorFrame>
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

    public func frames(_ kind: SensorKind) -> AsyncStream<SensorFrame> {
        AsyncStream { cont in
            let id = UUID()
            if kind.usesAltimeter {
                altitudeSubs[id] = { a in kind.value(from: a).map { cont.yield(SensorFrame(value: $0)) } }
            } else {
                motionSubs[id] = { m in kind.value(from: m).map { cont.yield(SensorFrame(value: $0, motion: m)) } }
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
        #endif
    }

    #if os(iOS)
    private var altimeterRunning = false
    #endif
}

/// `METAMON_MOCK=1` / シミュレータの確認用: 10 Hz のゆっくりした波(傾きは 0〜40° を往復してしきい値 30° をまたぐ)。
@MainActor
public final class MockSensorData: SensorSource {
    public init() {}

    public func availability(_ kind: SensorKind) -> SensorAvailability { .available }

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
