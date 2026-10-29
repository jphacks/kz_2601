import Foundation

/// `sensorValue`(#419)の計算: 生のセンサー値 → 表示値(単位変換)・整形・表示の間引き・一定間隔の記録・しきい値。
/// CoreMotion に依存しない(Renderer の実体とモックが `MotionSample` / `AltitudeSample` を渡す)。
public enum SensorKind: String, Sendable, CaseIterable {
    case tilt, pitch, roll, acceleration, rotation, pressure, altitude

    /// 気圧計(CMAltimeter)を使うか。それ以外は device motion。
    public var usesAltimeter: Bool { self == .pressure || self == .altitude }

    public var unit: String {
        switch self {
        case .tilt, .pitch, .roll: "°"
        case .acceleration: "G"
        case .rotation: "°/s"
        case .pressure: "hPa"
        case .altitude: "m"
        }
    }

    /// 小数の桁(表示と保存で同じ)。
    public var fractionDigits: Int {
        switch self {
        case .tilt, .pitch, .roll, .rotation: 0
        case .acceleration: 2
        case .pressure, .altitude: 1
        }
    }

    /// gauge の既定の範囲。
    public var defaultRange: ClosedRange<Double> {
        switch self {
        case .tilt: 0...90
        case .pitch: -90...90
        case .roll: -180...180
        case .acceleration: 0...2
        case .rotation: 0...360
        case .pressure: 950...1050
        case .altitude: -20...20
        }
    }

    /// 表示値。altitude 以外の altimeter 値は nil(device motion 側)。
    public func value(from m: MotionSample) -> Double? {
        switch self {
        case .tilt: SensorMath.tiltDegrees(gravity: m.gravity)
        case .pitch: SensorMath.degrees(m.pitch)
        case .roll: SensorMath.degrees(m.roll)
        case .acceleration: m.userAcceleration.magnitude
        case .rotation: SensorMath.degrees(m.rotationRate.magnitude)
        case .pressure, .altitude: nil
        }
    }

    public func value(from a: AltitudeSample) -> Double? {
        switch self {
        case .pressure: SensorMath.hectopascals(kilopascals: a.pressureKPa)
        case .altitude: a.relativeAltitude
        default: nil
        }
    }

    /// 「12°」「0.35 G」「1013.2 hPa」。-0 は 0 に丸める。
    public func format(_ v: Double) -> String {
        let r = rounded(v)
        return "\(r.formatted(.number.precision(.fractionLength(fractionDigits)).grouping(.never)))"
    }

    /// 保存する値(表示と同じ桁に丸める)。
    public func rounded(_ v: Double) -> Double {
        let p = pow(10, Double(fractionDigits))
        let r = (v * p).rounded() / p
        return r == 0 ? 0 : r
    }
}

public struct Vector3: Sendable, Equatable {
    public var x, y, z: Double
    public init(_ x: Double, _ y: Double, _ z: Double) { self.x = x; self.y = y; self.z = z }
    public var magnitude: Double { (x * x + y * y + z * z).squareRoot() }
}

/// CoreMotion の device motion 1 回分(角度はラジアン・加速度は G・角速度は rad/s)。
public struct MotionSample: Sendable, Equatable {
    public var pitch: Double
    public var roll: Double
    public var gravity: Vector3
    public var userAcceleration: Vector3
    public var rotationRate: Vector3

    public init(pitch: Double = 0, roll: Double = 0, gravity: Vector3 = Vector3(0, 0, -1),
                userAcceleration: Vector3 = Vector3(0, 0, 0), rotationRate: Vector3 = Vector3(0, 0, 0)) {
        self.pitch = pitch
        self.roll = roll
        self.gravity = gravity
        self.userAcceleration = userAcceleration
        self.rotationRate = rotationRate
    }
}

/// CMAltimeter 1 回分(気圧 kPa・開始時からの高度差 m)。
public struct AltitudeSample: Sendable, Equatable {
    public var pressureKPa: Double
    public var relativeAltitude: Double
    public init(pressureKPa: Double, relativeAltitude: Double) {
        self.pressureKPa = pressureKPa
        self.relativeAltitude = relativeAltitude
    }
}

public enum SensorMath {
    public static func degrees(_ radians: Double) -> Double { radians * 180 / .pi }
    public static func hectopascals(kilopascals: Double) -> Double { kilopascals * 10 }

    /// 水平からの傾き(0 = 机に平置き・90 = 立てた状態)。画面の向きの表裏は区別しない。
    public static func tiltDegrees(gravity g: Vector3) -> Double {
        let m = g.magnitude
        guard m > 0 else { return 0 }
        return degrees(acos(Swift.min(1, abs(g.z) / m)))
    }

    /// 水平器の泡の位置(-1...1)。x = 左右、y = 前後。`maxDegrees` 傾くと端。
    public static func bubble(gravity g: Vector3, maxDegrees: Double = 45) -> (x: Double, y: Double) {
        let m = g.magnitude
        guard m > 0 else { return (0, 0) }
        func clamp(_ v: Double) -> Double { Swift.max(-1, Swift.min(1, v)) }
        // 泡は高い方へ動く = 重力の逆向き
        let x = degrees(asin(clamp(g.x / m))) / maxDegrees
        let y = degrees(asin(clamp(g.y / m))) / maxDegrees
        return (clamp(-x), clamp(y))
    }
}

