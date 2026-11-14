package jp.kz2601.metamon.desktop.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import java.util.prefs.Preferences
import kotlin.math.roundToInt

/**
 * Ctrl + / Ctrl - / Ctrl 0(macOS は ⌘)で変える画面全体の文字の倍率(#2208)。iOS の Dynamic Type・Mac の ⌘+ / ⌘- に当たる。
 * 0.8〜2.0 を 0.1 刻み。端末の設定(java.util.prefs)に残して次の起動にも使う。
 */
object TextZoom {
    const val MIN = 0.8f
    const val MAX = 2.0f
    const val DEFAULT = 1.0f
    private const val STEPS_PER_UNIT = 10 // 0.1 刻み

    /** 読んだ値を範囲に収める(NaN・無限は既定)。0.1 刻みに丸める。 */
    fun sanitize(v: Float): Float = if (!v.isFinite()) DEFAULT else snap(v).coerceIn(MIN, MAX)

    fun bigger(v: Float): Float = sanitize(snap(v) + 1f / STEPS_PER_UNIT)

    fun smaller(v: Float): Float = sanitize(snap(v) - 1f / STEPS_PER_UNIT)

    /** 表示用の「120%」。 */
    fun percent(v: Float): String = "${(v * 100).roundToInt()}%"

    private fun snap(v: Float): Float = (v * STEPS_PER_UNIT).roundToInt().toFloat() / STEPS_PER_UNIT

    /** アプリに 1 つ(全部のウィンドウで同じ倍率)。 */
    val app: TextZoomState by lazy { TextZoomState(PrefsFloatSetting("textZoom", DEFAULT)) }
}

/** [content] の文字を [state] の倍率で描く(OS の fontScale に掛ける)。 */
@Composable
fun ProvideTextZoom(state: TextZoomState = TextZoom.app, content: @Composable () -> Unit) {
    val d = LocalDensity.current
    CompositionLocalProvider(LocalDensity provides Density(d.density, d.fontScale * state.scale), content = content)
}

/** 小数 1 つの設定の口。 */
interface FloatSetting {
    fun get(): Float
    fun set(value: Float)
}

class MemoryFloatSetting(private var value: Float) : FloatSetting {
    override fun get() = value
    override fun set(value: Float) {
        this.value = value
    }
}

/** java.util.prefs(Windows はレジストリ、Linux は ~/.java)。ウィンドウの大きさ・トレイの設定と同じ場所。 */
class PrefsFloatSetting(private val key: String, private val default: Float) : FloatSetting {
    private val prefs by lazy { Preferences.userRoot().node("jp/kz2601/tailorit/desktop") }
    override fun get() = runCatching { prefs.getFloat(key, default) }.getOrDefault(default)
    override fun set(value: Float) {
        runCatching { prefs.putFloat(key, value) }
    }
}

/** 今の倍率。変えると Compose が描き直し、設定にも書く。 */
class TextZoomState(private val setting: FloatSetting) {
    var scale by mutableFloatStateOf(TextZoom.sanitize(setting.get()))
        private set

    fun zoomIn() = update(TextZoom.bigger(scale))

    fun zoomOut() = update(TextZoom.smaller(scale))

    fun reset() = update(TextZoom.DEFAULT)

    /** 変わった時だけ書く。戻り値は新しい倍率。 */
    private fun update(v: Float): Float {
        if (v != scale) {
            scale = v
            setting.set(v)
        }
        return v
    }
}
