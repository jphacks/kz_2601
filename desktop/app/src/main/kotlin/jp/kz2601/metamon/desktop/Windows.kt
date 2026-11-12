package jp.kz2601.metamon.desktop

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.desktop.store.SessionPersistence
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.records.Entity
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull

// 複数のウィンドウ(#1763): アプリごとに 1 枚・記録の詳細を別の 1 枚。画面の状態(遷移・下書き・キー)はウィンドウごと、記録はアプリごとに 1 つを共有する。

/** ウィンドウの種類。大きさ・位置はこの種類ごとに覚える。 */
enum class WindowKind { APP, RECORD }

/** 何を開いているウィンドウか。同じ物をもう一度開くと、新しく作らず前に出す。 */
data class WindowKey(val appId: String, val screen: String? = null, val recordId: String? = null) {
    val kind: WindowKind get() = if (recordId == null) WindowKind.APP else WindowKind.RECORD

    companion object {
        fun app(appId: String) = WindowKey(appId)
        fun record(appId: String, screen: String, recordId: String) = WindowKey(appId, screen, recordId)
    }
}

/** 開いているウィンドウ 1 枚。[focusRequests] が増えたら前に出す。 */
class WindowEntry(val id: Int, val key: WindowKey) {
    var focusRequests by mutableIntStateOf(0)
        internal set
}

/** 開いているウィンドウの一覧(画面に頼らない。テストはこれを動かす)。 */
class WindowRegistry {
    private var nextId = 1
    val windows = mutableStateListOf<WindowEntry>()

    /** 最後に前に出たウィンドウ(トレイのすぐ記録・トレイの「開く」の行き先)。 */
    var activeId: Int? by mutableStateOf(null)
        private set

    val active: WindowEntry? get() = windows.firstOrNull { it.id == activeId } ?: windows.lastOrNull()

    /** 前に出た順(後ろほど新しい)。 */
    private val recency = mutableListOf<Int>()

    private fun touch(id: Int) {
        recency.remove(id)
        recency += id
        activeId = id
    }

    fun find(key: WindowKey): WindowEntry? = windows.firstOrNull { it.key == key }

    /** 開く。同じアプリ・同じ記録のウィンドウがあれば作らずにそれを前に出す。 */
    fun open(key: WindowKey): WindowEntry {
        find(key)?.let { e ->
            e.focusRequests++
            touch(e.id)
            return e
        }
        return WindowEntry(nextId++, key).also {
            windows += it
            touch(it.id)
        }
    }

    fun activate(id: Int) {
        if (windows.any { it.id == id }) touch(id)
    }

    /** 閉じる。残りがあれば最後に開いた物を次の行き先にする。 */
    fun close(id: Int) {
        windows.removeAll { it.id == id }
        recency.remove(id)
        if (activeId == id) activeId = windows.lastOrNull()?.id
    }

    /** そのアプリのウィンドウ(詳細でない物)のうち最後に前に出た 1 枚。 */
    fun latestAppWindow(appId: String): WindowEntry? =
        recency.asReversed().firstNotNullOfOrNull { id -> windows.firstOrNull { it.id == id && it.key == WindowKey.app(appId) } }
            ?: find(WindowKey.app(appId))

    /** トレイの「すぐ記録」の行き先: 前にあるウィンドウのアプリの、アプリのウィンドウ(詳細には入れない)。無ければ開く。 */
    fun quickRecordWindow(): WindowEntry? {
        val appId = active?.key?.appId ?: return null
        return latestAppWindow(appId)?.also { activate(it.id) } ?: open(WindowKey.app(appId))
    }

    /**
     * 画面の中の「閉じる」・Esc(詳細のウィンドウの根から戻る)。アプリは終わらせない。
     * 最後の 1 枚ならそのアプリのウィンドウを先に開いてから閉じる(トレイに隠す・終わるのはタイトルバーの × だけ)。
     */
    fun closeInApp(id: Int) {
        val entry = windows.firstOrNull { it.id == id } ?: return
        if (windows.size == 1) open(WindowKey.app(entry.key.appId))
        if (windows.size > 1) close(id)
    }

