package jp.kz2601.metamon.desktop

import java.io.File
import java.nio.charset.StandardCharsets
import java.util.concurrent.TimeUnit

// ログイン時に開く(Mac の MacLoginItem と同じ役)。Windows は HKCU の Run、Linux は XDG の autostart。既定は off で、自分からは登録しない。

/** ログイン時に開くかの口(テストでは偽物に替える)。状態は毎回 OS 側を見る(外で外されても合わせる)。重いので UI のスレッドで呼ばない。 */
interface LoginItem {
    /** 登録があり、今のアプリの道を指している。 */
    fun isEnabled(): Boolean

    /** 何か登録がある(前の道を指していても true)。 */
    fun isRegistered(): Boolean = isEnabled()

    /** 失敗したら投げる。 */
    fun setEnabled(on: Boolean)
}

/** 外のコマンドを動かして終了コードを返す。 */
fun interface CommandRunner {
    fun run(command: List<String>): Int

    companion object {
        val System = CommandRunner { cmd ->
            val p = ProcessBuilder(cmd).redirectErrorStream(true).redirectOutput(ProcessBuilder.Redirect.DISCARD).start()
            if (!p.waitFor(10, TimeUnit.SECONDS)) {
                p.destroyForcibly()
                -1
            } else {
                p.exitValue()
            }
        }
    }
}

/** Windows: HKCU\...\Run の値 1 つ。読み書きとも .reg(UTF-16LE)を通す(空白・日本語を含む道を引用符ごと残すため)。 */
class WindowsRunKey(
    private val appPath: String,
    private val runner: CommandRunner = CommandRunner.System,
    private val tempDir: File = File(System.getProperty("java.io.tmpdir")),
    /** reg.exe は絶対パスで呼ぶ。 */
    private val regExe: String = defaultRegExe(System.getenv("SystemRoot")),
) : LoginItem {
    init {
        require(Autostart.isSafePath(appPath)) { "app path must not contain line breaks" }
    }

    /** 今入っている値(無ければ null)。 */
    fun storedValue(): String? = withTempReg { file ->
        if (runner.run(listOf(regExe, "export", KEY, file.absolutePath, "/y")) != 0 || file.length() == 0L) {
            null
        } else {
            parseValue(decodeReg(file.readBytes()))
        }
    }

    override fun isRegistered(): Boolean = storedValue() != null

    override fun isEnabled(): Boolean = storedValue()?.equals(runValue(appPath), ignoreCase = true) == true

    override fun setEnabled(on: Boolean) {
        val code = if (on) {
            withTempReg { file ->
                // reg import は UTF-16LE(BOM 付き)を読む。日本語の道でも化けない
                file.writeBytes(byteArrayOf(0xFF.toByte(), 0xFE.toByte()) + regFile(appPath).toByteArray(StandardCharsets.UTF_16LE))
                runner.run(listOf(regExe, "import", file.absolutePath))
            }
        } else {
            if (!isRegistered()) return
            runner.run(listOf(regExe, "delete", KEY, "/v", VALUE, "/f"))
        }
        check(code == 0) { "reg exited with $code" }
    }

    private fun <T> withTempReg(block: (File) -> T): T {
        val file = File.createTempFile("tailorit-autostart", ".reg", tempDir)
        try {
            return block(file)
        } finally {
            file.delete()
        }
    }

    companion object {
        const val KEY = "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
        const val VALUE = "TailorIt"

        fun defaultRegExe(systemRoot: String?): String =
            File(systemRoot?.takeIf { it.isNotBlank() } ?: "C:\\Windows", "System32\\reg.exe").path

        /** Run に入れる値(引用符付きの道)。 */
        fun runValue(appPath: String): String = "\"$appPath\""

        private fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")

        /** 値は `"<道>"`(引用符付き)。.reg の文字列は \ と " を \ で逃がす。 */
        fun regFile(appPath: String): String =
            "Windows Registry Editor Version 5.00\r\n\r\n" +
                "[HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run]\r\n" +
                "\"$VALUE\"=\"${esc(runValue(appPath))}\"\r\n"

        /** reg export は UTF-16LE(BOM 付き)で書く。 */
        fun decodeReg(bytes: ByteArray): String =
            if (bytes.size >= 2 && bytes[0] == 0xFF.toByte() && bytes[1] == 0xFE.toByte()) {
                String(bytes, 2, bytes.size - 2, StandardCharsets.UTF_16LE)
            } else {
                String(bytes, StandardCharsets.UTF_16LE)
            }

        /** .reg の中から `"TailorIt"="..."` の文字列を取り出す(\\ と \" を戻す)。無ければ null。 */
        fun parseValue(reg: String): String? {
            val prefix = "\"$VALUE\"=\""
            val line = reg.lineSequence().map { it.trimEnd('\r') }.firstOrNull { it.startsWith(prefix, ignoreCase = true) } ?: return null
            val out = StringBuilder()
            var i = prefix.length
            while (i < line.length) {
                val ch = line[i]
                when {
                    ch == '\\' && i + 1 < line.length -> {
                        out.append(line[i + 1])
                        i += 2
                    }
                    ch == '"' -> return out.toString()
                    else -> {
                        out.append(ch)
                        i++
                    }
                }
            }
            return null
        }
    }
}

