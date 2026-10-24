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
        if let id = Bundle.main.bundleIdentifier {      // 設定・appId・タイマー等の UserDefaults
            UserDefaults.standard.removePersistentDomain(forName: id)
        }
        let center = UNUserNotificationCenter.current()
        center.removeAllPendingNotificationRequests()
        center.removeAllDeliveredNotifications()
        resetToInitial()                                // 新しい appId を発行して初期 DSL に(UserDefaults を消した後に)
        toast = nil
    }
}
