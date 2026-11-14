package jp.kz2601.metamon.desktop.os

import com.sun.jna.Native
import com.sun.jna.ptr.IntByReference
import com.sun.jna.win32.StdCallLibrary

/**
 * 通知を控える状態を読む口(#2996)。iOS の集中モードのフィルタの代わりの土台。
 * 控える時は通知を出さずトレイの一覧にだけ残す(出し分けは呼ぶ側)。読めない時・分からない時は「出す」。
 */
fun interface QuietStateReader {
    /** 今は通知を控えるべきなら true。読めない時は false。 */
    fun isQuiet(): Boolean
}

object NotificationState {
    /** 控える判断を使うか。既定は off。`-Dtailorit.notifyQuiet=1` / `TAILORIT_NOTIFY_QUIET=1`(1・true・on)で on。 */
    fun enabled(raw: String? = System.getProperty("tailorit.notifyQuiet") ?: System.getenv("TAILORIT_NOTIFY_QUIET")): Boolean =
        raw?.trim()?.lowercase() in setOf("1", "true", "on")

    /** on の時だけ [reader] を見る。off の時は読まず、常に「出す」。 */
    fun shouldHold(enabled: Boolean, reader: QuietStateReader): Boolean = enabled && runCatching { reader.isQuiet() }.getOrDefault(false)
}

/**
 * Windows: shell32 の `SHQueryUserNotificationState` の値(QUERY_USER_NOTIFICATION_STATE)。
 * 5 以外の「通知を受けにくい」状態(応答不可・全画面・プレゼン中・集中)で控える。
 */
object WindowsQuietState {
    const val NOT_PRESENT = 1
    const val BUSY = 2
    const val RUNNING_D3D_FULL_SCREEN = 3
    const val PRESENTATION_MODE = 4
    const val ACCEPTS_NOTIFICATIONS = 5
    const val QUIET_TIME = 6
    const val APP = 7

    /** 控える値か。5(受け付ける)・1(席を外している: 画面ロック・スクリーンセーバー)・分からない値・null は出す。 */
    fun isQuiet(quns: Int?): Boolean = when (quns) {
        BUSY, RUNNING_D3D_FULL_SCREEN, PRESENTATION_MODE, QUIET_TIME, APP -> true
        else -> false
    }

    /** [query] は QUNS を返す(失敗は null)。既定は shell32 を JNA で呼ぶ(Windows 以外・載らない時は null = 出す)。 */
    fun reader(query: () -> Int? = ::queryShell32): QuietStateReader = QuietStateReader { isQuiet(query()) }

    private val isWindows = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    private val shell32: Shell32? by lazy {
        if (!isWindows) null else runCatching { Native.load("shell32", Shell32::class.java) }.getOrNull()
    }

    private fun queryShell32(): Int? {
        val s = shell32 ?: return null
        return runCatching {
            val out = IntByReference()
            if (s.SHQueryUserNotificationState(out) == S_OK) out.value else null
        }.getOrNull()
    }

    private const val S_OK = 0

    @Suppress("FunctionName")
    internal interface Shell32 : StdCallLibrary {
        /** HRESULT を返す。 */
        fun SHQueryUserNotificationState(pquns: IntByReference): Int
    }
}
