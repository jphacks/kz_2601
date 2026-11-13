package jp.kz2601.metamon.desktop

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.window.ApplicationScope
import androidx.compose.ui.window.MenuScope
import androidx.compose.ui.window.Tray
import androidx.compose.ui.window.isTraySupported
import androidx.compose.ui.window.rememberTrayState
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.notify.TrayNotifier
import jp.kz2601.metamon.desktop.settings.UpcomingNotifications
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.time.LocalDate
import java.util.prefs.Preferences
import androidx.compose.ui.window.Notification as TrayNotification

// トレイ / メニューバーに常駐する(#1761): 前にあるアプリの集計・開く・すぐ記録・音声(使えない旨)・走っているタイマー・閉じてもトレイに残す・終了。

/** トレイのメニューの 1 行(画面に頼らない形。テストはこれを比べる)。 */
sealed interface TrayItem {
    data class Command(val command: TrayCommand, val label: String, val enabled: Boolean = true) : TrayItem
    /**
     * [revision] が変わったら行を作り直す(押した側の見た目が OS 側の状態とずれた時に戻すため)。
     * [enabled] が false の間は押せない(切り替え中)。
     */
    data class Toggle(val command: TrayCommand, val label: String, val checked: Boolean, val enabled: Boolean = true, val revision: Int = 0) : TrayItem
    data class Info(val label: String) : TrayItem
    data object Separator : TrayItem

    /** 入れ子のメニュー(新しいウィンドウ ▸ アプリ)。 */
    data class Submenu(val label: String, val items: List<TrayItem>) : TrayItem

    /** そのアプリを別のウィンドウで開く(開いていれば前に出す・#1763)。 */
    data class OpenApp(val appId: String, val label: String) : TrayItem

    /** 予約した通知 1 件。選ぶと取り消す。 */
    data class CancelNotification(val id: String, val label: String) : TrayItem
}

enum class TrayCommand { OPEN, QUICK_RECORD, VOICE, CLOSE_TO_TRAY, OPEN_AT_LOGIN, QUIT }

/** 走っている(または止めてある)timerDisplay 1 つ。seconds は countdown なら残り、他は経過。 */
data class TimerStatus(val key: String, val name: String, val seconds: Int, val countdown: Boolean, val running: Boolean)

/** トレイに出す stat 1 個(値は書式後。記録を読み終える前は「—」)。 */
data class TrayStat(val label: String, val value: String) {
    val text: String get() = if (label.isEmpty()) value else "$label $value"
}

/** 前にあるアプリの先頭の `stat`(Mac のメニューバー `MacMenuBarStats`・iOS のウィジェットと同じ並び)。 */
object TrayStats {
    const val MAX = 3
    const val LOADING = "—"

    /** 集計の基準日(アプリ内の stat と同じ時計・時間帯)。 */
    fun today(): LocalDate = LocalDate.ofInstant(DSLClock.now(), FieldCoding.zone)

    /** 画面順(initialScreen → タブ順 → 残り)で `stat` を並べる。 */
    fun components(dsl: AppDSL): List<Component> {
        val order = (listOf(dsl.initialScreen) + dsl.app.tabs.map { it.screen } + dsl.screens.map { it.id }).distinct()
        return order.mapNotNull { dsl.screen(it) }.flatMap { it.allComponents() }.filter { it.type == ComponentType.STAT }
    }

    /** 先頭 [MAX] 個。値はアプリ内の stat と同じ [StatValue.resolve]。[store] が null(読み込み中)なら「—」。 */
    fun make(dsl: AppDSL, store: MockStore?): List<TrayStat> {
        val stats = components(dsl).take(MAX)
        if (stats.isEmpty()) return emptyList()
        val data = store?.let { LeafData.of(dsl, it) }
        return stats.map { c -> TrayStat(c.label ?: c.title.orEmpty(), data?.let { StatValue.resolve(c, it).first } ?: LOADING) }
    }
}

