package jp.kz2601.metamon.desktop

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
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
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import kotlinx.coroutines.delay
import java.util.prefs.Preferences
import androidx.compose.ui.window.Notification as TrayNotification

// トレイ / メニューバーに常駐する(#1761): 開く・すぐ記録・音声(使えない旨)・走っているタイマー・閉じてもトレイに残す・終了。

/** トレイのメニューの 1 行(画面に頼らない形。テストはこれを比べる)。 */
sealed interface TrayItem {
    data class Command(val command: TrayCommand, val label: String, val enabled: Boolean = true) : TrayItem
    data class Toggle(val command: TrayCommand, val label: String, val checked: Boolean) : TrayItem
    data class Info(val label: String) : TrayItem
    data object Separator : TrayItem

    /** 入れ子のメニュー(新しいウィンドウ ▸ アプリ)。 */
    data class Submenu(val label: String, val items: List<TrayItem>) : TrayItem

    /** そのアプリを別のウィンドウで開く(開いていれば前に出す・#1763)。 */
    data class OpenApp(val appId: String, val label: String) : TrayItem
}

enum class TrayCommand { OPEN, QUICK_RECORD, VOICE, CLOSE_TO_TRAY, QUIT }

/** 走っている(または止めてある)timerDisplay 1 つ。seconds は countdown なら残り、他は経過。 */
data class TimerStatus(val key: String, val name: String, val seconds: Int, val countdown: Boolean, val running: Boolean)

object TrayMenu {
    /** [apps] は「新しいウィンドウ」に並べる同梱のアプリ(id, 名前)。空なら出さない。 */
    fun items(
        appName: String,
        canQuickRecord: Boolean,
        timers: List<TimerStatus>,
        closeToTray: Boolean,
        apps: List<Pair<String, String>> = emptyList(),
    ): List<TrayItem> = buildList {
        add(TrayItem.Command(TrayCommand.OPEN, Strings.format("desktop_tray_open", appName)))
        add(TrayItem.Command(TrayCommand.QUICK_RECORD, Strings["desktop_tray_quick_record"], enabled = canQuickRecord))
        // 音声はデスクトップ版ではまだ使えない(openVoice と同じ)。押せない行で場所だけ取る
        add(TrayItem.Command(TrayCommand.VOICE, Strings["desktop_tray_voice"], enabled = false))
        if (apps.isNotEmpty()) add(TrayItem.Submenu(Strings["desktop_tray_new_window"], apps.map { (id, name) -> TrayItem.OpenApp(id, name) }))
        if (timers.isNotEmpty()) {
            add(TrayItem.Separator)
            timers.forEach { add(TrayItem.Info(timerLabel(it))) }
        }
        add(TrayItem.Separator)
        add(TrayItem.Toggle(TrayCommand.CLOSE_TO_TRAY, Strings["desktop_tray_close_to_tray"], closeToTray))
        add(TrayItem.Command(TrayCommand.QUIT, Strings["desktop_tray_quit"]))
    }

    /** ツールチップ: 「Tailor It — <アプリ名>」、タイマーが走っていれば後ろに並べる。 */
    fun tooltip(appName: String, timers: List<TimerStatus>): String {
        val title = Strings.format("desktop_window_title", appName)
        return if (timers.isEmpty()) title else title + "\n" + timers.joinToString("\n") { timerLabel(it) }
    }

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
) {
    /** 今前にあるウィンドウのアプリ(名前・すぐ記録の行き先)。 */
    var dsl: AppDSL by mutableStateOf(dsl)
        private set
    var quickRecordAction: Action? by mutableStateOf(QuickRecordTarget.find(dsl))
        private set

    /** 「新しいウィンドウ」でアプリを選んだ(Main がウィンドウを開く)。 */
    var onOpenApp: (String) -> Unit = {}

    /** 前に出たウィンドウのアプリに替える(複数のウィンドウ・#1763)。 */
    fun switchApp(next: AppDSL) {
        if (next === dsl) return
        dsl = next
        quickRecordAction = QuickRecordTarget.find(next)
    }

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

    fun items(timers: List<TimerStatus>): List<TrayItem> = TrayMenu.items(dsl.app.name, quickRecordAction != null, timers, closeToTray, apps)

    companion object {
        fun create(dsl: AppDSL): TrayController =
            TrayController(dsl, runCatching { isTraySupported }.getOrDefault(false), apps = BundledApps.all())
    }
}

val LocalTray = compositionLocalOf<TrayController?> { null }

/** トレイのアイコンとメニュー。トレイが無い OS では何も出さない。通知([Notifier.Tray])もこのアイコンから出す。 */
@Composable
fun ApplicationScope.DesktopTray(c: TrayController) {
    if (!c.traySupported) return
    val state = rememberTrayState()
    DisposableEffect(state) {
        Notifier.Tray.sink = { title, body -> state.sendNotification(TrayNotification(title, body, TrayNotification.Type.Info)) }
        onDispose { Notifier.Tray.sink = null }
    }
    var timers by remember { mutableStateOf(DesktopTimers.active()) }
    LaunchedEffect(Unit) {
        while (true) {
            timers = DesktopTimers.active()
            // 走っているタイマーがある間だけ毎秒、無ければ間を空ける
            delay(if (timers.any { it.running }) 1_000 else 3_000)
        }
    }
    Tray(icon = TrayIconPainter, state = state, tooltip = TrayMenu.tooltip(c.dsl.app.name, timers), onAction = c::open) {
        TrayItems(c, c.items(timers), ::exitApplication)
    }
}

@Composable
private fun MenuScope.TrayItems(c: TrayController, items: List<TrayItem>, quit: () -> Unit) {
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
            is TrayItem.Toggle -> CheckboxItem(item.label, item.checked) { c.updateCloseToTray(it) }
            is TrayItem.Info -> Item(item.label, enabled = false) {}
            TrayItem.Separator -> Separator()
            is TrayItem.Submenu -> Menu(item.label) { TrayItems(c, item.items, quit) }
            is TrayItem.OpenApp -> Item(item.label) { c.openApp(item.appId) }
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
