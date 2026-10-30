import Foundation

/// `dashcam`(#410)の純粋ロジック。カメラ・CoreMotion に依存しない(時刻とセンサー値を渡してテストする)。
/// - `DashcamConfig`: DSL の値を範囲に丸めた設定
/// - `DashcamSegment`: 1 分ごとの録画区間(ファイル名に開始時刻と長さを持つ)
/// - `DashcamLoop`: ループ録画で消す区間・容量の上限
/// - `DashcamImpact`: 加速度の閾値で衝撃を検知し、前後の区間を保存済みにする
/// - `DashcamAutoStart`: 「横向き・ほぼ静止・充電中」が続いたら録画を始める
public struct DashcamConfig: Equatable, Sendable {
    public enum Condition: String, CaseIterable, Sendable {
        case landscape, still, charging
    }

    /// 1 区間の長さ(秒)。DSL では変えない。
    public static let segmentSeconds: TimeInterval = 60

    public var keepSeconds: TimeInterval
    public var maxBytes: Int64
    public var impactG: Double
    public var autoStart: Set<Condition>
    public var audio: Bool

    public init(keepMinutes: Int? = nil, maxStorageMB: Int? = nil, impactG: Double? = nil, autoStart: [String]? = nil, audio: Bool? = nil) {
        keepSeconds = TimeInterval(Swift.min(Swift.max(keepMinutes ?? 10, 1), 60)) * 60
        maxBytes = Int64(Swift.min(Swift.max(maxStorageMB ?? 2000, 200), 20000)) * 1_000_000
        self.impactG = Swift.min(Swift.max(impactG ?? 2.5, 1.2), 8)
        self.autoStart = Set((autoStart ?? []).compactMap(Condition.init(rawValue:)))
        self.audio = audio ?? false
    }

    public init(_ c: Component) {
        self.init(keepMinutes: c.keepMinutes, maxStorageMB: c.maxStorageMB, impactG: c.impactG, autoStart: c.autoStart, audio: c.audio)
    }
}

/// 録画区間 1 つ。ファイル名 `seg-<開始 ms>-<長さ ms>.mov` に開始時刻と長さを持たせる(メタデータのファイルを別に持たない)。
public struct DashcamSegment: Equatable, Sendable, Identifiable {
    public var start: Date
    public var duration: TimeInterval
    public var bytes: Int64
    /// 保存済み(衝撃・手動で保存した区間。ループで消さない)
    public var saved: Bool

    public var id: String { fileName }
    public var end: Date { start.addingTimeInterval(duration) }

    public init(start: Date, duration: TimeInterval, bytes: Int64 = 0, saved: Bool = false) {
        self.start = start
        self.duration = duration
        self.bytes = bytes
        self.saved = saved
    }

    public static let fileExtension = "mov"

    public var fileName: String {
        "seg-\(Int64((start.timeIntervalSince1970 * 1000).rounded()))-\(Int64((duration * 1000).rounded())).\(Self.fileExtension)"
    }

    /// 録画中のファイル名(長さが決まる前)。
    public static func recordingName(start: Date) -> String {
        "rec-\(Int64((start.timeIntervalSince1970 * 1000).rounded())).\(fileExtension)"
    }

    /// ファイル名から読む。形が違えば nil(区間として扱わない)。
    public init?(fileName: String, bytes: Int64 = 0, saved: Bool = false) {
        guard fileName.hasPrefix("seg-"), fileName.hasSuffix("." + Self.fileExtension) else { return nil }
        let body = fileName.dropFirst(4).dropLast(Self.fileExtension.count + 1)
        let parts = body.split(separator: "-", omittingEmptySubsequences: false)
        guard parts.count == 2, let startMs = Int64(parts[0]), let durMs = Int64(parts[1]), startMs >= 0, durMs >= 0 else { return nil }
        self.init(start: Date(timeIntervalSince1970: TimeInterval(startMs) / 1000), duration: TimeInterval(durMs) / 1000, bytes: bytes, saved: saved)
    }
}

public enum DashcamLoop {
    /// 消すループ区間(古い順)。直近 `keepSeconds` より前に終わった区間と、合計が `maxBytes` を超える分の古い区間。保存済みは消さない。
    public static func expired(_ segments: [DashcamSegment], now: Date, keepSeconds: TimeInterval, maxBytes: Int64) -> [DashcamSegment] {
        let loop = segments.filter { !$0.saved }.sorted { $0.start < $1.start }
        let cutoff = now.addingTimeInterval(-keepSeconds)
        var drop = Set(loop.filter { $0.end < cutoff }.map(\.id))
        var total = segments.filter { !drop.contains($0.id) }.reduce(Int64(0)) { $0 + $1.bytes }
        for s in loop where total > maxBytes && !drop.contains(s.id) {
            drop.insert(s.id)
            total -= s.bytes
        }
        return loop.filter { drop.contains($0.id) }
    }

    /// 保存済みだけで上限に届く(次の 1 区間 `reserve` を足すと超える)= これ以上録れない。
    public static func isFull(_ segments: [DashcamSegment], maxBytes: Int64, reserve: Int64) -> Bool {
        segments.filter(\.saved).reduce(Int64(0)) { $0 + $1.bytes } + reserve > maxBytes
    }