object TrayMenu {
    /**
     * [apps] は「新しいウィンドウ」に並べる同梱のアプリ(id, 名前)。空なら出さない。
     * [openAtLogin] は「ログイン時に開く」の今の状態。null(この OS・この動かし方では使えない)なら出さない。
     * [openAtLoginBusy] の間(読み込み・切り替え中)は押せない。[openAtLoginRevision] は [TrayItem.Toggle.revision] に入れる。
     * [stats] は前にあるアプリの集計。先頭に押せない行で出す(Mac のメニューバーと同じく上に)。
     */
    fun items(
        appName: String,
        canQuickRecord: Boolean,
        timers: List<TimerStatus>,
        closeToTray: Boolean,
        apps: List<Pair<String, String>> = emptyList(),
        openAtLogin: Boolean? = null,
        stats: List<TrayStat> = emptyList(),
        openAtLoginBusy: Boolean = false,
        openAtLoginRevision: Int = 0,
        upcoming: UpcomingNotifications = UpcomingNotifications.EMPTY,
    ): List<TrayItem> = buildList {
        if (stats.isNotEmpty()) {
            stats.forEach { add(TrayItem.Info(it.text)) }
            add(TrayItem.Separator)
        }
        add(TrayItem.Command(TrayCommand.OPEN, Strings.format("desktop_tray_open", appName)))
        add(TrayItem.Command(TrayCommand.QUICK_RECORD, Strings["desktop_tray_quick_record"], enabled = canQuickRecord))
        // 音声はデスクトップ版ではまだ使えない(openVoice と同じ)。押せない行で場所だけ取る
        add(TrayItem.Command(TrayCommand.VOICE, Strings["desktop_tray_voice"], enabled = false))
        if (apps.isNotEmpty()) add(TrayItem.Submenu(Strings["desktop_tray_new_window"], apps.map { (id, name) -> TrayItem.OpenApp(id, name) }))
        if (timers.isNotEmpty()) {
            add(TrayItem.Separator)
            timers.forEach { add(TrayItem.Info(timerLabel(it))) }
        }
        if (!upcoming.isEmpty) {
            add(TrayItem.Separator)
            add(
                TrayItem.Submenu(
                    Strings["desktop_tray_upcoming"],
                    buildList {
                        add(TrayItem.Info(Strings["desktop_tray_upcoming_hint"]))
                        upcoming.rows.forEach { add(TrayItem.CancelNotification(it.id, UpcomingNotifications.label(it))) }
                        if (upcoming.hidden > 0) add(TrayItem.Info(Strings.format("desktop_tray_upcoming_more", upcoming.hidden)))
                    },
                ),
            )
        }
        add(TrayItem.Separator)
        add(TrayItem.Toggle(TrayCommand.CLOSE_TO_TRAY, Strings["desktop_tray_close_to_tray"], closeToTray))
        if (openAtLogin != null) {
            add(TrayItem.Toggle(TrayCommand.OPEN_AT_LOGIN, Strings["desktop_tray_open_at_login"], openAtLogin, !openAtLoginBusy, openAtLoginRevision))
        }
        add(TrayItem.Command(TrayCommand.QUIT, Strings["desktop_tray_quit"]))
    }

    /** ツールチップ: 「Tailor It — <アプリ名>」、走っているタイマーを後ろに並べる。集計はメニューにだけ出す(記録の値を出し過ぎない)。 */
    fun tooltip(appName: String, timers: List<TimerStatus>): String =
        (listOf(Strings.format("desktop_window_title", appName)) + timers.map { timerLabel(it) }).joinToString("\n")

    fun timerLabel(t: TimerStatus): String {
        val time = TimerRun.format(t.seconds)
        val base = Strings.format(if (t.countdown) "desktop_tray_timer_left" else "desktop_tray_timer_elapsed", t.name, time)
        return if (t.running) base else Strings.format("desktop_tray_timer_paused", base)
    }
}

/** ウィンドウの × を押した時。 */
enum class CloseDecision { HIDE_TO_TRAY, EXIT }

object CloseToTray {
    /** トレイが無い環境(GNOME で拡張が無い等)で隠すと戻れないので、設定が on でも終わる。 */
    fun decide(enabled: Boolean, traySupported: Boolean): CloseDecision =
        if (enabled && traySupported) CloseDecision.HIDE_TO_TRAY else CloseDecision.EXIT
}

/** on / off の設定 1 つ。 */
interface BoolSetting {
    fun get(): Boolean
    fun set(value: Boolean)
}

class MemoryBoolSetting(private var value: Boolean) : BoolSetting {
    override fun get() = value
    override fun set(value: Boolean) {
        this.value = value
    }
}

/** java.util.prefs(Windows はレジストリ、Linux は ~/.java)。ウィンドウの大きさと同じ場所。 */
class PrefsBoolSetting(private val key: String, private val default: Boolean) : BoolSetting {
    private val prefs = Preferences.userRoot().node("jp/kz2601/tailorit/desktop")
    override fun get() = prefs.getBoolean(key, default)
    override fun set(value: Boolean) = prefs.putBoolean(key, value)
}

