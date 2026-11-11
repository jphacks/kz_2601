package jp.kz2601.metamon.desktop

import androidx.compose.material3.MaterialTheme
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
import java.io.File
import java.util.prefs.Preferences

/**
 * 起動。引数 1 つ目(または -Dtailorit.example)で同梱の例を選ぶ。既定は shopping。
 * 引数 2 つ目に記録の JSON(`{"<entity>": [{"id", "data"}]}`・case.json の形も可)を渡すとモックに入れて開く。3 つ目は最初の画面の id。
 * ウィンドウの大きさ・位置は覚えて次の起動で戻す。サインインはまだ無い(モック)。
 */
@OptIn(FlowPreview::class)
fun main(args: Array<String>) = application {
    val first = args.firstOrNull() ?: System.getProperty("tailorit.example") ?: "shopping"
    // コマンドパレット(#1765)で同梱の別の例に切り替える。記録・最初の画面は起動時の例にだけ使う
    var name by remember { mutableStateOf(first) }
    val dsl = remember(name) { Examples.load(name) }
    val store = remember(name) { args.getOrNull(1)?.takeIf { name == first }?.let { MockStore.fromJson(File(it).readText()) } ?: MockStore() }
    val saved = remember { WindowBounds.load() }
    val state = rememberWindowState(
        size = saved?.size ?: DpSize(480.dp, 820.dp),
        position = saved?.position ?: WindowPosition.PlatformDefault,
    )
    Window(onCloseRequest = ::exitApplication, title = "Tailor It — ${dsl.app.name}", state = state) {
        LaunchedEffect(Unit) {
            // 6 列が潰れない最小(1 列 ≈ 50dp)
            window.minimumSize = Dimension(360, 560)
            snapshotFlow { state.size to state.position }.debounce(500).collect { (size, pos) -> WindowBounds.save(size, pos) }
        }
        MaterialTheme { key(name) { DesktopApp(dsl, store, args.getOrNull(2)?.takeIf { name == first }, onOpenApp = { name = it }) } }
    }
}

/** ウィンドウの大きさ・位置(java.util.prefs: Windows はレジストリ、Linux は ~/.java)。 */
private object WindowBounds {
    private val prefs = Preferences.userRoot().node("jp/kz2601/tailorit/desktop")

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