    /**
     * ウィンドウの × ・詳細の「閉じる」。他にウィンドウがあればこれだけ閉じる。
     * 最後の 1 枚は [onLast](トレイに隠すか終わるか)に任せ、その返り値(終わるべきか)を返す。
     */
    fun requestClose(id: Int, onLast: () -> Boolean): Boolean {
        if (windows.size > 1) {
            close(id)
            return false
        }
        return onLast()
    }

    /** 同じ種類のウィンドウのうち何枚目か(新しいウィンドウを少しずらして重ねないため)。 */
    fun indexInKind(entry: WindowEntry): Int = windows.filter { it.key.kind == entry.key.kind }.indexOf(entry).coerceAtLeast(0)
}

/**
 * アプリ 1 つの記録。同じアプリのウィンドウは全部これを見るので、片方で保存すると他方の一覧にもすぐ出る。
 * 端末への書き込み([persistence])もアプリごとに 1 本(ウィンドウの数だけ二重に書かない)。
 * 端末から読む間は [loading](画面はスケルトン・[opening])。
 */
class SharedRecords(
    initial: MockStore = MockStore(),
    val persistence: SessionPersistence? = null,
    loading: Boolean = false,
    /** 画面のスレッドで動かす口(Main は Swing の EDT)。記録はここでだけ書き換える。 */
    private val post: (() -> Unit) -> Unit = { it() },
) {
    var store: MockStore by mutableStateOf(initial)
        private set

    /** 端末の記録を読んでいる間(#1887)。読み終えるまで画面は記録を書き換えない。 */
    var loading: Boolean by mutableStateOf(loading)
        private set

    /** 端末のデータを消した回数。増えたら各ウィンドウが下書き・遷移を戻す。 */
    var wipes: Int by mutableIntStateOf(0)
        private set

    private val ready = CompletableDeferred<Unit>().also { if (!loading) it.complete(Unit) }

    init {
        // 送れずに断られて端末の行が戻った時は、戻った行だけを画面のスレッドで混ぜる(端末にはもう書いてあるので persistence には渡さない)
        persistence?.onReload = { delta -> post { store = delta.applyTo(store) } }
    }

    /** 画面から書き換えた。前後の差を端末に渡す。読み終わる前は捨てる(読んだ記録で上書きされるため)。 */
    fun update(models: List<Entity>, next: MockStore) {
        if (loading) return
        val old = store
        store = next
        persistence?.changed(models, old, next)
    }

    /** 端末から読み終えた。 */
    fun loaded(initial: MockStore) {
        store = initial
        loading = false
        ready.complete(Unit)
    }

    /** 読み終わるまで待つ。 */
    suspend fun awaitLoaded() = ready.await()

    /** 端末のデータを消した後に空にする。 */
    fun reset() {
        store = MockStore()
        wipes++
    }

    /** 端末のデータを消す(#1887): 積んだ書き込みを流し終えてから端末の記録・送り待ち・設定を消し、画面を空にする。 */
    suspend fun wipe() {
        persistence?.wipe()
        reset()
    }

    fun close() {
        persistence?.close()
    }

    /** 積んだ書き込みが終わるのを待つ(終わる時・最後のウィンドウを閉じた時)。 */
    suspend fun idle() {
        persistence?.idle()
    }

    /** そのアプリのウィンドウが全部閉じた: 書き終えてから列を閉じ、裏の仕事を止める。 */
    suspend fun shutdown() {
        persistence?.shutdown()
    }

    companion object {
        /**
         * 起動時(#1887): 画面を止めずに端末の記録を [scope] で読む。読み終えるまで [loading]。
         * 読み終えたら前回送れなかった分を送り直す。読めなければ空で開く。
         */
        fun opening(
            persistence: SessionPersistence,
            scope: CoroutineScope,
            post: (() -> Unit) -> Unit = { it() },
            read: suspend () -> MockStore,
        ): SharedRecords = SharedRecords(MockStore(), persistence, loading = true, post).also { r ->
            scope.launch {
                val initial = runCatching { read() }.getOrElse {
                    System.err.println("[TailorIt] 端末の記録を読めませんでした: ${it::class.simpleName}")
                    MockStore()
                }
                r.post { r.loaded(initial) }
                persistence.resendPending()
            }
        }
    }
}