/** Linux: `$XDG_CONFIG_HOME/autostart/tailorit.desktop`(無ければ `~/.config/autostart`)。 */
class XdgAutostart(private val appPath: String, private val autostartDir: File) : LoginItem {
    init {
        require(Autostart.isSafePath(appPath)) { "app path must not contain line breaks" }
    }

    val file: File get() = File(autostartDir, FILE_NAME)

    override fun isRegistered(): Boolean = file.isFile

    /** Exec が今の道を指している時だけ。 */
    override fun isEnabled(): Boolean =
        file.isFile && runCatching { file.readLines() }.getOrDefault(emptyList()).any { it == "Exec=" + execQuote(appPath) }

    override fun setEnabled(on: Boolean) {
        if (on) {
            autostartDir.mkdirs()
            file.writeText(desktopEntry(appPath))
        } else if (file.exists()) {
            check(file.delete()) { "could not delete $file" }
        }
    }

    companion object {
        const val FILE_NAME = "tailorit.desktop"

        /** XDG の決まりどおり、相対の XDG_CONFIG_HOME は無視する。 */
        fun dir(env: Map<String, String>, home: String): File =
            env["XDG_CONFIG_HOME"]?.takeIf { it.isNotBlank() && File(it).isAbsolute }?.let { File(it, "autostart") }
                ?: File(home, ".config/autostart")

        fun desktopEntry(appPath: String): String = buildString {
            append("[Desktop Entry]\n")
            append("Type=Application\n")
            append("Name=Tailor It\n")
            append("Exec=").append(execQuote(appPath)).append('\n')
            append("X-GNOME-Autostart-enabled=true\n")
        }

        /**
         * Desktop Entry の Exec の値。Exec の引用(" ` $ \ を逃がし、% は %% にする)の後に、
         * 文字列の値としての逃がし(\ を \\ に)をかける。
         */
        fun execQuote(path: String): String {
            val inner = path.replace(Regex("[\"`$\\\\]")) { "\\" + it.value }
            return "\"$inner\"".replace("%", "%%").replace("\\", "\\\\")
        }
    }
}

object Autostart {
    /** 改行(CR・LF)を含む道は登録しない。 */
    fun isSafePath(path: String): Boolean = '\r' !in path && '\n' !in path

    /**
     * この OS のログイン項目。配布した形(jpackage)で動いている時だけ([appPath] = `jpackage.app-path`)。
     * `gradlew run` で動かしている時と macOS(Mac 版がある)は null で、トレイに出さない。
     */
    fun forCurrentOs(
        osName: String = System.getProperty("os.name").orEmpty(),
        appPath: String? = System.getProperty("jpackage.app-path"),
        env: Map<String, String> = System.getenv(),
        home: String = System.getProperty("user.home").orEmpty(),
        runner: CommandRunner = CommandRunner.System,
    ): LoginItem? {
        if (appPath.isNullOrBlank() || !isSafePath(appPath)) return null
        val os = osName.lowercase()
        return when {
            os.startsWith("windows") -> WindowsRunKey(appPath, runner, regExe = WindowsRunKey.defaultRegExe(env["SystemRoot"]))
            os.startsWith("mac") || os.startsWith("darwin") -> null
            home.isNotBlank() -> XdgAutostart(appPath, XdgAutostart.dir(env, home))
            else -> null
        }
    }
}
