import Foundation
import UserNotifications
import MetamonDSL

/// `UNUserNotificationCenter` によるローカル通知(#228)。
/// - アプリごとの予約(`PlannedNotification`)を UserDefaults に持ち、別アプリに切り替えても前のアプリの通知を消さない。
/// - 予約し直す時は全アプリ分から近い順に `NotificationPlanner.limit` 件だけ iOS に渡す(64 件を超えた分が黙って捨てられないように)。
/// - 識別子は `dsl.<appId>.…`。それ以外(タイマー `timer.` 等)には触らない。
public struct UserNotificationScheduler: NotificationScheduler {
    public init() {}

    public func sync(appId: String, specs: [NotificationSpec], records: [String: [Record]]) async -> NotificationSyncResult {
        await NotificationRegistry.shared.set(appId, NotificationPlanner.plan(appId: appId, specs: specs, records: records))
        return await apply(appId: appId)
    }

    public func schedule(_ spec: NotificationSpec, appId: String) async throws -> Date? {
        guard let p = NotificationPlanner.action(appId: appId, spec: spec) else { return nil }
        await NotificationRegistry.shared.upsertAction(appId, p)
        _ = await apply(appId: appId)
        return p.nextFire(after: Date())
    }

    public func cancel(id: String, appId: String) async {
        await NotificationRegistry.shared.remove(appId: appId, specId: id)
        _ = await apply(appId: appId)
    }

    public func removeApp(_ appId: String) async {
        await NotificationRegistry.shared.removeApp(appId)
        _ = await apply(appId: appId)
    }

    public func removeAll() async {
        await NotificationRegistry.shared.removeAll()
        let center = UNUserNotificationCenter.current()
        let ids = await center.pendingNotificationRequests().map(\.identifier).filter { $0.hasPrefix(NotificationPlanner.prefix) }
        center.removePendingNotificationRequests(withIdentifiers: ids)
    }

    public func refresh(appId: String) async -> NotificationSyncResult { await apply(appId: appId) }

    /// 全アプリ分を近い順に上限まで予約し直す。結果の upcoming は `appId` の分だけ。
    private func apply(appId: String) async -> NotificationSyncResult {
        let center = UNUserNotificationCenter.current()
        let now = Date()
        let all = await NotificationRegistry.shared.pruned(now: now)
        let chosen = NotificationPlanner.select(all, now: now)
        let want = Set(chosen.map(\.0.identifier))
        let pending = await center.pendingNotificationRequests().map(\.identifier).filter { $0.hasPrefix(NotificationPlanner.prefix) }
        center.removePendingNotificationRequests(withIdentifiers: pending.filter { !want.contains($0) })
        var denied = false
        if !chosen.isEmpty {
            denied = await !requestPermissionIfNeeded()
        }
        for (p, _) in chosen {
            do {
                try await center.add(Self.request(p))
            } catch {
                print("[Metamon] notification add failed id=\(p.identifier): \(error)")
            }
        }
        let mine = NotificationPlanner.prefix + appId + "."
        let upcoming = chosen.filter { $0.0.identifier.hasPrefix(mine) }.map { UpcomingNotification(title: $0.0.title, body: $0.0.body, date: $0.1) }
        print("[Metamon] notifications scheduled: total=\(chosen.count) app=\(upcoming.count) next=\(upcoming.first.map { "\($0.date)" } ?? "-") denied=\(denied)")
        return NotificationSyncResult(denied: denied, upcoming: upcoming)
    }

    /// 許可されていれば true。未決定ならここでダイアログを出す。
    public func requestPermissionIfNeeded() async -> Bool {
        let center = UNUserNotificationCenter.current()
        switch await center.notificationSettings().authorizationStatus {
        case .notDetermined:
            return (try? await center.requestAuthorization(options: [.alert, .sound, .badge])) ?? false
        case .denied:
            return false
        default:
            return true
        }
    }

    static func request(_ p: PlannedNotification, calendar: Calendar = .current) -> UNNotificationRequest {
        let content = UNMutableNotificationContent()
        content.title = p.title
        content.body = p.body
        content.sound = .default
        let trigger: UNNotificationTrigger
        switch p.trigger {
        case .at(let d):
            trigger = UNCalendarNotificationTrigger(dateMatching: calendar.dateComponents([.year, .month, .day, .hour, .minute, .second], from: d), repeats: false)
        case .daily(let h, let m):
            trigger = UNCalendarNotificationTrigger(dateMatching: DateComponents(hour: h, minute: m), repeats: true)
        case .weekly(let w, let h, let m):
            trigger = UNCalendarNotificationTrigger(dateMatching: DateComponents(hour: h, minute: m, weekday: w), repeats: true)
        }
        return UNNotificationRequest(identifier: p.identifier, content: content, trigger: trigger)
    }

    // 旧 API 互換(テスト)
    public static func parseTime(_ t: String?) -> (Int, Int)? { NotificationPlanner.parseTime(t) }
    public static func weekdayNumber(_ w: String?) -> Int { NotificationPlanner.weekdayNumber(w) }
}

/// アプリごとの予約の保存先(UserDefaults・直列化のため actor)。キー: appId(DSL 分)/ appId + "#action"(ボタン分)。
actor NotificationRegistry {
    static let shared = NotificationRegistry()
    private static let key = "metamon.notifications.planned.v1"
    private let defaults: UserDefaults
    private var table: [String: [PlannedNotification]]

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        if let data = defaults.data(forKey: Self.key), let t = try? JSONDecoder().decode([String: [PlannedNotification]].self, from: data) {
            table = t
        } else {
            table = [:]
        }
    }

    func set(_ appId: String, _ planned: [PlannedNotification]) {
        table[appId] = planned
        save()
    }

    func upsertAction(_ appId: String, _ p: PlannedNotification) {
        var list = table[appId + "#action"] ?? []
        list.removeAll { $0.identifier == p.identifier }
        list.append(p)
        table[appId + "#action"] = list
        save()
    }

    func remove(appId: String, specId: String) {
        let ids = [NotificationPlanner.identifier(appId: appId, specId: specId), NotificationPlanner.actionIdentifier(appId: appId, specId: specId)]
        let recordPrefix = ids[0] + "."
        for k in [appId, appId + "#action"] {
            table[k]?.removeAll { ids.contains($0.identifier) || $0.identifier.hasPrefix(recordPrefix) }
        }
        save()
    }

    func removeApp(_ appId: String) {
        table[appId] = nil
        table[appId + "#action"] = nil
        save()
    }

    func removeAll() {
        table = [:]
        save()
    }

    /// 過ぎた 1 回きりを捨てて、全件を返す。
    func pruned(now: Date) -> [PlannedNotification] {
        var changed = false
        for (k, list) in table {
            let kept = list.filter { $0.nextFire(after: now) != nil }
            if kept.count != list.count { table[k] = kept; changed = true }
        }
        if changed { save() }
        return table.values.flatMap { $0 }
    }

    private func save() {
        if let data = try? JSONEncoder().encode(table) { defaults.set(data, forKey: Self.key) }
    }
}
