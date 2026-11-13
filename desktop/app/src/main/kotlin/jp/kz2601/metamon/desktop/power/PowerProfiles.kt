package jp.kz2601.metamon.desktop.power

import java.io.IOException
import java.util.concurrent.TimeUnit

/** power-profiles-daemon の ActiveProfile("power-saver"・"balanced"・"performance")を返す口。分からない時は null。 */
fun interface PowerProfileSource {
    fun activeProfile(): String?

    companion object {
        /** 読まない(テスト・Linux 以外)。 */
        val None = PowerProfileSource { null }
    }
}

/** D-Bus の読み書きの最小の口。実体は gdbus、テストは作り物。 */
interface DbusClient {
    /** `org.freedesktop.DBus.Properties.Get` の応答の文字(gdbus の表記)。無い・失敗は null。 */
    fun getProperty(dest: String, path: String, iface: String, name: String): String?

    /** [dest] の通知を 1 行ずつ [onLine] に渡す。終わるまで戻らない。始められなければ false。 */
    fun monitor(dest: String, path: String, onLine: (String) -> Unit): Boolean
}

/**
 * power-profiles-daemon(新しい名前 `org.freedesktop.UPower.PowerProfiles`・古い名前 `net.hadess.PowerProfiles`)の ActiveProfile。
 * 最初に読んだ時だけ Get を呼び、その後は 1 本の monitor で PropertiesChanged を追う(読むたびにプロセスを起こさない)。
 * デーモンが居なくなったら null、また現れたら 1 回だけ読み直す。
 */
class DbusPowerProfileSource(
    private val client: DbusClient = GdbusClient(),
    private val startThread: (Runnable) -> Unit = { r -> Thread(r, "power-profiles-monitor").apply { isDaemon = true }.start() },
) : PowerProfileSource {
    @Volatile private var profile: String? = null
    private var started = false

    override fun activeProfile(): String? {
        synchronized(this) {
            if (!started) {
                started = true
                val found = NAMES.firstNotNullOfOrNull { n -> readProfile(n)?.let { n to it } }
                if (found != null) {
                    profile = found.second
                    startThread { watch(found.first) }
                }
            }
        }
        return profile
    }

    private fun readProfile(name: Name): String? =
        client.getProperty(name.dest, name.path, name.dest, "ActiveProfile")?.let(::parseGetReply)

    private fun watch(name: Name) {
        client.monitor(name.dest, name.path) { line -> onMonitorLine(name, line) }
        // monitor が終わった・始められない時は追えないので分からない扱いに戻す
        profile = null
    }

    private fun onMonitorLine(name: Name, line: String) {
        when {
            line.contains("does not have an owner") -> profile = null
            line.contains("is owned by") -> profile = readProfile(name)
            else -> parseMonitorLine(line)?.let { profile = it }
        }
    }

    internal data class Name(val dest: String, val path: String)

    internal companion object {
        val NAMES = listOf(
            Name("org.freedesktop.UPower.PowerProfiles", "/org/freedesktop/UPower/PowerProfiles"),
            Name("net.hadess.PowerProfiles", "/net/hadess/PowerProfiles"),
        )

        private val GET_REPLY = Regex("""^\(<'([^']*)'>,\)$""")
        private val CHANGED = Regex("""'ActiveProfile': <'([^']*)'>""")

        /** `(<'power-saver'>,)` → power-saver。 */
        fun parseGetReply(out: String): String? = GET_REPLY.find(out.trim())?.groupValues?.get(1)?.ifEmpty { null }

        /** PropertiesChanged の行から ActiveProfile を抜く。他の行は null。 */
        fun parseMonitorLine(line: String): String? =
            if (line.contains("PropertiesChanged")) CHANGED.find(line)?.groupValues?.get(1)?.ifEmpty { null } else null
    }
}

/** gdbus(GLib に付いてくる)で D-Bus を読む。無い環境では全部失敗(null・false)になる。 */
class GdbusClient(private val timeoutSeconds: Long = 2) : DbusClient {
    override fun getProperty(dest: String, path: String, iface: String, name: String): String? = try {
        val p = ProcessBuilder(
            "gdbus", "call", "--system", "--dest", dest, "--object-path", path,
            "--method", "org.freedesktop.DBus.Properties.Get", iface, name,
        ).redirectErrorStream(true).start()
        if (!p.waitFor(timeoutSeconds, TimeUnit.SECONDS)) {
            p.destroyForcibly()
            null
        } else if (p.exitValue() == 0) {
            p.inputStream.bufferedReader().readText()
        } else {
            null
        }
    } catch (_: IOException) {
        null
    } catch (_: SecurityException) {
        null
    }

    override fun monitor(dest: String, path: String, onLine: (String) -> Unit): Boolean = try {
        val p = ProcessBuilder("gdbus", "monitor", "--system", "--dest", dest, "--object-path", path)
            .redirectErrorStream(true).start()
        p.inputStream.bufferedReader().useLines { lines -> lines.forEach(onLine) }
        true
    } catch (_: IOException) {
        false
    } catch (_: SecurityException) {
        false
    }
}
