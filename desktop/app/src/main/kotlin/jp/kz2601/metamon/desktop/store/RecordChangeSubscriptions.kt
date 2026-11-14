package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.services.FetchedDocument
import jp.kz2601.metamon.services.sync.DocumentSync
import jp.kz2601.metamon.services.sync.HotReload
import jp.kz2601.metamon.services.sync.RecordsChange
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * 開いているアプリごとに変化の通知(SSE・#2808)を 1 本張る。iOS の `RealtimeDocumentSync` の受け側と同じく、
 * `records{entity}` を [batchMillis] まとめ、表示中の entity だけ [refetch] で取り直す(取り直しと合わせ方は #2448)。
 *
 * - 同じアプリを何枚のウィンドウで開いても 1 本([open] は数える・[close] で 0 になったらやめる)
 * - 端末だけのアプリ([onServer] が false)は張らない
 * - スリープ明け・ネットワークの復帰は [reconnectAll] で張り直し、表示中を全部取り直す
 *
 * @param version 手元の DSL の版(張った時の `hello` で比べる)
 * @param visible いま表示している entity
 * @param onDocument DSL の版が進んだ(#2809 で差し替える)
 */
class RecordChangeSubscriptions(
    private val sync: DocumentSync,
    private val scope: CoroutineScope,
    private val onServer: (appId: String) -> Boolean,
    private val version: (appId: String) -> Int,
    private val visible: (appId: String) -> Set<String>,
    private val refetch: suspend (appId: String, entities: Set<String>) -> Unit,
    private val onDocument: suspend (appId: String, document: FetchedDocument) -> Unit = { _, _ -> },
    private val batchMillis: Long = BATCH_MILLIS,
    private val log: (String) -> Unit = { System.err.println("[TailorIt] $it") },
) {
    private class Sub(val hot: HotReload, val jobs: List<Job>) {
        var refs = 1
        val lock = Mutex()
        val pending = mutableSetOf<String>()
        var all = false
        var flush: Job? = null
    }

    private val subs = HashMap<String, Sub>()

    /** 購読しているアプリ(テスト・表示用)。 */
    @Synchronized
    fun active(): Set<String> = subs.keys.toSet()

    /** ウィンドウを 1 枚開いた。最初の 1 枚で張る。 */
    @Synchronized
    fun open(appId: String) {
        subs[appId]?.let {
            it.refs++
            return
        }
        if (!onServer(appId)) return
        val hot = HotReload(sync, scope)
        lateinit var sub: Sub
        val jobs = listOf(
            scope.launch { hot.recordsChange.collect { enqueue(appId, sub, it) } },
            scope.launch { hot.documents.collect { doc -> runSafely("dsl") { onDocument(appId, doc) } } },
        )
        sub = Sub(hot, jobs)
        subs[appId] = sub
        hot.start(appId, version(appId))
    }

    /** ウィンドウを 1 枚閉じた。そのアプリの最後の 1 枚で購読をやめる。 */
    @Synchronized
    fun close(appId: String) {
        val sub = subs[appId] ?: return
        if (--sub.refs > 0) return
        subs.remove(appId)
        stop(sub)
    }

    /** 自分で当てた版(保存・差し替え)。同じ版の通知で取り直さない。 */
    @Synchronized
    fun noteVersion(appId: String, version: Int) {
        subs[appId]?.hot?.noteVersion(version)
    }

    /** スリープ明け・ネットワークの復帰: 張り直し、その間に届かなかった変化のために表示中を全部取り直す。 */
    @Synchronized
    fun reconnectAll() {
        subs.values.forEach {
            it.hot.setForeground(false)
            it.hot.setForeground(true)
        }
    }

    /** 終わる時。 */
    @Synchronized
    fun closeAll() {
        subs.values.forEach(::stop)
        subs.clear()
    }

    private fun stop(sub: Sub) {
        sub.hot.stop()
        sub.jobs.forEach { it.cancel() }
        sub.flush?.cancel()
    }

    private suspend fun enqueue(appId: String, sub: Sub, change: RecordsChange) = sub.lock.withLock {
        when (change) {
            is RecordsChange.Entity -> sub.pending += change.entity
            RecordsChange.All -> sub.all = true
        }
        if (sub.flush?.isActive == true) return@withLock
        sub.flush = scope.launch {
            delay(batchMillis)
            val entities = sub.lock.withLock {
                val shown = visible(appId)
                val picked = if (sub.all) shown else sub.pending intersect shown
                sub.pending.clear()
                sub.all = false
                picked
            }
            if (entities.isNotEmpty() && isActive) runSafely("refetch") { refetch(appId, entities) }
        }
    }

    private suspend fun runSafely(what: String, block: suspend () -> Unit) {
        try {
            block()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("live $what failed: ${e::class.simpleName}")
        }
    }

    companion object {
        /** iOS と同じ 250ms */
        const val BATCH_MILLIS = 250L
    }
}

/**
 * スリープ明け・ネットワークの復帰を見る(#2808)。JVM には知らせが無いので [tick] を一定の間隔で呼び、
 * 時計が間隔より大きく飛んだ(スリープしていた)か、つながっているネットワークの組が変わった時に true を返す。
 */
class ResumeDetector(
    private val now: () -> Long = System::currentTimeMillis,
    private val networks: () -> Set<String> = ::upNetworks,
    private val wakeGapMillis: Long = WAKE_GAP_MILLIS,
) {
    private var last = now()
    private var nets = networks()

    /** 張り直すべきなら true。 */
    fun tick(): Boolean {
        val t = now()
        val n = networks()
        val woke = t - last > wakeGapMillis
        // 無い → ある・別のネットワークに替わった時。切れただけ(ある → 無い)は張り直さない(張れない)
        val changed = n != nets && n.isNotEmpty()
        last = t
        nets = n
        return woke || changed
    }

    companion object {
        const val TICK_MILLIS = 5_000L
        const val WAKE_GAP_MILLIS = 30_000L

        /** 上がっているネットワーク(ループバックを除く)の名前。読めなければ空。 */
        fun upNetworks(): Set<String> = runCatching {
            java.net.NetworkInterface.getNetworkInterfaces()?.toList().orEmpty()
                .filter { it.isUp && !it.isLoopback && !it.isVirtual }
                .map { it.name }
                .toSet()
        }.getOrDefault(emptySet())

        /** [scope] の間 [ResumeDetector] を回し、張り直す時に [onResume]。 */
        fun watch(scope: CoroutineScope, detector: ResumeDetector = ResumeDetector(), onResume: () -> Unit): Job = scope.launch {
            while (isActive) {
                delay(TICK_MILLIS)
                if (detector.tick()) onResume()
            }
        }
    }
}
