package jp.kz2601.metamon.desktop.hotkey

import jp.kz2601.metamon.desktop.i18n.Strings
import java.util.prefs.Preferences

// どこからでも話しかけるキー(#2201・親 #1762)。他のアプリが前にあっても受けるキー。
// 既定は off。キー・押し方の決定(#1762)までは `TAILORIT_HOTKEY=ctrl+shift+space` か端末の設定でだけ on にする。

/** 修飾キー + キー 1 つ。[vk] は Windows の仮想キーの番号(他の OS の実装もここから引く)。 */
data class HotkeyChord(
    val ctrl: Boolean = false,
    val alt: Boolean = false,
    val shift: Boolean = false,
    /** Windows キー(macOS の ⌘・Linux の Super)。 */
    val meta: Boolean = false,
    val vk: Int,
    val keyName: String,
) {
    /** 表示用の「Ctrl+Shift+Space」。 */
    val label: String
        get() = listOfNotNull("Ctrl".takeIf { ctrl }, "Alt".takeIf { alt }, "Shift".takeIf { shift }, "Win".takeIf { meta }, keyName).joinToString("+")
}

/** キーの文字列を読めなかった理由。 */
enum class HotkeyParseError { EMPTY, NO_KEY, MULTIPLE_KEYS, UNKNOWN_KEY, NO_MODIFIER }

sealed interface HotkeyParse {
    data class Ok(val chord: HotkeyChord) : HotkeyParse
    data class Error(val reason: HotkeyParseError, val token: String? = null) : HotkeyParse
}

/** `ctrl+shift+space` のような文字列を読む。大文字小文字・空白は問わない。 */
object HotkeySpec {
    private const val VK_A = 0x41
    private const val VK_0 = 0x30
    private const val VK_F1 = 0x70

    private val NAMED = mapOf(
        "space" to (0x20 to "Space"),
        "enter" to (0x0D to "Enter"),
        "return" to (0x0D to "Enter"),
        "tab" to (0x09 to "Tab"),
        "esc" to (0x1B to "Esc"),
        "escape" to (0x1B to "Esc"),
        "insert" to (0x2D to "Insert"),
        "home" to (0x24 to "Home"),
        "end" to (0x23 to "End"),
        "pause" to (0x13 to "Pause"),
    )

    fun parse(raw: String?): HotkeyParse {
        val tokens = raw?.split('+')?.map { it.trim().lowercase() }?.filter { it.isNotEmpty() }.orEmpty()
        if (tokens.isEmpty()) return HotkeyParse.Error(HotkeyParseError.EMPTY)
        var ctrl = false
        var alt = false
        var shift = false
        var meta = false
        val keys = mutableListOf<Pair<Int, String>>()
        for (t in tokens) {
            when (t) {
                "ctrl", "control" -> ctrl = true
                "alt", "option" -> alt = true
                "shift" -> shift = true
                "win", "meta", "super", "cmd", "command" -> meta = true
                else -> keys += key(t) ?: return HotkeyParse.Error(HotkeyParseError.UNKNOWN_KEY, t)
            }
        }
        if (keys.isEmpty()) return HotkeyParse.Error(HotkeyParseError.NO_KEY)
        if (keys.size > 1) return HotkeyParse.Error(HotkeyParseError.MULTIPLE_KEYS)
        val (vk, name) = keys.single()
        val isFunctionKey = vk in VK_F1 until VK_F1 + 24
        // 修飾キー無しの文字は普段の入力を奪うので受けない(F キーだけは単独でもよい)
        if (!ctrl && !alt && !shift && !meta && !isFunctionKey) return HotkeyParse.Error(HotkeyParseError.NO_MODIFIER)
        return HotkeyParse.Ok(HotkeyChord(ctrl, alt, shift, meta, vk, name))
    }

    private fun key(t: String): Pair<Int, String>? {
        NAMED[t]?.let { return it }
        if (t.length == 1) {
            val c = t[0]
            if (c in 'a'..'z') return (VK_A + (c - 'a')) to c.uppercase()
            if (c in '0'..'9') return (VK_0 + (c - '0')) to c.toString()
        }
        if (t.startsWith("f")) {
            val n = t.drop(1).toIntOrNull()
            if (n != null && n in 1..24) return (VK_F1 + n - 1) to "F$n"
        }
        return null
    }
}

/** 登録できなかった理由。 */
enum class HotkeyFailure {
    /** 他のアプリが同じ組みを使っている。 */
    IN_USE,

    /** この OS ではまだ受けられない。 */
    UNSUPPORTED,

    /** その他(OS の呼び出しの失敗・時間切れ)。 */
    ERROR,
}

