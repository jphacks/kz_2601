import Foundation
import UserNotifications
import MetamonRenderer
import MetamonServices

/// サインアウト・アカウント削除の後始末(#972)。iOS の `AppRuntime.wipeLocalData` の Mac 版で、この Mac に残る自分のデータを消す。
/// サーバーには何も送らない。Firebase のログインの状態(Keychain)は `AuthService.signOut` が消す。
/// 記録の暗号の鍵(iCloud キーチェーン・#233)は iOS と同じく消さない(同じ Apple ID のほかの端末が使う)。
/// 会話の履歴(#1211・アプリごとの `ChatHistoryStore`)も消す。
@MainActor
struct MacLocalWipe {
    /// 作ったアプリの記録(Application Support/metamon-mac-records)。開いているエンジンを止めてから消す
    var records: MacRecordEngines = .shared
    /// サイドバーの写し(metamon-thumbs)
    var thumbnails: MacAppThumbnailStore = .shared
    /// 作ったアプリの DSL・一覧(metamon-dsl・`LocalAppLibrary`)
    var documents: DocumentCache = .shared
    /// サーバーの一覧のキャッシュ(`GET /v1/apps`・#1057)
    var appList: AppListCache = .shared
    /// 作ったアプリの端末の一覧(`LocalAppLibrary`)。空で置き直す(無いと次の起動で前の置き場から移し直すため)
    var localList: AppListCache = LocalAppLibrary.standardList
    #if !APP_STORE_REVIEW
    /// 外部の公開データ(#1288)
    var feeds: FeedCache = .shared
    #endif
    /// 会話の履歴(#1211)。UserDefaults の掃除でも消えるが、保存先を差し替えた時も残さないよう明示して消す
    var chatHistory: ChatHistoryStore = .forRuntime()
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
        localList.store([])
        thumbnails.removeAll()
        chatHistory.removeAll()
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
