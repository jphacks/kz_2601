package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.switcher.AppThumbnailStore
import jp.kz2601.metamon.voice.ChatHistoryStore

/**
 * 削除したアプリの端末のデータ(iOS の `deleteApp` の後片付け・#896): 記録の DB と送信待ち・保存した DSL・
 * サムネ・会話の履歴(#1176)・通知。そのアプリで位置を記録中なら先に止める(#869)。サーバーの削除が済んでから呼ぶ。
 */
class LocalAppForget(
    private val recordSync: RecordSyncEngine,
    private val documentCache: DocumentCache,
    private val thumbnails: AppThumbnailStore,
    private val chatHistory: ChatHistoryStore,
    private val forgetNotifications: (String) -> Unit = {},
    /** そのアプリの位置の記録を止める(別のアプリの記録は止めない) */
    private val stopLocationTracking: (appId: String) -> Unit = {},
) {
    suspend operator fun invoke(appId: String) {
        stopLocationTracking(appId) // 消したアプリへ点を足し続けない
        recordSync.purge(appId)
        documentCache.remove(appId)
        thumbnails.remove(appId)
        chatHistory.remove(appId)
        forgetNotifications(appId)
    }
}
