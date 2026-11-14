package jp.kz2601.metamon.desktop.hotkey

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import jp.kz2601.metamon.desktop.i18n.Strings

/** キーで呼び出せない理由(#2203)。設定に出す。他のアプリと重なる・読めないは [HotkeyMessages] の知らせ。 */
enum class HotkeyUnavailable(val key: String) {
    /** Wayland でポータルの GlobalShortcuts が無い。 */
    WAYLAND("desktop_hotkey_unavailable_wayland"),

    /** 画面が無い(ssh・ヘッドレス)。 */
    NO_DISPLAY("desktop_hotkey_unavailable_no_display"),

    /** この OS ではまだ受けない。 */
    OS("desktop_hotkey_unavailable_os"),

    /** ポータルの確認で人が取り消した。 */
    CANCELLED("desktop_hotkey_unavailable_cancelled"),
    ;

    val text: String get() = Strings[key]
}

object HotkeyAvailability {
    /** 登録の結果から理由を選ぶ。登録できた・まだ試していない・重なる / その他の失敗は null。 */
    fun reason(
        registration: HotkeyRegistration?,
        osName: String = System.getProperty("os.name").orEmpty(),
        env: (String) -> String? = System::getenv,
    ): HotkeyUnavailable? {
        val failed = registration as? HotkeyRegistration.Failed ?: return null
        return when (failed.reason) {
            HotkeyFailure.UNSUPPORTED -> when {
                !osName.startsWith("Linux", ignoreCase = true) -> HotkeyUnavailable.OS
                else -> when (LinuxDisplay.detect(env)) {
                    LinuxDisplay.WAYLAND -> HotkeyUnavailable.WAYLAND
                    LinuxDisplay.NONE -> HotkeyUnavailable.NO_DISPLAY
                    LinuxDisplay.X11 -> HotkeyUnavailable.OS
                }
            }
            HotkeyFailure.ERROR -> HotkeyUnavailable.CANCELLED.takeIf { failed.detail == PortalGlobalHotkey.CANCELLED }
            HotkeyFailure.IN_USE -> null
        }
    }
}

/** 最後の登録の結果(設定の画面が読む)。 */
class HotkeyStatus {
    var last by mutableStateOf<HotkeyRegistration?>(null)

    companion object {
        val shared = HotkeyStatus()
    }
}

/** 設定の中に出す 1 行: キーで呼び出せない理由と代わりの案。呼び出せる時は何も出さない。 */
@Composable
fun HotkeyUnavailableNote(status: HotkeyStatus = HotkeyStatus.shared, modifier: Modifier = Modifier) {
    val reason = HotkeyAvailability.reason(status.last) ?: return
    Text(reason.text, modifier = modifier, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
}
