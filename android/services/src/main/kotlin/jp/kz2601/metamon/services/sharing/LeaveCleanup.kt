package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.services.AppSummary
import kotlinx.coroutines.CancellationException

/**
 * 共有から抜けた後の後片付け(#903)。
 * 1. 今そのアプリを開いていれば、一覧の先頭の別のアプリへ切り替える(無い・読めなければ新しいアプリ)。
 *    先に離れるのは、離れる前に消すと同期の `GET /dsl` が同じ appId を空のアプリとして作り直すため(iOS のアプリ削除と同じ順)
 * 2. 一覧から外し、自分のコピー(`DELETE /v1/apps/{appId}`。参加者が呼ぶと自分の木だけ消え、共有の記録は残る)を消す
 * 3. 端末に残った記録・送信待ち・会話の履歴・通知・DSL のキャッシュを消す
 * 2・3 は失敗しても続ける(抜けること自体は済んでいる)。
 */
class LeaveCleanup(
    private val currentAppId: () -> String,
    private val apps: suspend () -> List<AppSummary>,
    private val switchApp: suspend (String) -> Boolean,
    private val startNewApp: () -> Unit,
    private val hideApp: (String) -> Unit,
    private val deleteCopy: suspend (String) -> Unit,
    private val purgeLocal: suspend (String) -> Unit,
) {
    suspend fun run(appId: String) {
        if (currentAppId() == appId) {
            val next = attempt { AppSummary.latest(apps(), excluding = appId) }
            if (next == null || attempt { switchApp(next.id) } != true || currentAppId() == appId) startNewApp()
        }
        hideApp(appId)
        attempt { deleteCopy(appId) }
        attempt { purgeLocal(appId) }
    }

    private inline fun <T> attempt(block: () -> T): T? = try {
        block()
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        null
    }
}
