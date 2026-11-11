package jp.kz2601.metamon.services.sync

import jp.kz2601.metamon.services.FetchedDocument
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** 記録を取り直してほしい範囲。 */
sealed interface RecordsChange {
    /** この entity が他端末 / 他の人で変わった */
    data class Entity(val entity: String) : RecordsChange

    /** 背面・切断の間の変化は通知されないので、表示中の entity を全部読み直す */
    data object All : RecordsChange
}

/** 画面に出すホットリロードのつながり方(#1108)。 */
enum class HotReloadStatus {
    /** 購読していない(背面・未発行のアプリ・モック) */
    OFF,

    /** SSE の購読が稼働中 */
    REALTIME,

    /** SSE がつながっていない間。ポーリングで拾っている */
    POLLING,
}

/**
 * 開いているアプリのホットリロードを回す(iOS の `AppRuntime.startSync` / `setForeground` の :services 側)。
 * 画面(:app の runtime)は [documents] と [recordsChange] を集めて当てるだけにする(つなぎ込みは別 Issue)。
 *
 * - 前面の間だけ [DocumentSync] を集める。背面で止め、前面に戻ったら手元の版から張り直す
 *   (SSE の `hello` で版を比べて、背面の間に進んだ DSL を取り直す)
 * - [documents] には手元より新しい版だけを流す(自分で生成・保存した版は [noteVersion] で知らせる)
 *
 * @param scope 購読を回す範囲(アプリの生存期間)。止めるのは [stop] か scope の cancel
 */
class HotReload(
    private val sync: DocumentSync,
    private val scope: CoroutineScope,
) {
    private val _documents = MutableSharedFlow<FetchedDocument>(extraBufferCapacity = 1, onBufferOverflow = BufferOverflow.DROP_OLDEST)

    /** 「DSL が変わった」。手元より新しい版だけ(古い版は最新に追い越されたら捨てる)。 */
    val documents: SharedFlow<FetchedDocument> = _documents.asSharedFlow()

    private val _recordsChange = MutableSharedFlow<RecordsChange>(extraBufferCapacity = 64)

    /** 「記録が変わった」。受けた側がその entity を取り直す。 */
    val recordsChange: SharedFlow<RecordsChange> = _recordsChange.asSharedFlow()

    private val _realtime = MutableStateFlow(false)

    /** SSE の購読が稼働中か。false の間はポーリングで拾っている(画面の「リアルタイム」表示用)。 */
    val realtime: StateFlow<Boolean> = _realtime.asStateFlow()

    private val _status = MutableStateFlow(HotReloadStatus.OFF)

    /** 購読しているか・SSE かポーリングか(画面の「リアルタイム / ポーリング中」の表示用・#1108)。 */
    val status: StateFlow<HotReloadStatus> = _status.asStateFlow()

    private val lock = Any()
    private var appId: String? = null
    private var version = 0
    private var foreground = true
    private var realtimeWasLost = false
    private var job: Job? = null

    /** 張り直すたびに進める。取り消した前の購読から遅れて届いた知らせを見分ける(同じアプリに張り直した時も) */
    private var generation = 0L

    /** 開いているアプリと、当てている版。別のアプリに切り替えた時も呼ぶ。 */
    fun start(appId: String, version: Int) = synchronized(lock) {
        this.appId = appId
        this.version = version
        realtimeWasLost = false
        restart()
    }

    fun stop() = synchronized(lock) {
        appId = null
        job?.cancel()
        job = null
        setRealtime(null)
    }

    /** 自分で当てた版(生成・保存・復元)。同じ版を他から受けても流さない。 */
    fun noteVersion(version: Int) = synchronized(lock) {
        if (version > this.version) this.version = version
    }

    /** 前面の時だけ購読する(Activity / ProcessLifecycle の onStart / onStop から呼ぶ)。 */
    fun setForeground(active: Boolean) = synchronized(lock) {
        if (foreground == active) return@synchronized
        foreground = active
        if (active) {
            restart()
            // 背面の間の記録の変化は届いていないので読み直してもらう
            if (appId != null) _recordsChange.tryEmit(RecordsChange.All)
            realtimeWasLost = false
        } else {
            job?.cancel()
            job = null
            setRealtime(null)
        }
    }

    /** null = 購読していない。購読を張った直後は SSE の稼働の知らせが来るまでポーリング扱い。 */
    private fun setRealtime(listening: Boolean?) {
        _realtime.value = listening == true
        _status.value = when (listening) {
            null -> HotReloadStatus.OFF
            true -> HotReloadStatus.REALTIME
            false -> HotReloadStatus.POLLING
        }
    }

    private fun restart() {
        job?.cancel()
        job = null
        val id = appId ?: return setRealtime(null)
        if (!foreground) return setRealtime(null)
        setRealtime(false)
        val from = version
        val gen = ++generation
        job = scope.launch {
            sync.events(id, from).collect { handle(id, gen, it) }
        }
    }

    private suspend fun handle(id: String, gen: Long, update: SyncUpdate) {
        when (update) {
            is SyncUpdate.Document -> {
                val accepted = synchronized(lock) {
                    val doc = update.document
                    if (id != appId || doc.document.appId != id || doc.version <= version) return@synchronized false
                    version = doc.version
                    true
                }
                if (accepted) _documents.emit(update.document)
            }
            is SyncUpdate.Records -> _recordsChange.emit(RecordsChange.Entity(update.entity))
            is SyncUpdate.Realtime -> {
                val resumed = synchronized(lock) {
                    // 止めた・張り直した後に前の購読から届いた古い知らせは捨てる(cancel は collect の途中では止まらない)
                    if (gen != generation || id != appId || !foreground || job == null) return@synchronized false
                    val resumed = update.listening && realtimeWasLost
                    realtimeWasLost = !update.listening
                    setRealtime(update.listening)
                    resumed
                }
                // 切れていた間の他の人の投稿を取りこぼさない
                if (resumed) _recordsChange.emit(RecordsChange.All)
            }
        }
    }
}
