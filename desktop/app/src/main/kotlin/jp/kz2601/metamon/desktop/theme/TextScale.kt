package jp.kz2601.metamon.desktop.theme

import java.util.concurrent.TimeUnit

/**
 * OS の「文字の大きさ」(iOS の Dynamic Type に当たる)を Compose の文字の倍率にする(#3000)。
 * - Windows: `HKCU\Software\Microsoft\Accessibility` の `TextScaleFactor`(100〜225 の %)
 * - Linux: `gsettings get org.gnome.desktop.interface text-scaling-factor`(1.0〜3.0。1 未満は 1.0 に上げる)
 * 読めない時は 1.0。
 */
object TextScale {
    const val WINDOWS_MIN_PERCENT = 100
    const val WINDOWS_MAX_PERCENT = 225

    /** OS の倍率と Ctrl + / Ctrl - の倍率を掛けた最終の上限。 */
    const val MAX_COMBINED = 3.0f
    const val MIN_COMBINED = 0.5f

    /** `reg query` の出力。`TextScaleFactor    REG_DWORD    0xb4`(0xb4 = 180)。範囲外は 100〜225 に丸め、読めなければ null。 */
    fun parseWindowsRegistry(out: String): Float? {
        val percent = Regex("""TextScaleFactor\s+REG_DWORD\s+0x([0-9a-fA-F]+)""").find(out)?.groupValues?.get(1)?.toIntOrNull(16) ?: return null
        return percent.coerceIn(WINDOWS_MIN_PERCENT, WINDOWS_MAX_PERCENT) / 100f
    }

    const val GNOME_MIN = 1.0f
    const val GNOME_MAX = 3.0f

    /** `gsettings get …` の出力。`1.25`(型付きで `double 1.25` と出る版もある)。範囲外は 1.0〜3.0 に丸め、読めなければ null。 */
    fun parseGsettings(out: String): Float? {
        val v = Regex("""-?\d+(?:\.\d+)?""").find(out.trim())?.value?.toFloatOrNull() ?: return null
        return if (v.isFinite()) v.coerceIn(GNOME_MIN, GNOME_MAX) else null
    }

    /** OS の倍率と Ctrl の倍率を掛け、[MIN_COMBINED]〜[MAX_COMBINED] に収める。 */
    fun combine(os: Float, ctrl: Float): Float = (os * ctrl).coerceIn(MIN_COMBINED, MAX_COMBINED)

    /** 端末の OS の倍率。読めない時は 1.0。[run] は外部コマンドの実行(試験で差し替える)。 */
    fun detect(
        os: String = System.getProperty("os.name").orEmpty().lowercase(),
        run: (Array<String>) -> String? = ::runCommand,
    ): Float = when {
        os.startsWith("windows") ->
            run(arrayOf("reg", "query", """HKCU\Software\Microsoft\Accessibility""", "/v", "TextScaleFactor"))?.let(::parseWindowsRegistry)
        os.contains("mac") -> null
        else -> run(arrayOf("gsettings", "get", "org.gnome.desktop.interface", "text-scaling-factor"))?.let(::parseGsettings)
    } ?: 1f

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