/** 「すぐ記録」で開くフォーム画面。 */
object QuickRecordTarget {
    /**
     * 記録できる entity(single でなく項目がある)のフォーム画面のうち、DSL の中で prefillRef なしに開いている物を先に選び、その navigate を使う
     * (sheet などの開き方を DSL と揃える)。どこからも開いていなければ最初のフォーム画面を push。無ければ null。
     */
    fun find(dsl: AppDSL): Action? {
        val entities = dsl.entityModels().associateBy { it.id }
        val forms = dsl.screens.filter { s -> s.entity?.let(entities::get)?.let { !it.single && it.fields.isNotEmpty() } == true }.map { it.id }
        if (forms.isEmpty()) return null
        val opens = dsl.screens.flatMap { s -> s.toolbar.map { it.action } + s.allComponents().mapNotNull { it.action } }
            .filter { it.type == ActionType.NAVIGATE && it.prefillRef == null && it.screen in forms }
        return forms.firstNotNullOfOrNull { id -> opens.firstOrNull { it.screen == id } }
            ?: Action(type = ActionType.NAVIGATE, screen = forms.first())
    }
}

/**
 * トレイとウィンドウの間の状態。Main が 1 つ作り、[LocalTray] で DesktopApp(すぐ記録の遷移)に渡す。
 * @param traySupported OS にトレイがあるか(無ければトレイを出さず、× で終わる)
 */
