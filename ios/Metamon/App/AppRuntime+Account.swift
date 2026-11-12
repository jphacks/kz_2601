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
        AppGlanceStore.shared.clear()                   // 一覧のひと目の数字(link-10・前の人の数字を残さない)
        ExportFiles.removeAll()                         // 書き出した CSV / PDF(#421・記録の写し)
        RemoteImageLoader.removeAllCached()             // https の画像のキャッシュ(#1265・前の人が見た画像を残さない)
        URLCache.shared.removeAllCachedResponses()      // 共有の URL キャッシュも
        IntentHost.shared.cancelPendingRefresh()        // 前の人の一覧をコントロール(#1993)の一覧に書き戻さない
        WidgetSnapshot.defaults?.removePersistentDomain(forName: WidgetSnapshot.appGroupID) // App Group(ウィジェット・コントロールの一覧)
        WidgetCenter.shared.reloadAllTimelines()
        ControlIntentBridge.reloadControls()            // コントロールの見出し(前の人のアプリ名)を消す
        LiveActivities.enqueue { await LiveActivities.endAll() } // ロック画面・Dynamic Island(#423)
        LocalWipe.clearDefaults(.standard, domain: Bundle.main.bundleIdentifier)
        RemainingTodayStore.shared.clear()              // 今日の残り(#630・前のアカウントの数を出さない)
        PlanUsageStore.shared.clear()                   // 使っている量(#317)
        #if !APP_STORE_REVIEW
        CoinGate.shared.clear()                         // コインの残高(#630・前のアカウントの数を出さない)
        MatchingUI.shared.wipe()                        // 仲間探しの 18 歳以上の申告を送った記録・一覧(#507)
        FeedCache.shared.reset()                        // feed の覚えた行(#1318・diet の検索語から引いた結果を次の人に出さない)
        #endif
        aiConsent?.reload()                             // 同意(#246)・チュートリアル(#274)の画面の状態を消した後に合わせる
        tutorial?.reload()
        coachMarks?.reload()                            // 操作の吹き出し(#1441)
        let center = UNUserNotificationCenter.current()
        await services.notifications.removeAll()        // アプリごとの予約(#228)
        await spotlight.removeAll()                     // Spotlight のアプリ名(#1996)
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
