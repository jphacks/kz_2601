package jp.kz2601.metamon.desktop.os

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.ComponentType
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.io.File
import java.util.concurrent.TimeUnit

/**
 * Windows で通知がオフの時の「通知がオフ」の帯(iOS の `NotificationsOffBar` の代わり)。
 * 出すのは、通知を使うアプリで `HKCU\…\PushNotifications` の `ToastEnabled` が 0 の時だけ。閉じたらそのアプリでは出さない(起動中)。
 */
object NotificationsOff {
    const val KEY = """HKCU\Software\Microsoft\Windows\CurrentVersion\PushNotifications"""
    const val SETTINGS_URI = "ms-settings:notifications"

    /** 閉じたアプリ(appId)。起動中だけ覚える。 */
    private val dismissed = mutableSetOf<String>()

    /** 通知を使うアプリか: 宣言した通知・scheduleNotification・カウントダウンのタイマー(終わりにトレイで知らせる)。 */
    fun usesNotifications(dsl: AppDSL): Boolean {
        if (dsl.app.notifications.isNotEmpty()) return true
        fun walk(c: Component): Sequence<Component> = sequenceOf(c) + c.children.asSequence().flatMap(::walk)
        val components = dsl.screens.asSequence().flatMap { it.components.asSequence().flatMap(::walk) }
        val actions = dsl.screens.asSequence().flatMap { it.toolbar.asSequence().map { b -> b.action } } +
            components.flatMap { sequenceOf(it.action, it.onItemTap, it.onDateTap, it.threshold?.action).filterNotNull() }
        return actions.any { it.type == ActionType.SCHEDULE_NOTIFICATION } ||
            components.any { it.type == ComponentType.TIMER_DISPLAY && it.mode == ComponentMode.COUNTDOWN }
    }

    /** `reg query … /v ToastEnabled` の出力。`ToastEnabled    REG_DWORD    0x0` は false。値が無い・読めない時は null(既定はオン)。 */
    fun parseToastEnabled(out: String): Boolean? =
        Regex("""ToastEnabled\s+REG_DWORD\s+0x([0-9a-fA-F]+)""").find(out)?.groupValues?.get(1)?.toLongOrNull(16)?.let { it != 0L }

    /** 帯を出すか。オフと分かった時だけ(読めない・Windows 以外は出さない)。 */
    fun isVisible(toastEnabled: Boolean?, usesNotifications: Boolean, dismissed: Boolean): Boolean =
        toastEnabled == false && usesNotifications && !dismissed

    fun isDismissed(appId: String): Boolean = synchronized(dismissed) { appId in dismissed }

    fun dismiss(appId: String) {
        synchronized(dismissed) { dismissed += appId }
    }

    /** テスト用。 */
    internal fun resetDismissed() = synchronized(dismissed) { dismissed.clear() }

    /** Windows の時だけレジストリを読む。ほかの OS は null。 */
    fun read(os: String = System.getProperty("os.name").orEmpty()): Boolean? {
        if (!os.startsWith("Windows", ignoreCase = true)) return null
        return run(systemExe("reg.exe"), "query", KEY, "/v", "ToastEnabled")?.let(::parseToastEnabled)
    }

    /** 設定アプリの「通知」を開く。 */
    fun openSettings() {
        runCatching { ProcessBuilder(systemExe("explorer.exe", inSystem32 = false), SETTINGS_URI).start() }
    }

    private fun systemExe(name: String, inSystem32: Boolean = true): String {
        val root = System.getenv("SystemRoot")?.takeIf { it.isNotBlank() } ?: "C:\\Windows"
        return File(if (inSystem32) File(root, "System32") else File(root), name).path
    }

    private fun run(vararg cmd: String): String? = runCatching {
        val p = ProcessBuilder(*cmd).redirectErrorStream(true).start()
        if (!p.waitFor(2, TimeUnit.SECONDS)) {
            p.destroyForcibly()
            return null
        }
        val out = p.inputStream.bufferedReader().readText()
        if (p.exitValue() == 0) out else null
    }.getOrNull()
}

/** 設定を変えて戻ってきた時に消えるよう読み直す間隔。 */
private const val POLL_MS = 10_000L

/** ウィンドウの上に出す帯: 文言 [設定を開く] [閉じる]。通知を使わないアプリでは何もしない。 */
@Composable
fun NotificationsOffBar(appId: String, dsl: AppDSL) {
    val uses = remember(dsl) { NotificationsOff.usesNotifications(dsl) }
    if (!uses) return
    var enabled by remember { mutableStateOf<Boolean?>(null) }
    var dismissed by remember(appId) { mutableStateOf(NotificationsOff.isDismissed(appId)) }
    if (!dismissed) {
        LaunchedEffect(appId) {
            while (true) {
                enabled = withContext(Dispatchers.IO) { NotificationsOff.read() }
                delay(POLL_MS)
            }
        }
    }
    if (!NotificationsOff.isVisible(enabled, uses, dismissed)) return
    Surface(tonalElevation = 2.dp) {
        Row(Modifier.fillMaxWidth().padding(start = 12.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(
                Strings["permission_off_notifications"],
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
            )
            TextButton(onClick = NotificationsOff::openSettings) { Text(Strings["permission_open_settings"]) }
            TextButton(onClick = {
                NotificationsOff.dismiss(appId)
                dismissed = true
            }) { Text(Strings["desktop_close"]) }
        }
    }
    HorizontalDivider()
}
