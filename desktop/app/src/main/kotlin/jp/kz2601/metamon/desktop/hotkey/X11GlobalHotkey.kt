package jp.kz2601.metamon.desktop.hotkey

import com.sun.jna.Callback
import com.sun.jna.Library
import com.sun.jna.Memory
import com.sun.jna.Native
import com.sun.jna.NativeLong
import com.sun.jna.Pointer
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean

/** Linux の画面の種類(#2202)。キーの受け方が違う。 */
enum class LinuxDisplay {
    /** X11: XGrabKey で受ける。 */
    X11,

    /** Wayland: アプリが勝手にキーを取れない(GlobalShortcuts のポータルに頼む)。 */
    WAYLAND,

    /** 画面が無い(ssh・ヘッドレス)。 */
    NONE,
    ;

    companion object {
        /** `$WAYLAND_DISPLAY` があれば Wayland(XWayland の `$DISPLAY` があっても X11 では取らない)。無くて `$DISPLAY` があれば X11。 */
        fun detect(env: (String) -> String? = System::getenv): LinuxDisplay = when {
            !env("WAYLAND_DISPLAY").isNullOrBlank() -> WAYLAND
            !env("DISPLAY").isNullOrBlank() -> X11
            else -> NONE
        }
    }
}

/** X11 の修飾キーの bit と keysym(X11/X.h・X11/keysymdef.h)。 */
internal object X11Keys {
    const val SHIFT_MASK = 1 shl 0
    const val LOCK_MASK = 1 shl 1
    const val CONTROL_MASK = 1 shl 2

    /** Alt。 */
    const val MOD1_MASK = 1 shl 3

    /** NumLock(多くの配列で)。 */
    const val MOD2_MASK = 1 shl 4

    /** Super(Windows キー)。 */
    const val MOD4_MASK = 1 shl 6

    fun modifiers(c: HotkeyChord): Int =
        (if (c.shift) SHIFT_MASK else 0) or (if (c.ctrl) CONTROL_MASK else 0) or (if (c.alt) MOD1_MASK else 0) or (if (c.meta) MOD4_MASK else 0)

    /** CapsLock・NumLock が付いていても押したことになるよう、4 通りを全部つかむ。 */
    fun withLocks(mods: Int): List<Int> = listOf(0, LOCK_MASK, MOD2_MASK, LOCK_MASK or MOD2_MASK).map { mods or it }

    /** [HotkeyChord.vk](Windows の仮想キー)→ keysym。知らないキーは null。 */
    fun keysym(c: HotkeyChord): Long? = when (val vk = c.vk) {
        0x20 -> 0x0020L // space
        0x0D -> 0xFF0DL // Return
        0x09 -> 0xFF09L // Tab
        0x1B -> 0xFF1BL // Escape
        0x2D -> 0xFF63L // Insert
        0x24 -> 0xFF50L // Home
        0x23 -> 0xFF57L // End
        0x13 -> 0xFF13L // Pause
        in 0x41..0x5A -> 0x61L + (vk - 0x41) // a-z(小文字の keysym)
        in 0x30..0x39 -> vk.toLong() // 0-9
        in 0x70..0x87 -> 0xFFBEL + (vk - 0x70) // F1-F24
        else -> null
    }
}

/** Xlib の呼び出し(テストで作り物に替える)。[close] 以外は全部キーを受けるスレッドから呼ぶ。 */
internal interface Xlib {
    sealed interface Event {
        data class Press(val keycode: Int) : Event
        data class Release(val keycode: Int) : Event

        /** 待つ間に何も来なかった・関係ない出来事。 */
        data object Idle : Event
    }

    /** XOpenDisplay。開けなければ false。 */
    fun open(): Boolean

    /** keysym → keycode。今の配列に無ければ 0。 */
    fun keycode(keysym: Long): Int

    /** [modifierSets] を全部 XGrabKey し、XSync で返ってきた X のエラーの番号(無ければ null)。 */
    fun grab(keycode: Int, modifierSets: List<Int>): Int?

    fun ungrab(keycode: Int, modifierSets: List<Int>)

    /** 次の出来事を最大 [waitMillis] 待つ。 */
    fun next(waitMillis: Long): Event

    /** XCloseDisplay。 */
    fun close()
}

/** 押しっぱなしの自動の繰り返しで何度も openVoice しない。押した → 離すまでで 1 回。 */
internal class RepeatFilter {
    private var held = false

