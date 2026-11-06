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

/// 終了通知をどうするか(純粋関数・#38)。countdown が走っていれば **開始した時点で** 終了時刻に予約する
/// (部品が画面から消えても・背面に回っても残る)。一時停止・リセット・終了で取り消す。
/// 前面でその部品が見えている間はバナーを出さない(`TimerVisibility`・アプリの通知 delegate が判定)。
public enum TimerNotificationPlan: Equatable, Sendable {
    case schedule(at: Date)
    case cancel

    public static func decide(isCountdown: Bool, run: TimerRun, total: Int, now: Date) -> TimerNotificationPlan {
        guard isCountdown, let end = run.endDate(total: total), end > now else { return .cancel }
        return .schedule(at: end)
    }
}

/// 走行状態の保存先(UserDefaults)。アプリを殺しても開始時刻が残り、次に開いた時に残り時間 / 完了を正しく出す。
/// キーは `<appId>/<画面>/<部品 id>`(`key(appId:screen:component:)`)。同じ名前のアプリが 2 つあっても混ざらない。
public struct TimerRunStore: @unchecked Sendable {
    public static let shared = TimerRunStore()
    static let prefix = "metamon.timer."

    let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    /// 部品ごとに一意なキー(通知の識別子・ライブアクティビティのキーにも使う)。
    public static func key(appId: String, screen: String, component: String) -> String {
        "\(appId)/\(screen)/\(component)"
    }

    public func load(_ key: String) -> TimerRun? {
        guard let data = defaults.data(forKey: Self.prefix + key) else { return nil }
        return try? JSONDecoder().decode(TimerRun.self, from: data)
    }

    public func save(_ run: TimerRun, for key: String) {
        guard run != TimerRun() else { return clear(key) }
        defaults.set(try? JSONEncoder().encode(run), forKey: Self.prefix + key)
    }

    public func clear(_ key: String) {
        defaults.removeObject(forKey: Self.prefix + key)
    }

    /// アプリを消した時: そのアプリのタイマーの状態をすべて消す。
    public func clearApp(appId: String) {
        let prefix = Self.prefix + Self.appPrefix(appId)
        for key in defaults.dictionaryRepresentation().keys where key.hasPrefix(prefix) {
            defaults.removeObject(forKey: key)
        }
    }

    static func appPrefix(_ appId: String) -> String { appId + "/" }
}

/// アプリを消した時のタイマーの後片付け(#38 の終了通知と保存した走行状態)。キーは appId で始まるので、
/// 同じ名前の別のアプリのタイマーは残る。
public enum TimerCleanup {
    @MainActor
    public static func removeApp(appId: String) async {
        TimerRunStore.shared.clearApp(appId: appId)
        await TimerNotifier.cancelAll(prefix: TimerNotifier.prefix + TimerRunStore.appPrefix(appId))
    }
}

/// 前面で画面に出ているタイマー部品(キー)。見えているタイマーの終了はバナーを出さない(画面で 00:00 が見える)。
@MainActor
public enum TimerVisibility {
    private static var visible: [String: Int] = [:]

    static func appeared(_ key: String) { visible[key, default: 0] += 1 }
    static func disappeared(_ key: String) {
        guard let n = visible[key] else { return }
        visible[key] = n > 1 ? n - 1 : nil
    }

    /// 通知の識別子(`timer.<キー>`)のタイマーが今見えているか。タイマー以外の通知は false。
    public static func isShowing(notificationId id: String) -> Bool {
        guard id.hasPrefix(TimerNotifier.prefix) else { return false }
        return visible[String(id.dropFirst(TimerNotifier.prefix.count))] != nil
    }
}

/// 終了通知の予約先(テストでは差し替える)。
public protocol TimerNotificationCenter: AnyObject {
    func add(_ request: UNNotificationRequest) async throws
    func removePendingNotificationRequests(withIdentifiers identifiers: [String])
    func pendingNotificationRequests() async -> [UNNotificationRequest]
    func requestAuthorizationIfNotDetermined() async
}

extension UNUserNotificationCenter: TimerNotificationCenter {
    public func requestAuthorizationIfNotDetermined() async {
        if await notificationSettings().authorizationStatus == .notDetermined {
            _ = try? await requestAuthorization(options: [.alert, .sound])
        }
    }
}

/// 終了時のローカル通知(識別子 prefix `timer.`。DSL 通知の `dsl.` とは別なので `sync` に消されない)。
/// 予約・取り消しは 1 本の列で順番に流す(開始 → すぐ一時停止で、後から予約が残らないように)。
@MainActor
public enum TimerNotifier {
    public static let prefix = "timer."
    /// nil なら `UNUserNotificationCenter.current()`(テストのホスト無しでは触らない)
    static var center: (any TimerNotificationCenter)?
    private static var resolvedCenter: any TimerNotificationCenter { center ?? UNUserNotificationCenter.current() }
    private static var tail: Task<Void, Never>?

    static func enqueue(_ work: @escaping @MainActor (any TimerNotificationCenter) async -> Void) {
        let previous = tail
        let target = resolvedCenter
        tail = Task { @MainActor in
            await previous?.value
            await work(target)
        }
    }

    /// 列が空になるまで待つ(テスト・移行用)。
    static func drain() async { await tail?.value }

