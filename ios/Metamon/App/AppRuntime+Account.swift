import Foundation
import UserNotifications
import WidgetKit
import MetamonServices

extension AppRuntime {
    /// アカウント削除(#119)の後始末: この端末に残る自分のデータをすべて消し、初期状態の新しい appId に戻す。
    /// サーバーには何も送らない(削除済みユーザーのトークンで 401 にならないように)。
    func wipeLocalData() async {
        await recordSync.wipe()                         // オフライン DB(metamon-records)と未送信キュー
        DocumentCache.shared.removeAll()                // DSL キャッシュとアプリ一覧(metamon-dsl)
        AppThumbnailStore.shared.removeAll()            // 変身一覧のサムネ(metamon-thumbs)
        WidgetSnapshot.defaults?.removePersistentDomain(forName: WidgetSnapshot.appGroupID) // App Group(ウィジェット)
        WidgetCenter.shared.reloadAllTimelines()
        // 設定・appId・タイマー等の UserDefaults。removePersistentDomain は直後の書き込み(新 appId)が
        // 永続化されないことがあったので、キーを 1 つずつ消す
        let defaults = UserDefaults.standard
        if let id = Bundle.main.bundleIdentifier, let keys = defaults.persistentDomain(forName: id)?.keys {
            keys.forEach(defaults.removeObject(forKey:))
        }
        let center = UNUserNotificationCenter.current()
        await services.notifications.removeAll()        // アプリごとの予約(#228)
        center.removeAllPendingNotificationRequests()
        center.removeAllDeliveredNotifications()
        resetToInitial()                                // 新しい appId を発行して初期 DSL に(UserDefaults を消した後に)
        chatHistoryStore.removeAll()                    // チャット履歴(#210)。resetToInitial が閉じた会話を書くので後で消す
        chatHistory = []
        DeviceAppID.set(document.appId)                 // 念のため保存し直す(次回起動で別 appId にならないように)
        toast = nil
    }
}
