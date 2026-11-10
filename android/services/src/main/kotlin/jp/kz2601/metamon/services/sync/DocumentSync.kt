package jp.kz2601.metamon.services.sync

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.FetchedDocument
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flow

/**
 * ホットリロード(iOS の `DocumentSync`)。他の端末・共有の相手の変更を流す。
 * Flow を集めている間だけ動く(背面で止めるのは集める側が cancel する。[HotReload] を参照)。
 */
fun interface DocumentSync {
    /** [afterVersion] より新しい DSL・記録の変化・リアルタイム購読の稼働/停止を流す。 */
    fun events(appId: String, afterVersion: Int): Flow<SyncUpdate>
}

/** [DocumentSync.events] が流すもの(iOS の `SyncUpdate`)。 */
sealed interface SyncUpdate {
    /** 版が進んだ DSL(appconfig から取り直した本体) */
    data class Document(val document: FetchedDocument) : SyncUpdate

    /** この entity の記録が他端末 / 他の人で変わった(本体は呼び出し側が取り直す) */
    data class Records(val entity: String) : SyncUpdate

    /** リアルタイム購読が稼働した(true)/ 切れてポーリングに落ちた(false)。値が変わった時だけ流す */
    data class Realtime(val listening: Boolean) : SyncUpdate
}

/** DSL の取り直し(`If-None-Match` 付き)。テストで差し替えるために [AppConfigClient] から切り出す。 */
fun interface DocumentFetcher {
    /** 304 か同じ版なら null。 */
    suspend fun fetchIfChanged(appId: String, currentVersion: Int): FetchedDocument?
}

fun AppConfigClient.asDocumentFetcher(): DocumentFetcher = DocumentFetcher(::fetchDocumentIfChanged)

/** 何も流さない(モック・オフラインのデモ用。iOS の `NoopDocumentSync`)。 */
object NoopDocumentSync : DocumentSync {
    override fun events(appId: String, afterVersion: Int): Flow<SyncUpdate> = emptyFlow()
}

/**
 * ポーリングだけのホットリロード(iOS の `PollingDocumentSync`)。
 * [intervalMillis] ごとに `If-None-Match` 付きで取り直し、版が進んだ時だけ流す。失敗は次の周期でやり直す。
 */
class PollingDocumentSync(
    private val fetcher: DocumentFetcher,
    private val intervalMillis: Long = 5_000,
    private val log: (String) -> Unit = {},
) : DocumentSync {
    override fun events(appId: String, afterVersion: Int): Flow<SyncUpdate> = flow {
        var last = afterVersion
        while (true) {
            delay(intervalMillis)
            val doc = try {
                fetcher.fetchIfChanged(appId, last)
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                log("poll failed: $e")
                null
            }
            if (doc != null && doc.version > last) {
                last = doc.version
                emit(SyncUpdate.Document(doc))
            }
        }
    }
}