    /// - title: 通知の見出し(アプリ名) / name: タイマーの名前(部品の label)
    static func apply(_ plan: TimerNotificationPlan, key: String, title: String, name: String) {
        let id = prefix + key
        switch plan {
        case .cancel:
            enqueue { $0.removePendingNotificationRequests(withIdentifiers: [id]) }
        case .schedule(let end):
            let content = UNMutableNotificationContent()
            content.title = title
            content.body = L("\(name)のタイマーが終わりました")
            content.sound = .default
            enqueue { center in
                await center.requestAuthorizationIfNotDetermined() // 初めての countdown で 1 回だけダイアログ
                let trigger = UNTimeIntervalNotificationTrigger(timeInterval: max(1, end.timeIntervalSinceNow), repeats: false)
                try? await center.add(UNNotificationRequest(identifier: id, content: content, trigger: trigger))
                print("[Metamon] timer: notification scheduled id=\(id) in \(Int(end.timeIntervalSinceNow))s")
            }
        }
    }

    static func cancelAll(prefix: String) async {
        enqueue { center in
            let ids = await center.pendingNotificationRequests().map(\.identifier).filter { $0.hasPrefix(prefix) }
            center.removePendingNotificationRequests(withIdentifiers: ids)
        }
        await drain()
    }
}

/// 古いキー(`<アプリ名>/<画面>/<部品 id>`)から新しいキー(`<appId>/...`)への 1 回きりの移行。
/// 同じ名前のアプリが 2 つあると状態が混ざっていたため。どのアプリのものか決められない分は捨てる。
public enum TimerKeyMigration {
    /// 移した印(`metamon.timer.` で始めない: タイマーのキーと見分けるため)
    public static let doneKey = "metamon.timerKeys.migratedToAppId"

    public struct App: Equatable, Sendable {
        public var id: String
        public var name: String
        public init(id: String, name: String) {
            self.id = id
            self.name = name
        }
    }

    public struct Result: Equatable, Sendable {
        /// 古いキー → 新しいキー(`metamon.timer.` を除いた部分)
        public var moved: [String: String] = [:]
        /// どのアプリのものか決められず捨てた古いキー
        public var dropped: [String] = []
    }

    /// 古いキーの持ち主を決める。同じ名前のアプリが複数ある時は、開いているアプリがその名前ならそちらに寄せ、
    /// そうでなければ決められない(nil = 捨てる)。名前に `/` を含むアプリもあるので、一致する名前の最長を使う。
    static func owner(of oldKey: String, apps: [App], current: App?) -> (app: App, rest: String)? {
        let all = apps + (current.map { [$0] } ?? [])
        let names = Set(all.map(\.name)).filter { oldKey.hasPrefix($0 + "/") }
        guard let name = names.max(by: { $0.count < $1.count }) else { return nil }
        let rest = String(oldKey.dropFirst(name.count + 1))
        let ids = Set(all.filter { $0.name == name }.map(\.id))
        if ids.count == 1, let id = ids.first { return (App(id: id, name: name), rest) }
        if let current, current.name == name { return (current, rest) }
        return nil
    }

    /// UserDefaults の古いキーを移す(1 回だけ・印を残す)。印があれば何もしない(空の結果)。
    @discardableResult
    public static func migrateStore(_ defaults: UserDefaults, apps: [App], current: App?) -> Result {
        guard !defaults.bool(forKey: doneKey) else { return Result() }
        var result = Result()
        let appIds = Set((apps + (current.map { [$0] } ?? [])).map(\.id))
        let prefix = TimerRunStore.prefix
        for (fullKey, value) in defaults.dictionaryRepresentation() where fullKey.hasPrefix(prefix) {
            let oldKey = String(fullKey.dropFirst(prefix.count))
            // 既に新しい形(appId は UUID。一覧に無いアプリの分もあるので形でも見る)
            if let first = oldKey.split(separator: "/").first.map(String.init), appIds.contains(first) || UUID(uuidString: first) != nil { continue }
            defaults.removeObject(forKey: fullKey)
            guard let (app, rest) = owner(of: oldKey, apps: apps, current: current) else {
                result.dropped.append(oldKey)
                continue
            }
            let newKey = TimerRunStore.appPrefix(app.id) + rest
            if defaults.object(forKey: prefix + newKey) == nil { defaults.set(value, forKey: prefix + newKey) }
            result.moved[oldKey] = newKey
        }
        defaults.set(true, forKey: doneKey)
        result.dropped.sort()
        return result
    }

    /// 予約済みの終了通知を新しい識別子で取り直す(捨てたキーの分は取り消す)。
    @MainActor
    public static func migrateNotifications(_ result: Result) async {
        guard !result.moved.isEmpty || !result.dropped.isEmpty else { return }
        TimerNotifier.enqueue { center in
            for request in await center.pendingNotificationRequests() where request.identifier.hasPrefix(TimerNotifier.prefix) {
                let oldKey = String(request.identifier.dropFirst(TimerNotifier.prefix.count))
                guard result.moved[oldKey] != nil || result.dropped.contains(oldKey) else { continue }
                center.removePendingNotificationRequests(withIdentifiers: [request.identifier])
                guard let newKey = result.moved[oldKey],
                      let end = (request.trigger as? UNTimeIntervalNotificationTrigger)?.nextTriggerDate(), end > .now else { continue }
                let trigger = UNTimeIntervalNotificationTrigger(timeInterval: max(1, end.timeIntervalSinceNow), repeats: false)
                try? await center.add(UNNotificationRequest(identifier: TimerNotifier.prefix + newKey, content: request.content, trigger: trigger))
            }
        }
        await TimerNotifier.drain()
    }
}

/// `timerDisplay.liveActivity` でホストに渡す 1 回分の状態(#423)。ホストは ActivityKit に映すだけで、時間の計算はここで済ませる。
public struct TimerActivityUpdate: Equatable, Sendable {
    /// 部品ごとに一意(`TimerRunStore` のキーと同じ・appId で始まる)
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
