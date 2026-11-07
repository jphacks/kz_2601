import Foundation
import UserNotifications
import WidgetKit
import MetamonRenderer
import MetamonServices

extension AppRuntime {
    /// アカウント削除(#119)・ログアウトの後始末: この端末に残る自分のデータをすべて消し、初期状態の新しい appId に戻す。
    /// サーバーには何も送らない(削除済みユーザーのトークンで 401 にならないように)。
    func wipeLocalData() async {
        #if !APP_STORE_REVIEW
        LocationTrackingControl.stop()                  // 位置の記録(#168)を止める(青いインジケータを残さない)
        #endif
        recordUndo = nil                                // 「元に戻す」(M11)を次の人に出さない
        await recordSync.wipe()                         // オフライン DB(metamon-records)と未送信キュー・取り消し待ちの削除
        DocumentCache.shared.removeAll()                // DSL キャッシュとアプリ一覧(metamon-dsl)
        AppThumbnailStore.shared.removeAll()            // 変身一覧のサムネ(metamon-thumbs)
        ExportFiles.removeAll()                         // 書き出した CSV / PDF(#421・記録の写し)
        WidgetSnapshot.defaults?.removePersistentDomain(forName: WidgetSnapshot.appGroupID) // App Group(ウィジェット)
        WidgetCenter.shared.reloadAllTimelines()
        LiveActivities.enqueue { await LiveActivities.endAll() } // ロック画面・Dynamic Island(#423)
        LocalWipe.clearDefaults(.standard, domain: Bundle.main.bundleIdentifier)
        RemainingTodayStore.shared.clear()              // 今日の残り(#630・前のアカウントの数を出さない)
        aiConsent?.reload()                             // 同意(#246)・チュートリアル(#274)の画面の状態を消した後に合わせる
        tutorial?.reload()
        let center = UNUserNotificationCenter.current()
        await services.notifications.removeAll()        // アプリごとの予約(#228)
        center.removeAllPendingNotificationRequests()
        center.removeAllDeliveredNotifications()
        apps = []                                       // 変身一覧(オフラインだと前のアカウントの一覧が残る)
        resetToInitial()                                // 新しい appId を発行して初期 DSL に(UserDefaults を消した後に)
        chatHistoryStore.removeAll()                    // チャット履歴(#210)。resetToInitial が閉じた会話を書くので後で消す
        chatHistory = []
        DeviceAppID.set(document.appId)                 // 念のため保存し直す(次回起動で別 appId にならないように)
        toast = nil
    }
}

/// 端末のデータ削除のうち UserDefaults の分。
enum LocalWipe {
    /// 消さずに残すキー: 個人のデータではない端末の設定(表示の言語)と、本人が選んだ送信のオプトアウト
    /// (クラッシュ報告 #296・AI ボタンの利用状況 #446。消すと、画面はオフのまま次の起動で黙ってオンに戻る)と、端末の移行の印。
    static let preservedKeys: Set<String> = [
        "metamon.appLanguage",
        "metamon.crashReporting.enabled",
        "metamon.aiButtonMetrics.enabled",
        "metamon.push.installationId", // 端末ごとに安定させる(#269)。個人のデータではない
        TimerKeyMigration.doneKey, // タイマーのキーの移行(#623)の印。消すと次の起動で移行がもう一度走る(データではない)
    ]

    /// removePersistentDomain は直後の書き込み(新 appId)が永続化されないことがあったので、キーを 1 つずつ消す。
    static func clearDefaults(_ defaults: UserDefaults, domain: String?) {
        guard let domain, let keys = defaults.persistentDomain(forName: domain)?.keys else { return }
        for key in keys where !preservedKeys.contains(key) {
            defaults.removeObject(forKey: key)
        }
    }
}
