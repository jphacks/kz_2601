package jp.kz2601.metamon.desktop.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.State
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import com.sun.jna.Native
import com.sun.jna.ptr.IntByReference
import com.sun.jna.win32.StdCallLibrary
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

/**
 * OS の「動きを減らす」設定を読む(#2998)。iOS・Android の「動きを減らす」に当たる。
 * - 固定: `-Dtailorit.reduceMotion` / `TAILORIT_REDUCE_MOTION` に 1・true・on で減らす、0・false・off で減らさない(OS を見ない)
 * - Windows: `SystemParametersInfo(SPI_GETCLIENTAREAANIMATION)` が FALSE(アニメーション効果が off)なら減らす
 * - Linux: `gsettings get org.gnome.desktop.interface enable-animations` が false なら減らす
 * 読めない時は減らさない(false)。
 */
object ReduceMotion {
    /** 環境変数・プロパティの固定。1・true・on は true、0・false・off は false、無い・それ以外は null(OS に従う)。 */
    fun parseOverride(raw: String?): Boolean? = when (raw?.trim()?.lowercase()) {
        "1", "true", "on" -> true
        "0", "false", "off" -> false
        else -> null
    }

    fun overrideFromEnv(): Boolean? = parseOverride(System.getProperty("tailorit.reduceMotion") ?: System.getenv("TAILORIT_REDUCE_MOTION"))

    /** `gsettings get org.gnome.desktop.interface enable-animations` の出力。アニメーションが off(false)なら動きを減らす = true。 */
    fun parseGsettings(out: String?): Boolean? = when (out?.trim()?.lowercase()) {
        "true" -> false
        "false" -> true
        else -> null
    }

    /** SPI_GETCLIENTAREAANIMATION の結果。呼べて(ok)値が 0 なら減らす。呼べなかった時は null。 */
    fun fromClientAreaAnimation(ok: Boolean, value: Int): Boolean? = if (ok) value == 0 else null

    /** 決める。固定があればそれ、無ければ OS、OS が読めなければ false。 */
    fun resolve(override: Boolean?, os: Boolean?): Boolean = override ?: os ?: false

    /** OS の値だけ。読めない時は null。[run] は gsettings を呼ぶ口、[windows] は SPI を呼ぶ口(どちらも差し替えられる)。 */
    fun detect(
        os: String = System.getProperty("os.name").orEmpty().lowercase(),
        windows: () -> Boolean? = ::querySpi,
        run: (Array<String>) -> String? = ::runCommand,
    ): Boolean? = runCatching {
        when {
            os.startsWith("windows") -> windows()
            os.contains("mac") -> null
            else -> parseGsettings(run(arrayOf("gsettings", "get", "org.gnome.desktop.interface", "enable-animations")))
        }
    }.getOrNull()

    /** 今の値(固定 → OS → false)。 */
    fun current(override: Boolean? = overrideFromEnv()): Boolean = resolve(override, if (override == null) detect() else null)

    private const val SPI_GETCLIENTAREAANIMATION = 0x1042

    private val isWindows = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    private val user32: User32? by lazy {
        if (!isWindows) null else runCatching { Native.load("user32", User32::class.java) }.getOrNull()
    }

    private fun querySpi(): Boolean? {
        val u = user32 ?: return null
        return runCatching {
            val v = IntByReference()
            fromClientAreaAnimation(u.SystemParametersInfoW(SPI_GETCLIENTAREAANIMATION, 0, v, 0), v.value)
        }.getOrNull()
    }

    private fun runCommand(cmd: Array<String>): String? = runCatching {
        val p = ProcessBuilder(*cmd).redirectErrorStream(true).start()
        if (!p.waitFor(2, TimeUnit.SECONDS)) {
            p.destroyForcibly()
            return null
        }
        val out = p.inputStream.bufferedReader().readText()
        if (p.exitValue() == 0) out else null
    }.getOrNull()

    @Suppress("FunctionName")
    internal interface User32 : StdCallLibrary {
        fun SystemParametersInfoW(uiAction: Int, uiParam: Int, pvParam: IntByReference, fWinIni: Int): Boolean
    }
}

/** 起動中に OS の設定が変わっても追いかける間隔(明暗と同じ)。 */
private const val POLL_MS = 5_000L

/** 動きを減らすか。アニメーションを足す所はこれを見て、true なら即時に切り替える。 */
val LocalReduceMotion = compositionLocalOf { false }

/** 今の「動きを減らす」。固定が無い時だけ OS を [POLL_MS] ごとに読み直す(読むのは裏のスレッド)。 */
@Composable
fun rememberReduceMotion(override: Boolean? = ReduceMotion.overrideFromEnv()): State<Boolean> {
    val state = remember(override) { mutableStateOf(override ?: false) }
    if (override == null) {
        LaunchedEffect(Unit) {
            while (true) {
                state.value = ReduceMotion.resolve(null, withContext(Dispatchers.IO) { ReduceMotion.detect() })
                delay(POLL_MS)
            }
        }
    }
    return state
}
