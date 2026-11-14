package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.switcher.AppThumbnailStore
import jp.kz2601.metamon.voice.ChatHistoryStore

/**
 * 削除したアプリの端末のデータ(iOS の `deleteApp` の後片付け・#896): 記録の DB と送信待ちと記録の動画・録音のファイル(#1098)・保存した DSL・
 * サムネ・会話の履歴(#1176)・通知・ホーム画面のウィジェット(#912)・フォームの書きかけ(#1185)。そのアプリで位置を記録中なら先に止める(#869)。サーバーの削除が済んでから呼ぶ。
 */
class LocalAppForget(
    private val recordSync: RecordSyncEngine,
    private val documentCache: DocumentCache,
    private val thumbnails: AppThumbnailStore,
    private val chatHistory: ChatHistoryStore,
    private val forgetNotifications: (String) -> Unit = {},
    /** そのアプリの位置の記録を止め、保存の途中の 1 点を待つ(別のアプリの記録は止めない) */
    private val stopLocationTracking: suspend (appId: String) -> Unit = {},
    /** ホーム画面のウィジェットがそのアプリを出していれば消す(#912) */
    private val forgetWidget: (appId: String) -> Unit = {},
    private val forgetFormDrafts: (String) -> Unit = {},
) {
    suspend operator fun invoke(appId: String) {
        stopLocationTracking(appId) // 消したアプリへ点を足し続けない(書きかけの点を待ってから消す)
        recordSync.purge(appId)
        documentCache.remove(appId)
        thumbnails.remove(appId)
        chatHistory.remove(appId)
        forgetNotifications(appId)
        forgetWidget(appId)
        forgetFormDrafts(appId)
    }
}