/// 表示の更新を最大 `hz` 回/秒に間引く(センサーの更新はそれより速いことがある)。
public struct SensorThrottle: Sendable {
    public static let maxUIHz: Double = 10
    public let interval: TimeInterval
    private var last: Date?

    public init(hz: Double = SensorThrottle.maxUIHz) { interval = 1 / Swift.max(0.1, Swift.min(hz, Self.maxUIHz)) }

    public mutating func shouldEmit(at now: Date) -> Bool {
        if let last, now.timeIntervalSince(last) < interval - 0.0005 { return false }
        last = now
        return true
    }
}

/// 一定間隔の記録(画面を開いている間だけ)。開始直後に 1 件、以後 `interval` 秒ごと、`limit` 件で終わる。
public struct SensorLogSession: Sendable, Equatable {
    public static let defaultLimit = 60
    public static let maxLimit = 1000
    public static let maxInterval = 3600

    public let interval: TimeInterval
    public let limit: Int
    public private(set) var saved = 0
    private var next: Date?

    /// DSL の値を丸める(seconds 1...3600・limit 既定 60・1...1000)。
    public init(seconds: Int, limit: Int?) {
        interval = TimeInterval(Swift.max(1, Swift.min(seconds, Self.maxInterval)))
        self.limit = Swift.max(1, Swift.min(limit ?? Self.defaultLimit, Self.maxLimit))
    }

    public var isFinished: Bool { saved >= limit }

    /// `now` に 1 件保存すべきか。true を返したら保存済みとして数える(保存の失敗は数えたまま=無限に再試行しない)。
    public mutating func tick(at now: Date) -> Bool {
        guard !isFinished else { return false }
        if let next, now < next { return false }
        saved += 1
        // 予定どおりなら刻みを保ち、大きく遅れたら今から数え直す(取りこぼし分を一気に保存しない)
        let candidate = (next ?? now).addingTimeInterval(interval)
        next = candidate > now ? candidate : now.addingTimeInterval(interval)
        return true
    }
}

/// しきい値: 条件に入った瞬間に 1 回だけ発火。条件から出るまで再発火しない・最短 `cooldown` 秒おき。
public struct SensorThresholdTrigger: Sendable {
    public static let defaultCooldown: TimeInterval = 3
    public let above: Double?
    public let below: Double?
    public let cooldown: TimeInterval
    public private(set) var isInside = false
    private var lastFired: Date?

    public init(above: Double?, below: Double?, cooldown: TimeInterval = SensorThresholdTrigger.defaultCooldown) {
        self.above = above
        self.below = below
        self.cooldown = cooldown
    }

    public func matches(_ v: Double) -> Bool {
        (above.map { v > $0 } ?? false) || (below.map { v < $0 } ?? false)
    }

    /// 発火したら true。
    public mutating func update(_ v: Double, at now: Date) -> Bool {
        let inside = matches(v)
        defer { isInside = inside }
        guard inside, !isInside else { return false }
        if let lastFired, now.timeIntervalSince(lastFired) < cooldown { return false }
        lastFired = now
        return true
    }
}

/// 部品が 1 サンプルごとに通す処理(間引き → しきい値 → 一定間隔の記録)。View はイベントを反映するだけ。
public struct SensorPipeline: Sendable {
    public enum Event: Equatable, Sendable {
        /// しきい値に入った(警告 + 振動・任意で action)
        case fired(Double)
        /// この値を 1 件保存する
        case save(Double)
        /// 一定間隔の記録が上限件数に達して止まった
        case logFinished(Int)
    }

    private var throttle: SensorThrottle
    private var trigger: SensorThresholdTrigger?
    private let saveOnFire: Bool
    public private(set) var log: SensorLogSession?

    public init(threshold: SensorThreshold?, canSave: Bool, hz: Double = SensorThrottle.maxUIHz) {
        throttle = SensorThrottle(hz: hz)
        if let t = threshold, t.above != nil || t.below != nil {
            trigger = SensorThresholdTrigger(above: t.above, below: t.below)
        }
        saveOnFire = canSave && threshold?.save == true
    }

    public var isLogging: Bool { log != nil }
    /// 今の値がしきい値の条件に入っているか(警告の表示)。
    public var isAlerting: Bool { trigger?.isInside ?? false }

    public mutating func startLog(seconds: Int, limit: Int?) { log = SensorLogSession(seconds: seconds, limit: limit) }
    public mutating func stopLog() { log = nil }

    /// 間引かれたら nil(表示も更新しない)。
    public mutating func ingest(_ v: Double, at now: Date) -> [Event]? {
        guard throttle.shouldEmit(at: now) else { return nil }
        var out: [Event] = []
        if trigger?.update(v, at: now) == true {
            out.append(.fired(v))
            if saveOnFire { out.append(.save(v)) }
        }
        if var s = log {
            // しきい値の保存と同じサンプルなら 1 件にまとめる
            if s.tick(at: now), !out.contains(.save(v)) { out.append(.save(v)) }
            if s.isFinished {
                out.append(.logFinished(s.saved))
                log = nil
            } else {
                log = s
            }
        }
        return out
    }
}
