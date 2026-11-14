import Foundation
import UserNotifications
import MetamonDSL
import MetamonRenderer
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
        for entity in Set(specs.compactMap(\.recordEntity)) {
            if let loaded = records[entity] {
                recs[entity] = loaded
            } else {
                recs[entity] = await recordSync.cached(appId: appId, entity: entity) // 未表示の entity は端末内 DB から
            }
        }
        let result = await services.notifications.sync(appId: appId, specs: specs, records: recs)
        handleNotificationResult(result, appId: appId, hasSpecs: !specs.isEmpty)
    }

    /// 条件の通知(#265): レコードの保存・削除・同期が続いても 1 回にまとめて評価する。
    func scheduleConditionCheck() {
        conditionCheckTask?.cancel()
        conditionCheckTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(300))
            guard let self, !Task.isCancelled else { return }
            await checkConditionNotificationsNow()
        }
    }

    /// 各 `when` を評価し、偽 → 真に変わった通知をすぐ出す。前回の真偽は `ConditionNotificationStore` に残る。
    /// 未表示の entity は端末内 DB から読む(読み込み前の空で評価して誤って鳴らさない)。
    func checkConditionNotificationsNow() async {
        let appId = document.appId
        let dsl = self.dsl
        let specs = dsl.app.notifications.filter(\.isConditional)
        guard !specs.isEmpty else { return }
        var recs: [String: [Record]] = [:]
        for entity in dsl.entities.map(\.id) {
            if let loaded = records[entity] {
                recs[entity] = loaded
            } else {
                recs[entity] = await recordSync.cached(appId: appId, entity: entity)
            }
        }
        guard document.appId == appId else { return }
        let current = ConditionNotifications.evaluate(specs, dsl: dsl, records: recs)
        let fire = await ConditionNotificationStore.shared.update(appId: appId, current: current)
        for spec in specs where fire.contains(spec.id) {
            let posted = await services.notifications.post(spec, appId: appId)
            print("[Metamon] condition notification id=\(spec.id) posted=\(posted)")
        }
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
        // 許可が無い時は上のバー(`NotificationsOffBarHost`・設定を開くボタン付き)で伝える。トーストは 3 秒で消えるのでやめた(L3)
        notificationsDenied = result.denied && hasSpecs
        notificationPlaces = result.places
        // 場所の通知の位置情報の許可は聞き直さない・バーも出さない(設定画面の通知の欄に書くだけ)
        placesLocationOff = result.locationOff && hasSpecs
    }
}

/// アプリを開いている間に時刻が来た通知もバナーで出す(delegate が無いと前面では表示されない)。
final class ForegroundNotificationPresenter: NSObject, UNUserNotificationCenterDelegate {
    static let shared = ForegroundNotificationPresenter()

    func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        // 終わったタイマーが画面に見えている時は出さない(00:00 が見えている)。見えていなければ出す(#38)
        let id = notification.request.identifier
        if await MainActor.run(body: { TimerVisibility.isShowing(notificationId: id) }) { return [] }
        return [.banner, .list, .sound]
    }

    /// 通知の応答。ボタン・文字入力(#2231)は `NotificationReplyInbox` へ。通知そのもののタップは、
    /// プッシュなら本文の appId のアプリを開く(#269)。ローカル通知には appId が無いので今までどおり何もしない。
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
        let reply = NotificationReply(response: response)
        await NotificationReplyInbox.shared.receive(reply)
        #if !APP_STORE_REVIEW
        guard reply.isPlainOpen, let appId = PushPayload.appId(from: response.notification.request.content.userInfo) else { return }
        await PushHost.shared.open(appId: appId)
        #endif
    }
}
