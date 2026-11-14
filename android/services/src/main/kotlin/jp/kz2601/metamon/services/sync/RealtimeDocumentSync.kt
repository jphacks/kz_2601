package jp.kz2601.metamon.services.sync

import jp.kz2601.metamon.services.FetchedDocument
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.channels.ProducerScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull

/**
 * リアルタイム版のホットリロード(iOS の `RealtimeDocumentSync`・G13 Day2・#82)。
 * `GET /v1/apps/{appId}/events`(SSE)を購読し、届くのは「何が変わったか」だけ。本体は従来の API で取り直す(正は API)。
 * - `hello {version}` / `dsl {version}` → 手元より新しければ `GET /dsl`(If-None-Match)で取り直して [SyncUpdate.Document]
 * - `records {entity}` → [SyncUpdate.Records](呼び出し側が取り直す)
 * - `bye` → サーバーの予定終了。すぐ張り直し、ポーリングには落とさない
 * - それ以外の切断 → [SyncUpdate.Realtime] false + 指数の間隔で張り直す。その間は [fallbackIntervalMillis] でポーリング
 * - 401 → 次の張り直しでトークンを取り直す
 *
 * ポーリングは購読中も [listeningIntervalMillis](60 秒)で保険として回す。
 * 購読が切れたら待ちを打ち切ってすぐ [fallbackIntervalMillis] の間隔に切り替える(iOS は次の周期まで 60 秒のまま)。
 */
class RealtimeDocumentSync(
    private val fetcher: DocumentFetcher,
    private val source: ChangeEventSource,
    private val fallbackIntervalMillis: Long = 5_000,
    private val listeningIntervalMillis: Long = 60_000,
    private val initialBackoffMillis: Long = 1_000,
    private val maxBackoffMillis: Long = 30_000,
    private val log: (String) -> Unit = {},
) : DocumentSync {

    override fun events(appId: String, afterVersion: Int): Flow<SyncUpdate> = channelFlow {
        val state = State(afterVersion, this)
        launch { listen(appId, state) }
        launch { poll(appId, state) }
    }

    /** 購読とポーリングで共有する「手元の版」と「購読中か」。版は進む方向にしか動かさない。 */
    private class State(version: Int, private val out: ProducerScope<SyncUpdate>) {
        private val lock = Mutex()
        @Volatile
        var version: Int = version
            private set
        val listening = MutableStateFlow(false)

        suspend fun setListening(value: Boolean) = lock.withLock {
            if (listening.value == value) return@withLock
            listening.value = value
            out.send(SyncUpdate.Realtime(value))
        }

        suspend fun offer(doc: FetchedDocument) = lock.withLock {
            if (doc.version <= version) return@withLock // 購読とポーリングが同時に取っても 1 回だけ流す
            version = doc.version
            out.send(SyncUpdate.Document(doc))
        }

        suspend fun recordsChanged(entity: String) = out.send(SyncUpdate.Records(entity))
    }

    private suspend fun refetch(appId: String, state: State) {
        try {
            fetcher.fetchIfChanged(appId, state.version)?.let { state.offer(it) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("realtime: refetch failed: $e")
        }
    }

    // ポーリング(予備)

    private suspend fun poll(appId: String, state: State) {
        while (true) {
            val listening = state.listening.value
            val interval = if (listening) listeningIntervalMillis else fallbackIntervalMillis
            // 待っている間に購読の状態が変わったら、新しい間隔で待ち直す
            val changed = withTimeoutOrNull(interval) { state.listening.first { it != listening } }
            if (changed != null) continue
            refetch(appId, state)
        }
    }

    // SSE

    private suspend fun listen(appId: String, state: State) {
        var backoff = initialBackoffMillis
        var forceRefresh = false
        while (true) {
            var planned = false
            try {
                planned = source.stream(appId, forceRefresh) { event ->
                    when (event) {
                        is ChangeEvent.Hello -> {
                            backoff = initialBackoffMillis
                            state.setListening(true)
                            // 背面・切断の間に進んだ版を取りこぼさない
                            if ((event.version ?: 0) > state.version) refetch(appId, state)
                        }
                        is ChangeEvent.Dsl -> if ((event.version ?: 0) > state.version) refetch(appId, state)
                        is ChangeEvent.Records -> state.recordsChanged(event.entity)
                        ChangeEvent.Bye -> Unit
                    }
                }
                forceRefresh = false
            } catch (e: CancellationException) {
                throw e
            } catch (e: StreamUnauthorized) {
                forceRefresh = true
            } catch (e: Exception) {
                log("realtime: stream ended: $e")
            }
            if (planned) continue
            state.setListening(false)
            delay(backoff)
            backoff = (backoff * 2).coerceAtMost(maxBackoffMillis)
        }
    }
}
