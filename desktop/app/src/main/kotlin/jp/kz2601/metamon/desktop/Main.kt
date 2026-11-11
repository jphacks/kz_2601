package jp.kz2601.metamon.desktop

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Window
import androidx.compose.ui.window.WindowPosition
import androidx.compose.ui.window.application
import androidx.compose.ui.window.rememberWindowState
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.debounce
import java.awt.Dimension
import java.awt.GraphicsEnvironment
import java.awt.Rectangle
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.theme.TailorItTheme
import java.io.File
import java.util.prefs.Preferences

/**
 * 起動。引数 1 つ目(または -Dtailorit.example)で同梱の例を選ぶ。既定は shopping。
 * 引数 2 つ目に記録の JSON(`{"<entity>": [{"id", "data"}]}`・case.json の形も可)を渡すとモックに入れて開く。3 つ目は最初の画面の id。
 * ウィンドウの大きさ・位置は覚えて次の起動で戻す。トレイ / メニューバーに常駐する(Tray.kt)。サインインはまだ無い(モック)。
 */
@OptIn(FlowPreview::class)
fun main(args: Array<String>) = application {
    val first = args.firstOrNull() ?: System.getProperty("tailorit.example") ?: "shopping"
    // コマンドパレット(#1765)で同梱の別の例に切り替える。記録・最初の画面は起動時の例にだけ使う
    var name by remember { mutableStateOf(first) }
    val dsl = remember(name) { Examples.load(name) }
    val store = remember(name) { args.getOrNull(1)?.takeIf { name == first }?.let { MockStore.fromJson(File(it).readText()) } ?: MockStore() }
    val saved = remember { WindowBounds.load()?.let { WindowBounds.fit(it, WindowBounds.screens()) } }
    val state = rememberWindowState(
        size = saved?.size ?: DpSize(480.dp, 820.dp),
        position = saved?.position ?: WindowPosition.PlatformDefault,
    )
    // トレイに常駐(#1761): × はトレイに隠す(設定で off・トレイが無ければ終わる)。終わるのはトレイの「終了」
    // パレットでアプリを替えたらトレイ(すぐ記録・名前)も替える
    val tray = remember(dsl) { TrayController.create(dsl) }
    DesktopTray(tray)
    Window(
        onCloseRequest = { if (tray.onCloseRequest()) exitApplication() },
        visible = tray.windowVisible,
        title = Strings.format("desktop_window_title", dsl.app.name),
        state = state,
    ) {
        LaunchedEffect(tray.focusRequests) {
            if (tray.focusRequests > 0) {
                state.isMinimized = false
                window.toFront()
            }
        }
        LaunchedEffect(Unit) {
            // 6 列が潰れない最小(1 列 ≈ 50dp)
            window.minimumSize = Dimension(360, 560)
            snapshotFlow { state.size to state.position }.debounce(500).collect { (size, pos) -> WindowBounds.save(size, pos) }
        }
        // 明暗は OS に従う・文言は OS の言語(日本語か英語)・#1759
        CompositionLocalProvider(LocalTray provides tray) {
            TailorItTheme { key(name) { DesktopApp(dsl, store, args.getOrNull(2)?.takeIf { name == first }, onOpenApp = { name = it }) } }
        }
    }
}

/** ウィンドウの大きさ・位置(java.util.prefs: Windows はレジストリ、Linux は ~/.java)。 */
internal object WindowBounds {
    private val prefs by lazy { Preferences.userRoot().node("jp/kz2601/tailorit/desktop") }

    data class Bounds(val size: DpSize, val position: WindowPosition)

    fun load(): Bounds? {
        val w = prefs.getFloat("w", -1f)
        val h = prefs.getFloat("h", -1f)
        if (w < 360 || h < 560) return null
        val x = prefs.getFloat("x", Float.NaN)
        val y = prefs.getFloat("y", Float.NaN)
        val pos = if (x.isNaN() || y.isNaN()) WindowPosition.PlatformDefault else WindowPosition(x.dp, y.dp)
        return Bounds(DpSize(w.dp, h.dp), pos)
    }

    /** 今つながっている画面(外した画面に開かないため)。分からなければ空。 */
    fun screens(): List<Rectangle> = runCatching {
        if (GraphicsEnvironment.isHeadless()) emptyList() else GraphicsEnvironment.getLocalGraphicsEnvironment().screenDevices.map { it.defaultConfiguration.bounds }
    }.getOrDefault(emptyList())

    /**
     * 覚えた位置・大きさを今の画面に収める。大きさは一番大きい画面まで。
     * 見出し(左上)がどの画面にも乗らない位置(外した画面・解像度を下げた)は OS の既定の位置に戻す。
     */
    fun fit(b: Bounds, screens: List<Rectangle>): Bounds {
        if (screens.isEmpty()) return b
        val maxW = screens.maxOf { it.width }.toFloat()
        val maxH = screens.maxOf { it.height }.toFloat()
        val size = DpSize(b.size.width.value.coerceAtMost(maxW).dp, b.size.height.value.coerceAtMost(maxH).dp)
        val pos = b.position as? WindowPosition.Absolute ?: return Bounds(size, b.position)
        val title = Rectangle(pos.x.value.toInt(), pos.y.value.toInt(), size.width.value.toInt().coerceIn(1, TITLE_W), TITLE_H)
        val onScreen = screens.firstOrNull { it.intersection(title).let { r -> !r.isEmpty && r.width >= TITLE_MIN_VISIBLE && r.height >= TITLE_H / 2 } }
            ?: return Bounds(size, WindowPosition.PlatformDefault)
        // 乗っている画面の中に収まるよう右下・左上を寄せる
        val x = pos.x.value.coerceIn(onScreen.x.toFloat(), (onScreen.x + onScreen.width - size.width.value).coerceAtLeast(onScreen.x.toFloat()))
        val y = pos.y.value.coerceIn(onScreen.y.toFloat(), (onScreen.y + onScreen.height - size.height.value).coerceAtLeast(onScreen.y.toFloat()))
        return Bounds(size, WindowPosition(x.dp, y.dp))
    }

    private const val TITLE_W = 200
    private const val TITLE_H = 32
    private const val TITLE_MIN_VISIBLE = 48

    fun save(size: DpSize, pos: WindowPosition) {
        if (!size.width.value.isFinite() || !size.height.value.isFinite()) return
        prefs.putFloat("w", size.width.value)
        prefs.putFloat("h", size.height.value)
        if (pos is WindowPosition.Absolute) {
            prefs.putFloat("x", pos.x.value)
            prefs.putFloat("y", pos.y.value)
        }
    }
}
