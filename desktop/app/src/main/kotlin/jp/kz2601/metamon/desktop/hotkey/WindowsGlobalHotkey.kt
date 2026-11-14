package jp.kz2601.metamon.desktop.hotkey

import com.sun.jna.Memory
import com.sun.jna.Native
import com.sun.jna.Pointer
import com.sun.jna.win32.StdCallLibrary
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/** RegisterHotKey まわりの Win32 の呼び出し(テストで作り物に替える)。全部キーを受けるスレッドから呼ぶ(postQuit だけ他から)。 */
internal interface Win32Hotkeys {
    sealed interface Message {
        data class Hotkey(val id: Int) : Message
        data object Other : Message

        /** WM_QUIT・GetMessage の失敗。 */
        data object Quit : Message
    }

    fun currentThreadId(): Int

    /** このスレッドのメッセージの列を作る(PostThreadMessage を受けられるように)。 */
    fun ensureQueue()

    /** 登録できなければ GetLastError の番号、できれば null。 */
    fun register(id: Int, modifiers: Int, vk: Int): Int?

    fun unregister(id: Int)

    /** 次のメッセージまで待つ。 */
    fun nextMessage(): Message

    fun postQuit(threadId: Int)
}

/**
 * Windows: 専用のスレッドでメッセージを回し、`RegisterHotKey`(ウィンドウ無し・スレッドの列に WM_HOTKEY が来る)で受ける。
 * 外す時は WM_QUIT を送ってループを抜け、同じスレッドで `UnregisterHotKey`(登録したスレッドでしか外せない)。
 */
class WindowsGlobalHotkey internal constructor(
    api: () -> Win32Hotkeys?,
    private val timeoutMillis: Long = 2_000,
) : GlobalHotkey {
    constructor() : this({ User32Hotkeys.load() })

    private val win: Win32Hotkeys? by lazy(api)

    private class Running(val thread: Thread, val threadId: Int)

    private var running: Running? = null

    @Synchronized
    override fun register(chord: HotkeyChord, onPressed: () -> Unit): HotkeyRegistration {
        unregister()
        val win = win ?: return HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED)
        val started = CountDownLatch(1)
        // スレッドで決めて latch の後に読む(latch が書いた値を見せる)
        var result: HotkeyRegistration = HotkeyRegistration.Failed(HotkeyFailure.ERROR, "timeout")
        var threadId = 0
        val cancelled = AtomicBoolean(false)
        val t = Thread({
            threadId = win.currentThreadId()
            win.ensureQueue()
            val error = win.register(HOTKEY_ID, modifiers(chord), chord.vk)
            result = if (error == null) HotkeyRegistration.Registered(chord) else failure(error)
            started.countDown()
            if (error != null) return@Thread
            try {
                // 待ちきれずに諦めた後で登録できた時は、持ち主がいないので外して終わる
                while (!cancelled.get()) {
                    when (val m = win.nextMessage()) {
                        is Win32Hotkeys.Message.Hotkey -> if (m.id == HOTKEY_ID) runCatching(onPressed)
                        Win32Hotkeys.Message.Other -> Unit
                        Win32Hotkeys.Message.Quit -> break
                    }
                }
            } finally {
                win.unregister(HOTKEY_ID)
            }
        }, "global-hotkey")
        t.isDaemon = true
        t.start()
        if (!started.await(timeoutMillis, TimeUnit.MILLISECONDS)) {
            cancelled.set(true)
            return HotkeyRegistration.Failed(HotkeyFailure.ERROR, "timeout")
        }
        val r = Running(t, threadId)
        if (result is HotkeyRegistration.Registered) running = r else t.join(timeoutMillis)
        return result
    }

    @Synchronized
    override fun unregister() {
        val r = running ?: return
        running = null
        win?.postQuit(r.threadId)
        r.thread.join(timeoutMillis)
    }

    internal companion object {
        const val HOTKEY_ID = 0x2201
        private const val MOD_ALT = 0x0001
        private const val MOD_CONTROL = 0x0002
        private const val MOD_SHIFT = 0x0004
        private const val MOD_WIN = 0x0008

        /** 押しっぱなしで何度も来ないように。 */
        private const val MOD_NOREPEAT = 0x4000

        /** 他のアプリ(か自分)が同じ組みを登録済み。 */
        const val ERROR_HOTKEY_ALREADY_REGISTERED = 1409

        fun modifiers(c: HotkeyChord): Int =
            (if (c.alt) MOD_ALT else 0) or (if (c.ctrl) MOD_CONTROL else 0) or (if (c.shift) MOD_SHIFT else 0) or
                (if (c.meta) MOD_WIN else 0) or MOD_NOREPEAT

        fun failure(lastError: Int): HotkeyRegistration.Failed =
            if (lastError == ERROR_HOTKEY_ALREADY_REGISTERED) {
                HotkeyRegistration.Failed(HotkeyFailure.IN_USE)
            } else {
                HotkeyRegistration.Failed(HotkeyFailure.ERROR, "GetLastError=$lastError")
            }
    }
}

