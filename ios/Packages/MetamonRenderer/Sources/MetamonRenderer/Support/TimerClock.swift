import Foundation
import UserNotifications

/// `timerDisplay` の時間計算(#38)。表示は持たず、保存する「開始時刻 + 一時停止までの累計」から都度求める。
/// 背面中は Timer が止まるが、前面に戻った瞬間に `now` から正しい値が出る(バックグラウンド実行に頼らない)。
public struct TimerRun: Codable, Equatable, Sendable {
    /// 一時停止までに積み上がった経過秒
    public var base: Int
    /// 走行中なら開始(再開)時刻。停止中は nil
    public var startedAt: Date?

    public init(base: Int = 0, startedAt: Date? = nil) {
        self.base = base
        self.startedAt = startedAt
    }

    public var isRunning: Bool { startedAt != nil }

    /// 経過秒(countdown は `total` で頭打ち)。
    public func elapsed(at now: Date, total: Int? = nil) -> Int {
        let raw = base + (startedAt.map { max(Int(now.timeIntervalSince($0)), 0) } ?? 0)
        return total.map { min(raw, $0) } ?? raw
    }

    /// countdown の残り秒(0 未満にならない)。
    public func remaining(at now: Date, total: Int) -> Int {
        max(total - elapsed(at: now, total: total), 0)
    }

    /// countdown が走行中なら終了時刻。停止中は nil(通知は予約しない)。
    public func endDate(total: Int) -> Date? {
        startedAt.map { $0.addingTimeInterval(TimeInterval(total - base)) }
    }
}

/// 終了通知をどうするか(純粋関数・#38)。前面では出さない(画面で見えている)ので取り消し、背面に回った時だけ予約する。
public enum TimerNotificationPlan: Equatable, Sendable {
    case schedule(at: Date)
    case cancel

    public static func decide(isForeground: Bool, isCountdown: Bool, run: TimerRun, total: Int, now: Date) -> TimerNotificationPlan {
        guard !isForeground, isCountdown, let end = run.endDate(total: total), end > now else { return .cancel }
        return .schedule(at: end)
    }
}

/// 走行状態の保存先(UserDefaults)。アプリを殺しても開始時刻が残り、次に開いた時に残り時間 / 完了を正しく出す。
public struct TimerRunStore: Sendable {
    public static let shared = TimerRunStore()
    private static let prefix = "metamon.timer."

    public init() {}

    public func load(_ key: String) -> TimerRun? {
        guard let data = UserDefaults.standard.data(forKey: Self.prefix + key) else { return nil }
        return try? JSONDecoder().decode(TimerRun.self, from: data)
    }

    public func save(_ run: TimerRun, for key: String) {
        guard run != TimerRun() else { return clear(key) }
        UserDefaults.standard.set(try? JSONEncoder().encode(run), forKey: Self.prefix + key)
    }

    public func clear(_ key: String) {
        UserDefaults.standard.removeObject(forKey: Self.prefix + key)
    }
}

/// 終了時のローカル通知(識別子 prefix `timer.`。DSL 通知の `dsl.` とは別なので `sync` に消されない)。
enum TimerNotifier {
    static let prefix = "timer."

    /// - title: 通知の見出し(アプリ名) / name: タイマーの名前(部品の label)
    static func apply(_ plan: TimerNotificationPlan, key: String, title: String, name: String) {
        let center = UNUserNotificationCenter.current()
        let id = prefix + key
        switch plan {
        case .cancel:
            center.removePendingNotificationRequests(withIdentifiers: [id])
        case .schedule(let end):
            let content = UNMutableNotificationContent()
            content.title = title
            content.body = L("\(name)のタイマーが終わりました")
            content.sound = .default
            let trigger = UNTimeIntervalNotificationTrigger(timeInterval: max(1, end.timeIntervalSinceNow), repeats: false)
            center.add(UNNotificationRequest(identifier: id, content: content, trigger: trigger))
            print("[Metamon] timer: notification scheduled id=\(id) in \(Int(end.timeIntervalSinceNow))s")
        }
    }

    /// countdown 開始時に 1 回だけ許可を求める(未決定の時のみダイアログ)。
    static func requestPermissionIfNeeded() {
        Task {
            let center = UNUserNotificationCenter.current()
            if await center.notificationSettings().authorizationStatus == .notDetermined {
                _ = try? await center.requestAuthorization(options: [.alert, .sound])
            }
        }
    }
}

/// `timerDisplay.liveActivity` でホストに渡す 1 回分の状態(#423)。ホストは ActivityKit に映すだけで、時間の計算はここで済ませる。
public struct TimerActivityUpdate: Equatable, Sendable {
    /// 部品ごとに一意(`TimerRunStore` のキーと同じ)
    public var key: String
    public var label: String
    /// countdown の合計秒。stopwatch は nil
    public var total: Int?
    public var run: TimerRun

    public init(key: String, label: String, total: Int?, run: TimerRun) {
        self.key = key
        self.label = label
        self.total = total
        self.run = run
    }

    public enum Phase: Equatable, Sendable {
        /// 走行中。`start` は一時停止分を差し引いた見かけの開始時刻、`end` は countdown の終了時刻
        case running(start: Date, end: Date?)
        /// 一時停止中(表示する秒)
        case paused(shown: Int)
        /// countdown が 0 になった: 「終わりました」を少し残して消す
        case finished
        /// リセット: すぐ消す
        case ended
    }

    public var phase: Phase {
        if let startedAt = run.startedAt {
            let start = startedAt.addingTimeInterval(-TimeInterval(run.base))
            return .running(start: start, end: total.map { start.addingTimeInterval(TimeInterval($0)) })
        }
        if run.base == 0 { return .ended }
        if let total {
            return run.base >= total ? .finished : .paused(shown: total - run.base)
        }
        return .paused(shown: run.base)
    }
}
