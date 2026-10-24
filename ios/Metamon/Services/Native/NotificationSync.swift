import Foundation
import UserNotifications
import MetamonDSL

/// ローカル通知(P0)。
/// - `app.notifications[]`: DSL ロード毎に id 一致で置換・消えた id は取消(識別子 prefix `dsl.`)。
/// - `scheduleNotification` アクション: 押下時に予約(同 prefix・同 id なら上書き)。
/// - `once` で time が今日既に過ぎていれば翌日に発火。
enum NotificationSync {
    static let prefix = "dsl."

    static func sync(_ specs: [NotificationSpec]) async {
        let center = UNUserNotificationCenter.current()
        let pending = await center.pendingNotificationRequests().map(\.identifier).filter { $0.hasPrefix(prefix) }
        let keep = Set(specs.map { prefix + $0.id })
        center.removePendingNotificationRequests(withIdentifiers: pending.filter { !keep.contains($0) })
        guard !specs.isEmpty else { return }
        await requestPermissionIfNeeded()
        for s in specs { try? await schedule(s) }
    }

    static func requestPermissionIfNeeded() async {
        let center = UNUserNotificationCenter.current()
        if await center.notificationSettings().authorizationStatus == .notDetermined {
            _ = try? await center.requestAuthorization(options: [.alert, .sound, .badge])
        }
    }

    static func schedule(_ s: NotificationSpec) async throws {
        let center = UNUserNotificationCenter.current()
        await requestPermissionIfNeeded()
        let content = UNMutableNotificationContent()
        content.title = s.title
        content.body = s.body
        content.sound = .default

        let (hour, minute) = parseTime(s.time) ?? (9, 0)
        var comps = DateComponents()
        comps.hour = hour
        comps.minute = minute
        let trigger: UNNotificationTrigger
        switch s.repeat {
        case .daily:
            trigger = UNCalendarNotificationTrigger(dateMatching: comps, repeats: true)
        case .weekly:
            comps.weekday = weekdayNumber(s.weekday)
            trigger = UNCalendarNotificationTrigger(dateMatching: comps, repeats: true)
        case .once:
            let next = Calendar.current.nextDate(after: Date(), matching: comps, matchingPolicy: .nextTime) ?? Date().addingTimeInterval(60)
            trigger = UNTimeIntervalNotificationTrigger(timeInterval: max(1, next.timeIntervalSinceNow), repeats: false)
        }
        try await center.add(UNNotificationRequest(identifier: prefix + s.id, content: content, trigger: trigger))
    }

    static func cancel(id: String) {
        UNUserNotificationCenter.current().removePendingNotificationRequests(withIdentifiers: [prefix + id])
    }

    static func parseTime(_ t: String?) -> (Int, Int)? {
        guard let t else { return nil }
        let p = t.split(separator: ":").compactMap { Int($0) }
        guard p.count == 2, (0...23).contains(p[0]), (0...59).contains(p[1]) else { return nil }
        return (p[0], p[1])
    }

    static func weekdayNumber(_ w: String?) -> Int {
        ["sun": 1, "mon": 2, "tue": 3, "wed": 4, "thu": 5, "fri": 6, "sat": 7][w ?? ""] ?? 2
    }
}
