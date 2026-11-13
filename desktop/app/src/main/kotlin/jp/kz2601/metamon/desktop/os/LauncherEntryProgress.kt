package jp.kz2601.metamon.desktop.os

import java.util.Locale
import java.util.concurrent.TimeUnit

/** 外のコマンドを動かす物(テストでは作り物に替える)。動かせた(終了コード 0)なら true。 */
fun interface CommandRunner {
    fun run(command: List<String>): Boolean
}

/** 実際に動かす。コマンドが無い・時間切れ・失敗は false(例外は外に出さない)。 */
object SystemCommandRunner : CommandRunner {
    private const val TIMEOUT_SECONDS = 2L

    override fun run(command: List<String>): Boolean = runCatching {
        val p = ProcessBuilder(command).redirectErrorStream(true).start()
        p.outputStream.close()
        if (!p.waitFor(TIMEOUT_SECONDS, TimeUnit.SECONDS)) {
            p.destroyForcibly()
            return false
        }
        p.exitValue() == 0
    }.getOrDefault(false)
}

/**
 * Linux のドックに動いているタイマーの進み具合を出す(#3011・`com.canonical.Unity.LauncherEntry`)。
 * Ubuntu Dock・KDE Plasma のタスクマネージャーが読む `Update` の信号の `progress`(0.0〜1.0)と `progress-visible` を D-Bus に送る。
 * 送るのは `gdbus emit`(GLib 付属)。Linux 以外・gdbus や D-Bus のセッションが無い環境では何もしない(落ちない)。
 *
 * @param desktopFile ドックが探すアプリの .desktop の名前(例 `tailorit.desktop`)
 */
class LauncherEntryProgress(
    private val desktopFile: String,
    private val runner: CommandRunner = SystemCommandRunner,
    private val isLinux: Boolean = System.getProperty("os.name").orEmpty().startsWith("Linux", ignoreCase = true),
) {
    private var last: String? = null

    /** [fraction] は 0.0〜1.0 の進み具合。null(動いているタイマーが無い)なら消す。同じ値は送り直さない。 */
    @Synchronized
    fun update(fraction: Double?) {
        if (!isLinux) return
        val variant = properties(fraction)
        if (variant == last) return
        // 送れなかった時は覚えない(次に送り直す)
        if (runner.run(command(desktopFile, variant))) last = variant
    }

    /** 消す。 */
    fun clear() = update(null)

    companion object {
        /** `gdbus emit` の引数。信号は `/` から、`application://<.desktop>` と GVariant の辞書を渡す。 */
        fun command(desktopFile: String, properties: String): List<String> = listOf(
            "gdbus", "emit", "--session",
            "--object-path", "/",
            "--signal", "com.canonical.Unity.LauncherEntry.Update",
            "application://$desktopFile",
            properties,
        )

        /** GVariant の辞書。進み具合は 0〜1 に収め、小数は「.」で(OS の地域の書き方にしない)。 */
        fun properties(fraction: Double?): String {
            if (fraction == null || fraction.isNaN()) return "{'progress-visible': <false>, 'progress': <0.0>}"
            val p = fraction.coerceIn(0.0, 1.0)
            return "{'progress-visible': <true>, 'progress': <${String.format(Locale.ROOT, "%.3f", p)}>}"
        }
    }
}
