package jp.kz2601.metamon.desktop.os

import java.util.concurrent.TimeUnit

/**
 * GNOME の「通知を表示しない」(#2997)。`gsettings get org.gnome.desktop.notifications show-banners` が false の間は通知を控える。
 * #2996 の通知を控える口(QuietStateReader)の Linux の実装。口が epic/god に入ったら `GnomeBannerState.reader()` を `QuietStateReader` として繋ぐ。
 * gsettings が無い・失敗・読めない値の時は「出す」。
 */
object GnomeBannerState {
    /** `gsettings get` の出力。`true` / `false`。 */
    fun parse(out: String?): Boolean? = when (out?.trim()?.lowercase()) {
        "true" -> true
        "false" -> false
        else -> null
    }

    /** 今は控えるべきか。[run] は gsettings を呼んで標準出力を返す(失敗・無い時は null)。show-banners が false の時だけ true。 */
    fun isQuiet(run: (Array<String>) -> String? = ::runCommand): Boolean =
        runCatching { parse(run(COMMAND)) == false }.getOrDefault(false)

    /** [isQuiet] を `() -> Boolean` で。 */
    fun reader(run: (Array<String>) -> String? = ::runCommand): () -> Boolean = { isQuiet(run) }

    private val COMMAND = arrayOf("gsettings", "get", "org.gnome.desktop.notifications", "show-banners")

    private fun runCommand(cmd: Array<String>): String? = runCatching {
        val p = ProcessBuilder(*cmd).redirectErrorStream(true).start()
        if (!p.waitFor(2, TimeUnit.SECONDS)) {
            p.destroyForcibly()
            return null
        }
        val out = p.inputStream.bufferedReader().readText()
        if (p.exitValue() == 0) out else null
    }.getOrNull()
}
