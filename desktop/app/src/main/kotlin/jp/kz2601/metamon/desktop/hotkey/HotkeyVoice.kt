package jp.kz2601.metamon.desktop.hotkey

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.desktop.ActionContext
import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType

/**
 * どこからでも話しかけるキーと、前にあるウィンドウの openVoice をつなぐ。
 * 押された: 音声入力が off なら「使えない」を通知、on ならウィンドウを前に出して [requests] を増やす。
 * 増えたら [HotkeyVoiceHook] の中で前にあるウィンドウが 1 回だけ openVoice を動かす。
 */
class HotkeyVoiceRequests {
    var requests by mutableIntStateOf(0)
        private set
    private var pending = false

    /** 押された(画面のスレッドで呼ぶ)。openVoice へ渡したら true。 */
    fun press(voiceEnabled: Boolean, notify: (title: String, body: String) -> Unit, bringToFront: () -> Unit): Boolean {
        if (!voiceEnabled) {
            notify(Strings["desktop_hotkey_title"], Strings["desktop_unavailable_voice"])
            return false
        }
        pending = true
        bringToFront()
        requests++
        return true
    }

    /** 前にあるウィンドウが受け取る。1 回の押下で 1 回だけ true。 */
    fun take(isActive: () -> Boolean): Boolean {
        if (!pending || !isActive()) return false
        pending = false
        return true
    }

    companion object {
        val shared = HotkeyVoiceRequests()
    }
}

/** DesktopApp の中で呼ぶ: キーの押下をこのウィンドウの openVoice にする(前にある 1 枚だけ)。 */
@Composable
fun HotkeyVoiceHook(session: DesktopSession, isActive: () -> Boolean, requests: HotkeyVoiceRequests = HotkeyVoiceRequests.shared) {
    LaunchedEffect(requests.requests) {
        if (!requests.take(isActive)) return@LaunchedEffect
        session.dispatch(Action(type = ActionType.OPEN_VOICE), ActionContext(session.top, session.form(session.top)))
    }
}

/** 登録の結果を人に伝える文。登録できた・この OS では受けない時は null(黙る)。 */
object HotkeyMessages {
    fun of(raw: String?, parse: HotkeyParse?, registration: HotkeyRegistration?): String? = when {
        parse is HotkeyParse.Error -> Strings.format("desktop_hotkey_invalid", raw.orEmpty())
        parse !is HotkeyParse.Ok -> null
        registration is HotkeyRegistration.Failed -> when (registration.reason) {
            HotkeyFailure.IN_USE -> Strings.format("desktop_hotkey_in_use", parse.chord.label)
            HotkeyFailure.ERROR -> Strings.format("desktop_hotkey_failed", parse.chord.label)
            HotkeyFailure.UNSUPPORTED -> null
        }
        else -> null
    }
}
