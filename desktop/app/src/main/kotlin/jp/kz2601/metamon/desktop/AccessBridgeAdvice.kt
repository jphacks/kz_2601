package jp.kz2601.metamon.desktop

import com.sun.jna.Native
import com.sun.jna.win32.StdCallLibrary
import com.sun.jna.ptr.IntByReference
import jp.kz2601.metamon.desktop.store.AppDataDir
import java.io.File
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Windows で読み上げ(Narrator など)が動いているのに Java Access Bridge が有効でない時の案内(#2209)。
 * 案内は 1 回だけ自動で出し(標識ファイル)、コマンドパレットの「読み上げの案内」からいつでも開き直せる。
 */
object AccessBridgeAdvice {
    /** 案内が要るか(読み上げが動いていて、橋渡しが無効の Windows だけ)。 */
    fun decide(windows: Boolean, screenReaderOn: Boolean, bridgeEnabled: Boolean): Boolean = windows && screenReaderOn && !bridgeEnabled

    /** `accessibility.properties` の中身で Access Bridge が有効か(コメント行は見ない)。 */
    fun bridgeEnabledIn(properties: String): Boolean = properties.lineSequence()
        .map { it.trim() }
        .filterNot { it.startsWith("#") || it.startsWith("!") }
        .any { line ->
            val (key, value) = line.split('=', limit = 2).let { it[0].trim() to it.getOrNull(1).orEmpty() }
            key == "assistive_technologies" && value.split(',').any { it.trim() == ACCESS_BRIDGE_CLASS }
        }

    /** 利用者の `~/.accessibility.properties` か実行環境の `conf/accessibility.properties` のどちらかで有効なら true。 */
    fun bridgeEnabled(
        javaHome: String = System.getProperty("java.home").orEmpty(),
        userHome: String = System.getProperty("user.home").orEmpty(),
    ): Boolean = listOf(File(userHome, ".accessibility.properties"), File(javaHome, "conf/accessibility.properties"))
        .any { f -> runCatching { f.isFile && bridgeEnabledIn(f.readText()) }.getOrDefault(false) }

    private interface User32 : StdCallLibrary {
        fun SystemParametersInfoW(action: Int, param: Int, out: IntByReference, winIni: Int): Boolean
    }

    /** `SystemParametersInfo(SPI_GETSCREENREADER)`。読めない時は false。 */
    fun screenReaderOn(): Boolean = runCatching {
        val user32 = Native.load("user32", User32::class.java)
        val out = IntByReference()
        user32.SystemParametersInfoW(SPI_GETSCREENREADER, 0, out, 0) && out.value != 0
    }.getOrDefault(false)

    private val windows: Boolean get() = HostOs.current == HostOs.WINDOWS

    /** 今この PC で案内が要るか。 */
    fun needed(): Boolean = windows && decide(windows = true, screenReaderOn = screenReaderOn(), bridgeEnabled = bridgeEnabled())

    private val checked = AtomicBoolean(false)

    /** 起動して最初の 1 回だけ true(標識ファイルで次の起動以降も出さない)。 */
    fun shouldAutoShow(marker: File = File(AppDataDir.resolve(), "a11y-bridge-advice-shown")): Boolean {
        if (!windows || !checked.compareAndSet(false, true) || marker.exists()) return false
        if (!needed()) return false
        runCatching { marker.parentFile?.mkdirs(); marker.writeText("1") }
        return true
    }

    const val ACCESS_BRIDGE_CLASS = "com.sun.java.accessibility.AccessBridge"
    private const val SPI_GETSCREENREADER = 0x0046
}