class TrayController(
    dsl: AppDSL,
    val traySupported: Boolean,
    private val closeSetting: BoolSetting = PrefsBoolSetting("closeToTray", true),
    private val hintSetting: BoolSetting = PrefsBoolSetting("trayHintShown", false),
    /** 「新しいウィンドウ」に並べるアプリ(id, 名前)。 */
    val apps: List<Pair<String, String>> = emptyList(),
    /** ログイン時に開く口。null(使えない)ならトレイに出さない。 */
    private val loginItem: LoginItem? = null,
    /** [loginItem] を動かす所(レジストリ・ファイルを触るので UI のスレッドでは動かさない)。 */
    private val loginItemDispatcher: CoroutineDispatcher = Dispatchers.IO,
) {
    /** 「ログイン時に開く」の状態。OS 側から読むまでは off。null は使えない。 */
    var openAtLogin: Boolean? by mutableStateOf(if (loginItem != null) false else null)
        private set

    /** 読み込み・切り替えの最中(行を押せなくする)。最初の読み込みが済むまでも true。 */
    var openAtLoginBusy by mutableStateOf(loginItem != null)
        private set

    /** 切り替えが済むたびに増やす(トレイの行を作り直して OS 側の状態に合わせる)。 */
    var openAtLoginRevision by mutableIntStateOf(0)
        private set

    /** 起動時に 1 回: OS 側の状態を読む。登録が前の道を指していたら今の道で書き直す。 */
    suspend fun loadOpenAtLogin() {
        val item = loginItem ?: return
        openAtLoginBusy = true
        try {
            openAtLogin = withContext(loginItemDispatcher) {
                runCatching { if (item.isRegistered() && !item.isEnabled()) item.setEnabled(true) }
                    .onFailure { System.err.println("[TailorIt] open at login: refresh failed $it") }
                readOpenAtLogin(item)
            }
        } finally {
            openAtLoginBusy = false
        }
    }

    private fun readOpenAtLogin(item: LoginItem): Boolean = runCatching { item.isEnabled() }.getOrDefault(false)

    /** 切り替える。失敗したら知らせ、どちらでも OS 側の今の状態に合わせる(行も作り直す)。 */
    suspend fun updateOpenAtLogin(on: Boolean, notify: (String, String) -> Unit = Notifier.Tray::notify) {
        val item = loginItem ?: return
        if (openAtLoginBusy) return
        openAtLoginBusy = true
        try {
            val (failure, now) = withContext(loginItemDispatcher) {
                runCatching { item.setEnabled(on) }.exceptionOrNull() to readOpenAtLogin(item)
            }
            if (failure != null) {
                System.err.println("[TailorIt] open at login: ${if (on) "enable" else "disable"} failed $failure")
                notify(Strings.format("desktop_window_title", dsl.app.name), Strings["desktop_tray_open_at_login_failed"])
            }
            openAtLogin = now
        } finally {
            openAtLoginBusy = false
            openAtLoginRevision++
        }
    }

    /** 今前にあるウィンドウのアプリ(名前・すぐ記録の行き先)。 */
    var dsl: AppDSL by mutableStateOf(dsl)
        private set
    var quickRecordAction: Action? by mutableStateOf(QuickRecordTarget.find(dsl))
        private set

    /** 「新しいウィンドウ」でアプリを選んだ(Main がウィンドウを開く)。 */
    var onOpenApp: (String) -> Unit = {}

    /** 前にあるアプリの記録(集計を出す)。null は未設定(集計は「—」)。 */
    var records: SharedRecords? by mutableStateOf(null)
        private set

    /** 前に出たウィンドウのアプリ・その記録に替える(複数のウィンドウ・#1763)。 */
    fun switchApp(next: AppDSL, records: SharedRecords? = null) {
        this.records = records
        if (next === dsl) return
        dsl = next
        quickRecordAction = QuickRecordTarget.find(next)
    }

    /** 前にあるアプリの集計。記録を読み終える前は「—」。記録は状態なので、保存・削除で読み直される。 */
    fun stats(): List<TrayStat> = TrayStats.make(dsl, records?.takeUnless { it.loading }?.store)

    /** アプリを別のウィンドウで開く。隠れていたウィンドウも出す。 */
    fun openApp(appId: String) {
        windowVisible = true
        onOpenApp(appId)
    }

    var windowVisible by mutableStateOf(true)
        private set
    var closeToTray by mutableStateOf(closeSetting.get())
        private set

    /** 増えたら DesktopApp が quickRecordAction を 1 回動かす。 */
    var quickRecordRequests by mutableIntStateOf(0)
        private set

    /** 増えたらウィンドウを前に出す。 */
    var focusRequests by mutableIntStateOf(0)
        private set

    /** 「すぐ記録」を受け取るウィンドウを決める(Main: 前にあるアプリの、アプリのウィンドウ。無ければ開く)。null を返せば前にある物。 */
    var findQuickRecordWindow: () -> Int? = { null }

    /** まだ受け取られていない「すぐ記録」の行き先(null は前にあるウィンドウ)。 */
    var quickRecordWindow: Int? by mutableStateOf(null)
        private set
    private var quickRecordPending = false

    fun open() {
        windowVisible = true
        focusRequests++
    }

    fun quickRecord() {
        if (quickRecordAction == null) return
        quickRecordWindow = findQuickRecordWindow()
        quickRecordPending = true
        open()
        quickRecordRequests++
    }

    /**
     * ウィンドウが「すぐ記録」を受け取る。行き先なら 1 回だけ true。
     * 行き先を開いたばかりで、記録を読み終えて画面ができた後に受け取っても動く。
     */
    fun takeQuickRecord(windowId: Int?, isActive: () -> Boolean): Boolean {
        if (!quickRecordPending) return false
        val target = quickRecordWindow
        if (if (target != null) windowId != target else !isActive()) return false
        quickRecordPending = false
        quickRecordWindow = null
        return true
    }

    fun updateCloseToTray(on: Boolean) {
        closeSetting.set(on)
        closeToTray = on
    }

    /** × を押した。隠したら初回だけ「トレイに残っています」を知らせる。戻り値は終わるべきか。 */
    fun onCloseRequest(notify: (String, String) -> Unit = Notifier.Tray::notify): Boolean =
        when (CloseToTray.decide(closeToTray, traySupported)) {
            CloseDecision.EXIT -> true
            CloseDecision.HIDE_TO_TRAY -> {
                windowVisible = false
                if (!hintSetting.get()) {
                    hintSetting.set(true)
                    notify(Strings.format("desktop_window_title", dsl.app.name), Strings["desktop_tray_hidden_hint"])
                }
                false
            }
        }

    fun items(timers: List<TimerStatus>, stats: List<TrayStat> = stats(), upcoming: UpcomingNotifications = UpcomingNotifications.EMPTY): List<TrayItem> =
        TrayMenu.items(dsl.app.name, quickRecordAction != null, timers, closeToTray, apps, openAtLogin, stats, openAtLoginBusy, openAtLoginRevision, upcoming)

    companion object {
        fun create(dsl: AppDSL): TrayController {
            val traySupported = runCatching { isTraySupported }.getOrDefault(false)
            // トレイが無いとメニューから切り替えられないので出さない
            val loginItem = if (traySupported) Autostart.forCurrentOs() else null
            return TrayController(dsl, traySupported, apps = BundledApps.all(), loginItem = loginItem)
        }
    }
}

