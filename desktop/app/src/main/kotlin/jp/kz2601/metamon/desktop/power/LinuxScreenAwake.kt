package jp.kz2601.metamon.desktop.power

/**
 * D-Bus の `org.freedesktop.ScreenSaver` を呼ぶ口。実機の接続は差し替えられるようにして、規則(数え方・失敗の扱い)だけをここで持つ。
 * セッションバスに繋げない・名前が無い時は [inhibit] が null を返すか例外を投げる(どちらも「何もしない」になる)。
 */
interface ScreenSaverBus {
    /** `Inhibit(app, reason)` を呼んで cookie を返す。 */
    fun inhibit(appName: String, reason: String): Int?

    /** `UnInhibit(cookie)`。 */
    fun unInhibit(cookie: Int)

    companion object {
        const val SERVICE = "org.freedesktop.ScreenSaver"
        const val PATH = "/org/freedesktop/ScreenSaver"
        const val INTERFACE = "org.freedesktop.ScreenSaver"

        /** バスが無い環境の既定。何もしない。 */
        val None: ScreenSaverBus = object : ScreenSaverBus {
            override fun inhibit(appName: String, reason: String): Int? = null
            override fun unInhibit(cookie: Int) = Unit
        }
    }
}

/**
 * Linux で、画面を消したくない部品を表示している間は画面を消さない(#2991)。
 * [hold] が 1 つ以上ある間だけ `Inhibit` を 1 回持ち、全部 [release] したら `UnInhibit` する。
 * D-Bus が無い・呼びが失敗する環境では落ちず、何もしない(hold は false を返す)。
 */
class LinuxScreenAwake(
    private val bus: ScreenSaverBus = ScreenSaverBus.None,
    private val appName: String = "Tailor It",
) {
    private var holders = 0
    private var cookie: Int? = null

    /** 画面を消さないでほしい部品が出た。実際に抑止できていれば true。 */
    @Synchronized
    fun hold(reason: String): Boolean {
        holders++
        if (cookie == null) {
            cookie = runCatching { bus.inhibit(appName, reason) }
                .onFailure { System.err.println("[TailorIt] 画面を消さない設定ができませんでした: $it") }
                .getOrNull()
        }
        return cookie != null
    }

    /** [hold] の対。数が 0 になったら `UnInhibit`。多く呼んでも負にならない。 */
    @Synchronized
    fun release() {
        if (holders > 0) holders--
        if (holders == 0) releaseAll()
    }

    /** 終わる時など、数にかかわらず外す。 */
    @Synchronized
    fun releaseAll() {
        holders = 0
        val c = cookie ?: return
        cookie = null
        runCatching { bus.unInhibit(c) }
            .onFailure { System.err.println("[TailorIt] 画面を消さない設定を外せませんでした: $it") }
    }

    @get:Synchronized
    val isHeld: Boolean get() = cookie != null
}
