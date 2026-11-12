package jp.kz2601.metamon.desktop

import java.io.File
import java.nio.charset.StandardCharsets
import java.util.concurrent.TimeUnit

// ログイン時に開く(Mac の MacLoginItem と同じ役)。Windows は HKCU の Run、Linux は XDG の autostart。既定は off で、自分からは登録しない。

/** ログイン時に開くかの口(テストでは偽物に替える)。状態は毎回 OS 側を見る(外で外されても合わせる)。 */
interface LoginItem {
    fun isEnabled(): Boolean

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

/** Windows: HKCU\...\Run の値 1 つ。書く時は .reg を取り込む(空白を含む道を引用符ごと残すため)。 */
class WindowsRunKey(
    private val appPath: String,
    private val runner: CommandRunner = CommandRunner.System,
    private val tempDir: File = File(System.getProperty("java.io.tmpdir")),
) : LoginItem {
    override fun isEnabled(): Boolean = runner.run(listOf("reg", "query", KEY, "/v", VALUE)) == 0

    override fun setEnabled(on: Boolean) {
        val code = if (on) {
            val file = File.createTempFile("tailorit-autostart", ".reg", tempDir)
            try {
                // reg import は UTF-16LE(BOM 付き)を読む。日本語の道でも化けない
                file.writeBytes(byteArrayOf(0xFF.toByte(), 0xFE.toByte()) + regFile(appPath).toByteArray(StandardCharsets.UTF_16LE))
                runner.run(listOf("reg", "import", file.absolutePath))
            } finally {
                file.delete()
            }
        } else {
            if (!isEnabled()) return
            runner.run(listOf("reg", "delete", KEY, "/v", VALUE, "/f"))
        }
        check(code == 0) { "reg exited with $code" }
    }

    companion object {
        const val KEY = "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run"
        const val VALUE = "TailorIt"

        /** 値は `"<道>"`(引用符付き)。.reg の文字列は \ と " を \ で逃がす。 */
        fun regFile(appPath: String): String {
            fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
            return "Windows Registry Editor Version 5.00\r\n\r\n" +
                "[HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run]\r\n" +
                "\"$VALUE\"=\"${esc("\"$appPath\"")}\"\r\n"
        }
    }
}

/** Linux: `$XDG_CONFIG_HOME/autostart/tailorit.desktop`(無ければ `~/.config/autostart`)。 */
class XdgAutostart(private val appPath: String, private val autostartDir: File) : LoginItem {
    val file: File get() = File(autostartDir, FILE_NAME)

    override fun isEnabled(): Boolean = file.isFile

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

        /** Desktop Entry の Exec の引用(" ` $ \ を逃がし、% は %% にする)。 */
        fun execQuote(path: String): String {
            val inner = path.replace(Regex("[\"`$\\\\]")) { "\\" + it.value }
            return "\"$inner\"".replace("%", "%%")
        }
    }
}

object Autostart {
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
        if (appPath.isNullOrBlank()) return null
        val os = osName.lowercase()
        return when {
            os.startsWith("windows") -> WindowsRunKey(appPath, runner)
            os.startsWith("mac") || os.startsWith("darwin") -> null
            home.isNotBlank() -> XdgAutostart(appPath, XdgAutostart.dir(env, home))
            else -> null
        }
    }
}