val LocalTray = compositionLocalOf<TrayController?> { null }

/** トレイのアイコンとメニュー。トレイが無い OS では何も出さない。通知([TrayNotifier])もこのアイコンから出す。 */
@Composable
fun ApplicationScope.DesktopTray(c: TrayController) {
    if (!c.traySupported) return
    val state = rememberTrayState()
    DisposableEffect(state) {
        TrayNotifier.sink = { title, body -> state.sendNotification(TrayNotification(title, body, TrayNotification.Type.Info)) }
        onDispose { TrayNotifier.sink = null }
    }
    var timers by remember { mutableStateOf(DesktopTimers.active()) }
    var upcoming by remember { mutableStateOf(UpcomingNotifications.of(Notifier.Tray.upcoming())) }
    // 「今月」などの集計の基準日。日付が替わったら集計を計算し直す
    var today by remember { mutableStateOf(TrayStats.today()) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(c) { c.loadOpenAtLogin() }
    LaunchedEffect(Unit) {
        while (true) {
            timers = DesktopTimers.active()
            upcoming = UpcomingNotifications.of(Notifier.Tray.upcoming())
            today = TrayStats.today()
            // 走っているタイマーがある間だけ毎秒、無ければ間を空ける
            delay(if (timers.any { it.running }) 1_000 else 3_000)
        }
    }
    // 集計はアプリ・記録・日付が替わった時だけ計算し直す(タイマーの毎秒の描き直しでは計算しない)
    val records = c.records
    val store = records?.store
    val loading = records?.loading
    val stats = remember(c.dsl, store, loading, today) { c.stats() }
    Tray(icon = TrayIconPainter, state = state, tooltip = TrayMenu.tooltip(c.dsl.app.name, timers), onAction = c::open) {
        TrayItems(c, c.items(timers, stats, upcoming), scope, ::exitApplication)
    }
}

@Composable
private fun MenuScope.TrayItems(c: TrayController, items: List<TrayItem>, scope: CoroutineScope, quit: () -> Unit) {
    items.forEach { item ->
        when (item) {
            is TrayItem.Command -> Item(item.label, enabled = item.enabled) {
                when (item.command) {
                    TrayCommand.OPEN -> c.open()
                    TrayCommand.QUICK_RECORD -> c.quickRecord()
                    TrayCommand.QUIT -> quit()
                    else -> Unit
                }
            }
            // AWT のチェックは押した時点で自分で反転する。revision で作り直して状態どおりに戻す
            is TrayItem.Toggle -> key(item.command, item.revision) {
                CheckboxItem(item.label, item.checked, enabled = item.enabled) {
                    when (item.command) {
                        TrayCommand.OPEN_AT_LOGIN -> scope.launch { c.updateOpenAtLogin(it) }
                        else -> c.updateCloseToTray(it)
                    }
                }
            }
            is TrayItem.Info -> Item(item.label, enabled = false) {}
            TrayItem.Separator -> Separator()
            is TrayItem.Submenu -> Menu(item.label) { TrayItems(c, item.items, scope, quit) }
            is TrayItem.OpenApp -> Item(item.label) { c.openApp(item.appId) }
            is TrayItem.CancelNotification -> Item(item.label) { Notifier.Tray.cancel(item.id) }
        }
    }
}

/**
 * DesktopApp の中で呼ぶ: トレイの「すぐ記録」をこのセッションの遷移にする。
 * 行き先はトレイが決めたウィンドウ([windowId]・詳細には入れない)。決めていなければ前にある 1 枚([isActive])。
 */
@Composable
fun TrayHook(session: DesktopSession, windowId: Int? = null, isActive: () -> Boolean = { true }) {
    val c = LocalTray.current ?: return
    LaunchedEffect(c.quickRecordRequests) {
        if (!c.takeQuickRecord(windowId, isActive)) return@LaunchedEffect
        val a = c.quickRecordAction ?: return@LaunchedEffect
        if (a.screen?.let { session.dsl.screen(it) } == null) return@LaunchedEffect
        // もう開いていれば重ねない
        if (session.top.screen != a.screen) session.dispatch(a, ActionContext(session.top, null))
    }
}

/** トレイのアイコン(色の丸)。画像ファイルは持たない。 */
private object TrayIconPainter : Painter() {
    override val intrinsicSize = Size(32f, 32f)
    override fun DrawScope.onDraw() {
        drawCircle(Color(0xFF3F51B5))
        drawCircle(Color.White, radius = size.minDimension * 0.22f)
    }
}
