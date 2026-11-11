import Foundation
import UserNotifications
import MetamonRenderer
import MetamonServices

/// サインアウト・アカウント削除の後始末(#972)。iOS の `AppRuntime.wipeLocalData` の Mac 版で、この Mac に残る自分のデータを消す。
/// サーバーには何も送らない。Firebase のログインの状態(Keychain)は `AuthService.signOut` が消す。
/// 記録の暗号の鍵(iCloud キーチェーン・#233)は iOS と同じく消さない(同じ Apple ID のほかの端末が使う)。
/// Mac は会話の履歴を端末に残さない(会話のシートを閉じると消える)ので、消す物は無い。
@MainActor
struct MacLocalWipe {
    /// 作ったアプリの記録(Application Support/metamon-mac-records)。開いているエンジンを止めてから消す
    var records: MacRecordEngines = .shared
    /// サイドバーの写し(metamon-thumbs)
    var thumbnails: MacAppThumbnailStore = .shared
    /// 作ったアプリの DSL・一覧(metamon-dsl・`LocalAppLibrary`)
    var documents: DocumentCache = .shared
    var appList: AppListCache = .shared
    #if !APP_STORE_REVIEW
    /// 外部の公開データ(#1288)
    var feeds: FeedCache = .shared
    #endif
    var defaults: UserDefaults = .standard
    var domain: String? = Bundle.main.bundleIdentifier
    /// ウインドウをまたいで 1 つの一覧(メモリ上の版の履歴も消す)
    var library: MacAppLibrary?
    /// 今日の残り・使っている量・同意・使い方の画面の状態(UserDefaults を消した後に合わせ直す)・書き出し・一時フォルダ
    var resetSharedState: @MainActor () -> Void = MacLocalWipe.resetSharedState
    /// 予約した通知と届いた通知
    var removeNotifications: @MainActor () async -> Void = MacLocalWipe.removeAllNotifications

    func run() async {
        await records.purgeAll()
        documents.removeAll()
        appList.clear()
        thumbnails.removeAll()
        #if !APP_STORE_REVIEW
        feeds.reset()
        #endif
        LocalWipe.clearDefaults(defaults, domain: domain) // iOS と同じ残すキー(言語・送信のオプトアウト)
        resetSharedState()
        await removeNotifications()
        library?.resetAfterWipe()
    }

    static func resetSharedState() {
        RemainingTodayStore.shared.clear()
        PlanUsageStore.shared.clear()
        AIConsentStore.shared.reload()
        FirstRunTutorialStore.shared.reload()
        ExportFiles.removeAll()
        MacAppRuntime.removePreviewDirectories()
    }

    static func removeAllNotifications() async {
        let center = UNUserNotificationCenter.current()
        center.removeAllPendingNotificationRequests()
        center.removeAllDeliveredNotifications()
    }
}
