package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.consent.CrashReportingPreference
import jp.kz2601.metamon.services.local.BackoffPolicy
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.local.RecordStore
import jp.kz2601.metamon.services.local.SyncQueue
import java.io.File

/**
 * 端末に残すもの一式(#1754)。中身の決まりは android/services と同じソース(iOS と同じ形)。
 *
 * ```
 * <AppDataDir>/
 *   settings.json     設定(最後に開いたアプリなど)
 *   consent.json      AI への送信の同意・クラッシュ報告(鍵は iOS・Android と同じ)
 *   records/<appId>/<entity>.json   記録のキャッシュ(未送信の印つき)
 *   queue/queue.json  送信待ちの列(通信が切れても消えない・再起動後に送る)
 *   documents/        取得した DSL の封筒(オフライン起動)
 *   thumbnails/<appId>.png  アプリの最後に見た画面の写し(#3203)
 * ```
 *
 * @param root null ならメモリだけ(テスト用)
 */
class DesktopStore(val root: File?) {
    val settings = JsonKeyValueStore(root?.resolve(SETTINGS_FILE))
    private val consentValues = JsonKeyValueStore(root?.resolve(CONSENT_FILE))
    val aiConsent = AiConsentStore(consentValues)
    val crashReporting = CrashReportingPreference(consentValues)
    val records = RecordStore(root?.resolve("records"))
    val documents = DocumentCache(root?.resolve("documents"))
    val thumbnails = ThumbnailStore(root?.resolve(ThumbnailStore.DIR_NAME))

    private val queueDir: File? = root?.resolve("queue")
    private var queue: SyncQueue? = null

    /** 送信待ちの列。送り先が決まってから作る(モックの間は作らない)。2 回目以降は同じ列を返す。 */
    @Synchronized
    fun queue(client: AppConfigClient, policy: BackoffPolicy = BackoffPolicy()): SyncQueue =
        queue ?: SyncQueue(queueDir, records, client, policy).also { queue = it }

    /** 最後に開いたアプリ(次の起動でそこから開く)。 */
    var lastAppId: String?
        get() = settings.getString(LAST_APP_ID)
        set(value) = if (value == null) settings.remove(LAST_APP_ID) else settings.putString(LAST_APP_ID, value)

    /**
     * サインアウト・アカウント削除・別の人のサインイン(Android の後片付けと同じ範囲)。
     * 記録・送信待ち・DSL・画面の写し・設定・AI の同意(人ごと)を消す。クラッシュ報告の設定(端末ごと)とウィンドウの位置は残す。
     */
    suspend fun wipe() {
        queue?.discardAll() ?: queueDir?.deleteRecursively()
        records.removeEverything()
        documents.removeAll()
        thumbnails.removeAll()
        settings.clear()
        aiConsent.forget()
        sweepTempFiles(root)
    }

    companion object {
        const val SETTINGS_FILE = "settings.json"
        const val CONSENT_FILE = "consent.json"
        const val LAST_APP_ID = "tailorit.lastAppId"

        /** 開く前に、前に落ちた時の一時ファイル(`<名前>.<数字>.tmp`・`<名前>.tmp`)を消す。 */
        fun open(dir: File = AppDataDir.resolve()): DesktopStore {
            sweepTempFiles(dir)
            return DesktopStore(dir)
        }
    }
}
