package jp.kz2601.metamon.desktop.auth

import jp.kz2601.metamon.desktop.i18n.Strings
import java.io.IOException
import java.util.concurrent.TimeUnit

/** D-Bus のセッションバス(作り物に差し替えられる形)。 */
interface SessionBus {
    /** バスがあるか(`DBUS_SESSION_BUS_ADDRESS`)。無ければ [names] は呼ばない。 */
    val present: Boolean

    /** 今いる名前と、呼べば起きる名前(gnome-keyring などは呼ばれて起きる)。調べられなければ投げる。 */
    fun names(): Set<String>
}

/** `dbus-send` で ListNames と ListActivatableNames を聞く(依存を足さない)。 */
class DbusSendSessionBus(
    private val env: (String) -> String? = System::getenv,
    private val timeoutSeconds: Long = 2,
) : SessionBus {
    override val present: Boolean get() = !env("DBUS_SESSION_BUS_ADDRESS").isNullOrBlank()

    override fun names(): Set<String> = listOf("ListNames", "ListActivatableNames").flatMapTo(mutableSetOf()) { call(it) }

    private fun call(method: String): List<String> {
        val p = ProcessBuilder(
            "dbus-send", "--session", "--print-reply", "--dest=org.freedesktop.DBus",
            "/org/freedesktop/DBus", "org.freedesktop.DBus.$method",
        ).redirectErrorStream(true).start()
        if (!p.waitFor(timeoutSeconds, TimeUnit.SECONDS)) {
            p.destroyForcibly()
            throw IOException("dbus-send $method timed out")
        }
        val out = p.inputStream.bufferedReader().readText()
        if (p.exitValue() != 0) throw IOException("dbus-send $method failed: ${out.take(200)}")
        return parseNames(out)
    }

    companion object {
        private val NAME = Regex("""string "([^"]+)"""")

        /** `--print-reply` の `string "…"` の行を拾う。 */
        fun parseNames(reply: String): List<String> = NAME.findAll(reply).map { it.groupValues[1] }.toList()
    }
}

/**
 * Linux の Secret Service(`org.freedesktop.secrets`)が使えるかを調べる(#2153 の 1)。
 * 使えない時(ヘッドレス・最小のデスクトップ・ポータルの無い Flatpak)は [NoTokenVault] に落ち、理由の文言を持つ。
 */
class LinuxSecretProbe(private val bus: SessionBus = DbusSendSessionBus()) {
    enum class Reason(val messageKey: String) {
        /** セッションバスが無い(ヘッドレス・SSH・サービスとして動いている)。 */
        NO_SESSION_BUS("desktop_keyring_no_session_bus"),

        /** バスはあるが Secret Service がいない(鍵束の無い最小のデスクトップ・Flatpak でポータルが無い)。 */
        NO_SECRET_SERVICE("desktop_keyring_no_service"),

        /** 調べるのに失敗した(dbus-send が無い・時間切れ・バスが答えない)。 */
        PROBE_FAILED("desktop_keyring_probe_failed"),
    }

    sealed interface Result {
        data object Available : Result

        data class Unavailable(val reason: Reason) : Result {
            /** 設定に出す文言(今の言語)。 */
            val message: String get() = Strings[reason.messageKey]
        }
    }

    fun probe(): Result {
        if (!bus.present) return Result.Unavailable(Reason.NO_SESSION_BUS)
        val names = try {
            bus.names()
        } catch (e: Exception) {
            return Result.Unavailable(Reason.PROBE_FAILED)
        }
        return if (SECRET_SERVICE in names) Result.Available else Result.Unavailable(Reason.NO_SECRET_SERVICE)
    }

    /** 使えれば [available] で作り、使えなければ「保存しない」に落とす。Secret Service に置く実装は次(#2153)。 */
    fun vault(available: () -> TokenVault): TokenVault = when (val r = probe()) {
        Result.Available -> available()
        is Result.Unavailable -> NoTokenVault(r.reason.messageKey)
    }

    companion object {
        const val SECRET_SERVICE = "org.freedesktop.secrets"
    }
}
