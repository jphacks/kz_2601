package jp.kz2601.metamon.desktop.power

import com.sun.jna.Native
import com.sun.jna.win32.StdCallLibrary
import java.util.concurrent.Executors

/**
 * Windows: kernel32 の SetThreadExecutionState で画面を消さない。
 * この関数は呼んだスレッドに効くので、必ず同じ 1 本のスレッドから呼ぶ(ES_CONTINUOUS の状態はそのスレッドが終わると消える)。
 */
object WindowsScreenAwake : ScreenAwakeApi {
    internal val isSupported = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    private const val ES_CONTINUOUS = 0x80000000.toInt()
    private const val ES_DISPLAY_REQUIRED = 0x00000002

    internal fun flags(on: Boolean): Int = if (on) ES_CONTINUOUS or ES_DISPLAY_REQUIRED else ES_CONTINUOUS

    private val kernel32: Kernel32? by lazy {
        if (!isSupported) null else runCatching { Native.load("kernel32", Kernel32::class.java) }.getOrNull()
    }

    private val worker by lazy {
        Executors.newSingleThreadExecutor { r -> Thread(r, "screen-awake").apply { isDaemon = true } }
    }

    override fun setAwake(on: Boolean) {
        val k = kernel32 ?: return
        runCatching { worker.submit { k.SetThreadExecutionState(flags(on)) }.get() }
    }

    @Suppress("FunctionName")
    internal interface Kernel32 : StdCallLibrary {
        fun SetThreadExecutionState(esFlags: Int): Int
    }
}