    /// 次の 1 区間の見込み容量: 直近の区間の最大(無ければ 60MB = 720p 1 分の目安)。
    public static func reserve(_ segments: [DashcamSegment]) -> Int64 {
        segments.sorted { $0.start > $1.start }.prefix(3).map(\.bytes).max().flatMap { $0 > 0 ? $0 : nil } ?? 60_000_000
    }
}

/// 衝撃の検知。重力を除いた加速度(G)の大きさが閾値以上なら衝撃。前後 `before` / `after` 秒にかかる区間を保存済みにする。
public struct DashcamImpact: Sendable {
    public static let before: TimeInterval = 60
    public static let after: TimeInterval = 30
    /// 同じ衝撃の揺れ戻しを 2 回と数えない
    public static let cooldown: TimeInterval = 5

    public var thresholdG: Double
    public private(set) var lastImpact: Date?
    public private(set) var windows: [ClosedRange<Date>] = []

    public init(thresholdG: Double) {
        self.thresholdG = thresholdG
    }

    /// 加速度 1 サンプル。衝撃として数えたら true。
    public mutating func feed(x: Double, y: Double, z: Double, at date: Date) -> Bool {
        guard (x * x + y * y + z * z).squareRoot() >= thresholdG else { return false }
        if let lastImpact, date.timeIntervalSince(lastImpact) < Self.cooldown { return false }
        mark(at: date)
        return true
    }

    /// 衝撃が無くても前後を保存する(「今の映像を保存」ボタン)。
    public mutating func mark(at date: Date) {
        lastImpact = date
        windows.append(date.addingTimeInterval(-Self.before)...date.addingTimeInterval(Self.after))
    }

    /// この区間を保存済みにするか(どれかの衝撃の前後にかかる)。
    public func protects(start: Date, end: Date) -> Bool {
        windows.contains { $0.lowerBound < end && start < $0.upperBound }
    }

    /// `date` より前に終わった窓を捨てる(録画が長く続いても配列を伸ばさない)。
    public mutating func forget(before date: Date) {
        windows.removeAll { $0.upperBound < date }
    }
}

/// 自動開始。条件が全部 `hold` 秒続いたら 1 回だけ開始を返す。開始・手での停止の後は、条件が一度外れるまで再び開始しない
/// (止めた直後にまた始まらないように)。
public struct DashcamAutoStart: Sendable {
    public struct Sample: Sendable {
        /// 重力の向き(G・端末座標)。横向きの判定に使う
        public var gravity: (x: Double, y: Double, z: Double)
        /// 重力を除いた加速度の大きさ(G)
        public var acceleration: Double
        /// 回転の速さ(rad/s)
        public var rotation: Double
        public var charging: Bool
        public var at: Date

        public init(gravity: (x: Double, y: Double, z: Double), acceleration: Double, rotation: Double, charging: Bool, at: Date) {
            self.gravity = gravity
            self.acceleration = acceleration
            self.rotation = rotation
            self.charging = charging
            self.at = at
        }
    }

    public static let hold: TimeInterval = 5
    public static let stillAcceleration = 0.06
    public static let stillRotation = 0.3

    public var conditions: Set<DashcamConfig.Condition>
    private var since: Date?
    private var armed = true

    public init(conditions: Set<DashcamConfig.Condition>) {
        self.conditions = conditions
    }

    /// 横向き: 重力が端末の横(x)方向に寄っている(画面の向きのロックに関係なく、置いた姿勢で判定する)。
    public static func isLandscape(_ g: (x: Double, y: Double, z: Double)) -> Bool {
        abs(g.x) >= 0.5 && abs(g.x) > abs(g.y)
    }

    public func met(_ s: Sample) -> Bool {
        conditions.allSatisfy { c in
            switch c {
            case .landscape: Self.isLandscape(s.gravity)
            case .still: s.acceleration < Self.stillAcceleration && s.rotation < Self.stillRotation
            case .charging: s.charging
            }
        }
    }

    /// 1 サンプル。今始めるなら true。
    public mutating func feed(_ s: Sample) -> Bool {
        guard !conditions.isEmpty else { return false }
        guard met(s) else {
            since = nil
            armed = true
            return false
        }
        guard armed else { return false }
        let first = since ?? s.at
        since = first
        guard s.at.timeIntervalSince(first) >= Self.hold else { return false }
        disarm()
        return true
    }

    /// 開始まであと何秒か(条件を満たしていない・待っていない時は nil)。
    public func remaining(at date: Date) -> TimeInterval? {
        guard armed, let since else { return nil }
        return max(0, Self.hold - date.timeIntervalSince(since))
    }

    /// 録画を始めた・手で止めた時。条件が一度外れるまで自動で始めない。
    public mutating func disarm() {
        armed = false
        since = nil
    }

    /// 裏から戻った時など、条件が続いていればまた始めてよい状態に戻す。
    public mutating func rearm() {
        armed = true
        since = nil
    }
}

/// 端末内の置き場のキー(アプリ名 → 英数字)。パスに使うので記号を通さない(FNV-1a 64bit)。
public enum DashcamStoreKey {
    public static func make(appName: String) -> String {
        var h: UInt64 = 0xcbf2_9ce4_8422_2325
        for b in appName.utf8 {
            h ^= UInt64(b)
            h = h &* 0x100_0000_01b3
        }
        return String(h, radix: 16)
    }
}
