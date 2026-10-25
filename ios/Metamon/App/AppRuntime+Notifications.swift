import Foundation
import UserNotifications
import MetamonDSL
import MetamonServices

/// 生成アプリのローカル通知(#228)。DSL の読み込み・レコードの変化・前面復帰で予約し直す。
extension AppRuntime {
    /// レコードの変化が続いても 1 回にまとめて予約し直す。
    func scheduleNotificationSync() {
        notificationSyncTask?.cancel()
        notificationSyncTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(400))
            guard let self, !Task.isCancelled else { return }
            await syncNotificationsNow()
        }
    }

    func syncNotificationsNow() async {
        let appId = document.appId
        let specs = dsl.app.notifications
        var recs: [String: [Record]] = [:]
        for entity in Set(specs.filter(\.isRecordBased).compactMap(\.entity)) {
            if let loaded = records[entity] {
                recs[entity] = loaded
            } else {
                recs[entity] = await recordSync.cached(appId: appId, entity: entity) // 未表示の entity は端末内 DB から
            }
        }
        let result = await services.notifications.sync(appId: appId, specs: specs, records: recs)
        handleNotificationResult(result, appId: appId, hasSpecs: !specs.isEmpty)
    }

    /// 前面復帰: 届いた 1 回きりの分の空き枠を次の通知で埋める。
    func refreshNotifications() async {
        let appId = document.appId
        let result = await services.notifications.refresh(appId: appId)
        handleNotificationResult(result, appId: appId, hasSpecs: !dsl.app.notifications.isEmpty)
    }

    private func handleNotificationResult(_ result: NotificationSyncResult, appId: String, hasSpecs: Bool) {
        guard document.appId == appId else { return }
        upcomingNotifications = result.upcoming
        notificationsDenied = result.denied && hasSpecs
        if notificationsDenied, !warnedNotificationsDenied {
            warnedNotificationsDenied = true
            toast = L10n.tr("通知がオフのため届きません。設定アプリの「通知」で Tailor It を許可してください")
        }
    }
}

enum NotificationCopy {
    /// 今日なら「7:00」、それ以外は「10/26(日) 7:00」(端末の言語の書式)。
    static func when(_ d: Date, now: Date = .now) -> String {
        let f = DateFormatter()
        f.locale = .current
        f.setLocalizedDateFormatFromTemplate(Calendar.current.isDate(d, inSameDayAs: now) ? "Hm" : "MdEEEHm")
        return f.string(from: d)
    }
}

/// アプリを開いている間に時刻が来た通知もバナーで出す(delegate が無いと前面では表示されない)。
final class ForegroundNotificationPresenter: NSObject, UNUserNotificationCenterDelegate {
    static let shared = ForegroundNotificationPresenter()

    func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .list, .sound]
    }
}