/** 一覧の行から詳細を別のウィンドウで開く時の行き先。 */
object RecordWindows {
    /** 行を押した時の遷移先(prefillRef の無い navigate だけ。新規を作る遷移は別のウィンドウにしない)。 */
    fun target(a: Action): String? = a.screen?.takeIf { a.type == ActionType.NAVIGATE && a.prefillRef == null }
}

/** 開いたアプリ(DSL と共有の記録)。同じ id はウィンドウが何枚あっても 1 つ。 */
class AppHub(
    private val load: (String) -> AppDSL = Examples::load,
    private val openRecords: (appId: String, dsl: AppDSL) -> SharedRecords = { _, _ -> SharedRecords() },
    /** 端末の保存先をまるごと消す(開いていないアプリの分も)。 */
    private val wipeStore: suspend () -> Unit = {},
) {
    class OpenedApp(val id: String, val dsl: AppDSL, val records: SharedRecords)

    private val apps = LinkedHashMap<String, OpenedApp>()
    private val dsls = HashMap<String, AppDSL>()

    /** アプリごとの開いているウィンドウの数。0 になったら記録の書き込みを閉じる。 */
    private val refs = HashMap<String, Int>()

    @Synchronized
    fun app(id: String): OpenedApp = apps.getOrPut(id) { dsl(id).let { OpenedApp(id, it, openRecords(id, it)) } }

    /** DSL だけ(トレイの名前・すぐ記録)。記録は開かない。 */
    @Synchronized
    fun dsl(id: String): AppDSL = apps[id]?.dsl ?: dsls.getOrPut(id) { load(id) }

    /** ウィンドウを 1 枚開いた。 */
    @Synchronized
    fun acquire(id: String): OpenedApp {
        refs[id] = (refs[id] ?: 0) + 1
        return app(id)
    }

    /**
     * ウィンドウを 1 枚閉じた。そのアプリの最後の 1 枚なら、書き終えるのを待って([timeoutMillis] まで)書き込みを閉じ、裏の仕事を止める。
     * 次に開く時は端末から読み直す。
     */
    fun release(id: String, timeoutMillis: Long = FLUSH_TIMEOUT_MILLIS) {
        val closing = synchronized(this) {
            val n = (refs[id] ?: 0) - 1
            if (n > 0) {
                refs[id] = n
                return
            }
            refs.remove(id)
            apps.remove(id)
        } ?: return
        runBlocking { withTimeoutOrNull(timeoutMillis) { closing.records.shutdown() } }
    }

    @Synchronized
    fun opened(): List<OpenedApp> = apps.values.toList()

    /**
     * 「この端末のデータを消す」(#1887・サインアウトの画面 #1749 までの口)。
     * 開いているアプリごとに書き込みを流し終えてから消して画面を空にし、最後に保存先をまるごと消す。
     */
    suspend fun wipeLocalData() {
        opened().forEach { it.records.wipe() }
        wipeStore()
    }

    /** 終わる時: 全部のアプリの積んだ書き込みを書き終えるのを待ち([timeoutMillis] まで)、それから閉じる。 */
    fun close(timeoutMillis: Long = FLUSH_TIMEOUT_MILLIS) {
        val all = synchronized(this) { apps.values.toList().also { apps.clear(); refs.clear() } }
        runBlocking { withTimeoutOrNull(timeoutMillis) { all.forEach { it.records.idle() } } }
        all.forEach { it.records.close() }
    }

    companion object {
        /** 終わる・閉じる時に書き終えるのを待つ長さ。 */
        const val FLUSH_TIMEOUT_MILLIS = 3_000L
    }
}
