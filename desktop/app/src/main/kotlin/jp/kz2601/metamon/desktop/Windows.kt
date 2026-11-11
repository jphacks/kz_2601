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

    fun find(key: WindowKey): WindowEntry? = windows.firstOrNull { it.key == key }

    /** 開く。同じアプリ・同じ記録のウィンドウがあれば作らずにそれを前に出す。 */
    fun open(key: WindowKey): WindowEntry {
        find(key)?.let { e ->
            e.focusRequests++
            activeId = e.id
            return e
        }
        return WindowEntry(nextId++, key).also {
            windows += it
            activeId = it.id
        }
    }

    fun activate(id: Int) {
        if (windows.any { it.id == id }) activeId = id
    }

    /** 閉じる。残りがあれば最後に開いた物を次の行き先にする。 */
    fun close(id: Int) {
        windows.removeAll { it.id == id }
        if (activeId == id) activeId = windows.lastOrNull()?.id
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
class SharedRecords(initial: MockStore = MockStore(), val persistence: SessionPersistence? = null, loading: Boolean = false) {
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
        // 送れずに断られて端末の行が戻った時は読み直す(端末にはもう書いてあるので persistence には渡さない)
        persistence?.onReload = { store = it }
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

    companion object {
        /**
         * 起動時(#1887): 画面を止めずに端末の記録を [scope] で読む。読み終えるまで [loading]。
         * 読み終えたら前回送れなかった分を送り直す。読めなければ空で開く。
         */
        fun opening(
            persistence: SessionPersistence,
            scope: CoroutineScope,
            read: suspend () -> MockStore,
        ): SharedRecords = SharedRecords(MockStore(), persistence, loading = true).also { r ->
            scope.launch {
                val initial = runCatching { read() }.getOrElse {
                    System.err.println("[TailorIt] 端末の記録を読めませんでした: ${it::class.simpleName}")
                    MockStore()
                }
                r.loaded(initial)
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

    @Synchronized
    fun app(id: String): OpenedApp = apps.getOrPut(id) { load(id).let { OpenedApp(id, it, openRecords(id, it)) } }

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

    /** 終わる時: 積んだ書き込みを書き終えてから閉じる。 */
    @Synchronized
    fun close() {
        apps.values.forEach { it.records.close() }
        apps.clear()
    }
}