    fun press(): Boolean {
        if (held) return false
        held = true
        return true
    }

    fun release() {
        held = false
    }
}

/**
 * Linux の X11(#2202): 専用のスレッドで自分の Display を開き、ルートウィンドウに `XGrabKey` する。
 * 他のアプリが同じ組みをつかんでいれば X が BadAccess を返すので [HotkeyFailure.IN_USE]。
 * 外す時はスレッドに止めるよう知らせ、同じスレッドで `XUngrabKey` → `XCloseDisplay`。
 */
class X11GlobalHotkey internal constructor(
    private val xlib: () -> Xlib?,
    private val timeoutMillis: Long = 2_000,
    private val pollMillis: Long = 50,
) : GlobalHotkey {
    constructor() : this({ JnaXlib.load() })

    private class Running(val thread: Thread, val stop: AtomicBoolean)

    private var running: Running? = null

    @Synchronized
    override fun register(chord: HotkeyChord, onPressed: () -> Unit): HotkeyRegistration {
        unregister()
        val keysym = X11Keys.keysym(chord) ?: return HotkeyRegistration.Failed(HotkeyFailure.ERROR, "no keysym for ${chord.keyName}")
        val x = xlib() ?: return HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED)
        val mods = X11Keys.withLocks(X11Keys.modifiers(chord))
        val started = CountDownLatch(1)
        var result: HotkeyRegistration = HotkeyRegistration.Failed(HotkeyFailure.ERROR, "timeout")
        val stop = AtomicBoolean(false)
        val t = Thread({
            if (!x.open()) {
                result = HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED, "XOpenDisplay")
                started.countDown()
                return@Thread
            }
            var keycode = 0
            var grabbed = false
            try {
                keycode = x.keycode(keysym)
                if (keycode == 0) {
                    result = HotkeyRegistration.Failed(HotkeyFailure.ERROR, "no keycode for ${chord.keyName}")
                    return@Thread
                }
                val error = x.grab(keycode, mods)
                grabbed = true
                result = if (error == null) HotkeyRegistration.Registered(chord) else failure(error)
                if (error != null) return@Thread
                started.countDown()
                val repeat = RepeatFilter()
                while (!stop.get()) {
                    when (val e = x.next(pollMillis)) {
                        is Xlib.Event.Press -> if (e.keycode == keycode && repeat.press()) runCatching(onPressed)
                        is Xlib.Event.Release -> if (e.keycode == keycode) repeat.release()
                        Xlib.Event.Idle -> Unit
                    }
                }
            } finally {
                // 失敗した時も一部つかめた組みが残らないよう、必ず外して閉じる
                if (grabbed) x.ungrab(keycode, mods)
                x.close()
                started.countDown()
            }
        }, "global-hotkey-x11")
        t.isDaemon = true
        t.start()
        if (!started.await(timeoutMillis, TimeUnit.MILLISECONDS)) {
            stop.set(true)
            return HotkeyRegistration.Failed(HotkeyFailure.ERROR, "timeout")
        }
        if (result is HotkeyRegistration.Registered) running = Running(t, stop) else t.join(timeoutMillis)
        return result
    }

    @Synchronized
    override fun unregister() {
        val r = running ?: return
        running = null
        r.stop.set(true)
        r.thread.join(timeoutMillis)
    }

    internal companion object {
        /** X の BadAccess: 他のクライアントが同じ組みをつかんでいる。 */
        const val BAD_ACCESS = 10

        fun failure(xError: Int): HotkeyRegistration.Failed =
            if (xError == BAD_ACCESS) HotkeyRegistration.Failed(HotkeyFailure.IN_USE) else HotkeyRegistration.Failed(HotkeyFailure.ERROR, "X error $xError")
    }
}

/** 本物の libX11(JNA)。Linux で `$DISPLAY` がある時だけ載せる。 */
internal class JnaXlib private constructor(private val x: X11Lib) : Xlib {
    private var display: Pointer? = null
    private var root: NativeLong = NativeLong(0)
    private val event = Memory(XEVENT_BYTES)

    override fun open(): Boolean {
        val d = x.XOpenDisplay(null) ?: return false
        display = d
        root = x.XDefaultRootWindow(d)
        // 押しっぱなしの時に Release を挟まない(RepeatFilter で 1 回にする)
        runCatching { x.XkbSetDetectableAutoRepeat(d, true, null) }
        return true
    }