sealed interface HotkeyRegistration {
    data class Registered(val chord: HotkeyChord) : HotkeyRegistration
    data class Failed(val reason: HotkeyFailure, val detail: String? = null) : HotkeyRegistration
}

/** OS 全体のキーの口。1 つの実体が 1 つの組みを持つ。 */
interface GlobalHotkey : AutoCloseable {
    /** [chord] を登録する。前の組みがあれば外してから。押されたら [onPressed](キーを受けるスレッドで呼ぶ)。 */
    fun register(chord: HotkeyChord, onPressed: () -> Unit): HotkeyRegistration

    /** 外す。登録していなければ何もしない。 */
    fun unregister()

    override fun close() = unregister()
}

/** 受けられない OS(今は Windows・Linux の X11 以外)。登録はいつも [HotkeyFailure.UNSUPPORTED]。 */
object UnsupportedGlobalHotkey : GlobalHotkey {
    override fun register(chord: HotkeyChord, onPressed: () -> Unit) = HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED)
    override fun unregister() = Unit
}

/** 文字の設定 1 つ。 */
interface StringSetting {
    fun get(): String?
}

/** java.util.prefs(Windows はレジストリ、Linux は ~/.java)。ウィンドウの大きさ・トレイの設定と同じ場所。 */
class PrefsStringSetting(private val key: String) : StringSetting {
    override fun get(): String? = runCatching { Preferences.userRoot().node("jp/kz2601/tailorit/desktop").get(key, null) }.getOrNull()
}

/** どのキーで受けるか。null = off(既定)。 */
object HotkeySettings {
    /** 端末の設定の鍵。値は `ctrl+shift+space` の形。空・`off` なら使わない。 */
    const val PREF_KEY = "globalHotkey"

    /**
     * `TAILORIT_HOTKEY`(`-Dtailorit.hotkey`)が先、無ければ端末の設定。どちらも無ければ off(null)。
     * 値が `off`・`0`・`false` なら off。読むのは [HotkeySpec.parse](読めない値は知らせるので、ここでは捨てない)。
     */
    fun raw(
        props: (String) -> String? = System::getProperty,
        env: (String) -> String? = System::getenv,
        setting: StringSetting = PrefsStringSetting(PREF_KEY),
    ): String? {
        val raw = (props("tailorit.hotkey") ?: env("TAILORIT_HOTKEY") ?: setting.get())?.trim()
        return raw?.takeUnless { it.isEmpty() || it.lowercase() in OFF }
    }

    private val OFF = setOf("off", "0", "false", "no", "none")
}

object GlobalHotkeys {
    /** OS ごとの実体。Windows は RegisterHotKey、Linux の X11 は XGrabKey(#2202)、他はまだ受けない。 */
    fun forOs(
        osName: String = System.getProperty("os.name").orEmpty(),
        env: (String) -> String? = System::getenv,
    ): GlobalHotkey = when {
        osName.startsWith("Windows", ignoreCase = true) -> WindowsGlobalHotkey()
        osName.startsWith("Linux", ignoreCase = true) && LinuxDisplay.detect(env) == LinuxDisplay.X11 -> X11GlobalHotkey()
        else -> UnsupportedGlobalHotkey
    }

    /**
     * 設定があれば登録する(off なら何もせず null)。登録は OS を待つので画面のスレッドで呼ばない。
     * 押されたら [post] で画面のスレッドへ移り、[requests] に渡す(音声入力が off なら「使えない」を通知)。
     * 読めない・他のアプリと重なる時は [notify] で知らせる。戻り値は終わる時に閉じる物。
     */
    fun startFromSettings(
        raw: String? = HotkeySettings.raw(),
        hotkey: () -> GlobalHotkey = { forOs() },
        voiceEnabled: () -> Boolean,
        bringToFront: () -> Unit,
        notify: (title: String, body: String) -> Unit,
        post: (Runnable) -> Unit,
        requests: HotkeyVoiceRequests = HotkeyVoiceRequests.shared,
        log: (String) -> Unit = { System.err.println(it) },
    ): GlobalHotkey? {
        raw ?: return null
        val parse = HotkeySpec.parse(raw)
        val chord = (parse as? HotkeyParse.Ok)?.chord
        val h = chord?.let { hotkey() }
        val registration = h?.register(chord) {
            post { requests.press(voiceEnabled(), notify, bringToFront) }
        }
        HotkeyMessages.of(raw, parse, registration)?.let { notify(Strings["desktop_hotkey_title"], it) }
        if (registration is HotkeyRegistration.Failed) log("[TailorIt] global hotkey: ${registration.reason} ${registration.detail.orEmpty()}")
        return h.takeIf { registration is HotkeyRegistration.Registered }
    }
}