/** 本物の user32 / kernel32(JNA)。Windows でだけ載せる。 */
internal class User32Hotkeys private constructor(private val user32: User32, private val kernel32: Kernel32) : Win32Hotkeys {
    private val msg = Memory(MSG_BYTES)

    override fun currentThreadId(): Int = kernel32.GetCurrentThreadId()

    override fun ensureQueue() {
        user32.PeekMessageW(msg, null, 0, 0, PM_NOREMOVE)
    }

    override fun register(id: Int, modifiers: Int, vk: Int): Int? =
        if (user32.RegisterHotKey(null, id, modifiers, vk)) null else Native.getLastError()

    override fun unregister(id: Int) {
        user32.UnregisterHotKey(null, id)
    }

    override fun nextMessage(): Win32Hotkeys.Message {
        val r = user32.GetMessageW(msg, null, 0, 0)
        if (r <= 0) return Win32Hotkeys.Message.Quit
        // MSG: hwnd(ポインタ) → message(UINT) → wParam(ポインタの幅・ポインタの境目に揃う)
        val message = msg.getInt(Native.POINTER_SIZE.toLong())
        if (message != WM_HOTKEY) return Win32Hotkeys.Message.Other
        val off = 2L * Native.POINTER_SIZE
        val id = if (Native.POINTER_SIZE == 8) msg.getLong(off).toInt() else msg.getInt(off)
        return Win32Hotkeys.Message.Hotkey(id)
    }

    override fun postQuit(threadId: Int) {
        user32.PostThreadMessageW(threadId, WM_QUIT, null, null)
    }

    @Suppress("FunctionName")
    interface User32 : StdCallLibrary {
        fun RegisterHotKey(hWnd: Pointer?, id: Int, fsModifiers: Int, vk: Int): Boolean
        fun UnregisterHotKey(hWnd: Pointer?, id: Int): Boolean
        fun GetMessageW(lpMsg: Pointer, hWnd: Pointer?, wMsgFilterMin: Int, wMsgFilterMax: Int): Int
        fun PeekMessageW(lpMsg: Pointer, hWnd: Pointer?, wMsgFilterMin: Int, wMsgFilterMax: Int, wRemoveMsg: Int): Boolean
        fun PostThreadMessageW(idThread: Int, msg: Int, wParam: Pointer?, lParam: Pointer?): Boolean
    }

    @Suppress("FunctionName")
    interface Kernel32 : StdCallLibrary {
        fun GetCurrentThreadId(): Int
    }

    companion object {
        private const val WM_QUIT = 0x0012
        private const val WM_HOTKEY = 0x0312
        private const val PM_NOREMOVE = 0x0000

        /** MSG は 64bit で 48 バイト。余裕を持たせる。 */
        private const val MSG_BYTES = 64L

        fun load(): Win32Hotkeys? {
            if (!System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)) return null
            return runCatching {
                User32Hotkeys(Native.load("user32", User32::class.java), Native.load("kernel32", Kernel32::class.java))
            }.getOrNull()
        }
    }
}