    override fun keycode(keysym: Long): Int = x.XKeysymToKeycode(display!!, NativeLong(keysym)).toInt() and 0xFF

    override fun grab(keycode: Int, modifierSets: List<Int>): Int? {
        val d = display!!
        val handler = ErrorCapture()
        val previous = x.XSetErrorHandler(handler)
        try {
            modifierSets.forEach { x.XGrabKey(d, keycode, it, root, false, GRAB_MODE_ASYNC, GRAB_MODE_ASYNC) }
            x.XSync(d, false)
        } finally {
            x.XSetErrorHandler(previous)
        }
        return handler.code
    }

    override fun ungrab(keycode: Int, modifierSets: List<Int>) {
        val d = display ?: return
        modifierSets.forEach { x.XUngrabKey(d, keycode, it, root) }
        x.XSync(d, false)
    }

    override fun next(waitMillis: Long): Xlib.Event {
        val d = display ?: return Xlib.Event.Idle
        if (x.XPending(d) <= 0) {
            Thread.sleep(waitMillis)
            return Xlib.Event.Idle
        }
        x.XNextEvent(d, event)
        val type = event.getInt(0)
        val keycode = event.getInt(KEYCODE_OFFSET)
        return when (type) {
            KEY_PRESS -> Xlib.Event.Press(keycode)
            KEY_RELEASE -> Xlib.Event.Release(keycode)
            else -> Xlib.Event.Idle
        }
    }

    override fun close() {
        display?.let { x.XCloseDisplay(it) }
        display = null
    }

    /** XGrabKey の失敗は XSync の間に非同期で来る。最初の番号だけ残す。 */
    private class ErrorCapture : X11Lib.XErrorHandler {
        @Volatile var code: Int? = null

        override fun callback(display: Pointer?, errorEvent: Pointer?): Int {
            // XErrorEvent: int type, Display*, XID resourceid, unsigned long serial, unsigned char error_code
            if (code == null && errorEvent != null) code = errorEvent.getByte(4L * NativeLong.SIZE).toInt() and 0xFF
            return 0
        }
    }

    @Suppress("FunctionName")
    interface X11Lib : Library {
        interface XErrorHandler : Callback {
            fun callback(display: Pointer?, errorEvent: Pointer?): Int
        }

        fun XOpenDisplay(name: String?): Pointer?
        fun XCloseDisplay(display: Pointer): Int
        fun XDefaultRootWindow(display: Pointer): NativeLong
        fun XKeysymToKeycode(display: Pointer, keysym: NativeLong): Byte
        fun XGrabKey(display: Pointer, keycode: Int, modifiers: Int, grabWindow: NativeLong, ownerEvents: Boolean, pointerMode: Int, keyboardMode: Int): Int
        fun XUngrabKey(display: Pointer, keycode: Int, modifiers: Int, grabWindow: NativeLong): Int
        fun XSync(display: Pointer, discard: Boolean): Int
        fun XPending(display: Pointer): Int
        fun XNextEvent(display: Pointer, event: Pointer): Int
        fun XSetErrorHandler(handler: XErrorHandler?): XErrorHandler?
        fun XkbSetDetectableAutoRepeat(display: Pointer, detectable: Boolean, supported: Pointer?): Boolean
    }

    companion object {
        private const val KEY_PRESS = 2
        private const val KEY_RELEASE = 3
        private const val GRAB_MODE_ASYNC = 1

        /** XEvent は long 24 個分の共用体。 */
        private val XEVENT_BYTES = 24L * NativeLong.SIZE

        /**
         * XKeyEvent の keycode の位置: type, serial, send_event, display, window, root, subwindow, time,
         * x, y, x_root, y_root, state の後。long が 8 バイトなら 84、4 バイトなら 52。
         */
        private val KEYCODE_OFFSET = 8L * NativeLong.SIZE + 5 * 4

        fun load(
            osName: String = System.getProperty("os.name").orEmpty(),
            env: (String) -> String? = System::getenv,
        ): Xlib? {
            if (!osName.startsWith("Linux", ignoreCase = true) || LinuxDisplay.detect(env) != LinuxDisplay.X11) return null
            return runCatching { JnaXlib(Native.load("X11", X11Lib::class.java)) }.getOrNull()
        }
    }
}
