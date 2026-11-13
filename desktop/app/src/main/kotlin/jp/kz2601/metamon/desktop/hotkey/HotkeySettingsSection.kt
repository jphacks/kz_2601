package jp.kz2601.metamon.desktop.hotkey

import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
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
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.isAltPressed
import androidx.compose.ui.input.key.isCtrlPressed
import androidx.compose.ui.input.key.isMetaPressed
import androidx.compose.ui.input.key.isShiftPressed
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import java.util.prefs.Preferences

// 設定の「どこからでも話しかけるキー」(#2204): on / off・今のキー・押して変える・既定に戻す・登録できない理由。

/** 書ける文字の設定 1 つ。 */
interface MutableStringSetting : StringSetting {
    fun set(value: String)
}

/** java.util.prefs に書く([PrefsStringSetting] と同じ場所)。 */
class PrefsMutableStringSetting(private val key: String) : MutableStringSetting {
    private val node get() = Preferences.userRoot().node("jp/kz2601/tailorit/desktop")

    override fun get(): String? = runCatching { node.get(key, null) }.getOrNull()

    override fun set(value: String) {
        runCatching {
            node.put(key, value)
            node.flush()
        }
    }
}

/** 押したキー(AWT のキーの番号 + 修飾キー)→ `ctrl+shift+space` の形。 */
object HotkeyCapture {
    /** 修飾キーだけ(まだ押している途中)。 */
    private val MODIFIER_ONLY = setOf(0x10, 0x11, 0x12, 0x9D, 0x020C, 0x14, 0x90)

    private val NAMED = mapOf(
        0x20 to "space",
        0x0A to "enter",
        0x09 to "tab",
        0x1B to "esc",
        0x9B to "insert",
        0x24 to "home",
        0x23 to "end",
        0x13 to "pause",
    )

    /** 読めた形。修飾キーだけなら null(続きを待つ)。知らないキーは `?` を返し、[HotkeySpec.parse] で UNKNOWN_KEY にする。 */
    fun spec(awtKeyCode: Int, ctrl: Boolean, alt: Boolean, shift: Boolean, meta: Boolean): String? {
        if (awtKeyCode in MODIFIER_ONLY) return null
        val key = NAMED[awtKeyCode]
            ?: when (awtKeyCode) {
                in 0x41..0x5A -> ('a' + (awtKeyCode - 0x41)).toString()
                in 0x30..0x39 -> ('0' + (awtKeyCode - 0x30)).toString()
                in 0x70..0x7B -> "f${awtKeyCode - 0x70 + 1}"
                in 0xF000..0xF00B -> "f${awtKeyCode - 0xF000 + 13}"
                else -> "?"
            }
        return (listOfNotNull("ctrl".takeIf { ctrl }, "alt".takeIf { alt }, "shift".takeIf { shift }, "win".takeIf { meta }) + key).joinToString("+")
    }
}

/** 設定の画面に出す今の状態。 */
data class HotkeySettingsState(val enabled: Boolean, val chord: HotkeyChord?)

/**
 * 話しかけるキーの設定の読み書き。[setting] は今使う値(空・`off` は off。#2201 の [HotkeySettings] と同じ鍵)、
 * [last] は off にした後に on へ戻す時の前のキー。
 */
class HotkeySettingsModel(
    private val setting: MutableStringSetting = PrefsMutableStringSetting(HotkeySettings.PREF_KEY),
    private val last: MutableStringSetting = PrefsMutableStringSetting(LAST_KEY),
) {
    fun state(): HotkeySettingsState {
        val raw = HotkeySettings.raw(props = { null }, env = { null }, setting = setting)
        val key = raw ?: last.get() ?: DEFAULT
        return HotkeySettingsState(enabled = raw != null, chord = (HotkeySpec.parse(key) as? HotkeyParse.Ok)?.chord)
    }

    fun setEnabled(on: Boolean) {
        if (on) setting.set(last.get()?.takeIf { HotkeySpec.parse(it) is HotkeyParse.Ok } ?: DEFAULT) else setting.set(OFF)
    }

    /** 押したキーに変える。読めない時は保存せずに理由を返す。変えたら on にする。 */
    fun change(spec: String): HotkeyParse {
        val parse = HotkeySpec.parse(spec)
        if (parse is HotkeyParse.Ok) {
            setting.set(spec)
            last.set(spec)
        }
        return parse
    }

    fun reset() {
        setting.set(DEFAULT)
        last.set(DEFAULT)
    }

    companion object {
        /** 既定のキー(#1762 で決めるまでの仮)。 */
        const val DEFAULT = "ctrl+shift+space"
        const val LAST_KEY = "globalHotkeyLast"
        private const val OFF = "off"
    }
}

/** 設定の文言の選び方。 */
object HotkeySettingsText {
    /** 押したキーを読めなかった時。 */
    fun captureError(e: HotkeyParse.Error): String = when (e.reason) {
        HotkeyParseError.NO_MODIFIER -> Strings["desktop_hotkey_need_modifier"]
        else -> Strings["desktop_hotkey_unknown_key"]
    }

    /** 登録できなかった理由(他のアプリが使っている / この環境では使えない など)。登録できた・off なら null。 */
    fun failure(
        state: HotkeySettingsState,
        registration: HotkeyRegistration?,
        osName: String = System.getProperty("os.name").orEmpty(),
        env: (String) -> String? = System::getenv,
    ): String? {
        if (!state.enabled) return null
        val chord = state.chord ?: return null
        val failed = registration as? HotkeyRegistration.Failed ?: return null
        HotkeyAvailability.reason(failed, osName, env)?.let { return it.text }
        return when (failed.reason) {
            HotkeyFailure.IN_USE -> Strings.format("desktop_hotkey_in_use", chord.label)
            HotkeyFailure.ERROR -> Strings.format("desktop_hotkey_failed", chord.label)
            HotkeyFailure.UNSUPPORTED -> null
        }
    }
}

/** 今のキーを登録し直す口(Main が入れる)。設定を変えたら呼ぶ。 */
class HotkeyRunner(private val start: (raw: String?) -> GlobalHotkey?) : AutoCloseable {
    private var current: GlobalHotkey? = null

    @Synchronized
    fun restart(raw: String?) {
        current?.close()
        current = null
        current = start(raw)
    }

    @Synchronized
    override fun close() {
        current?.close()
        current = null
    }

    companion object {
        @Volatile var shared: HotkeyRunner? = null

        /** 設定を変えた後: 裏のスレッドで今の設定を登録し直す(OS を待つので画面のスレッドで呼ばない)。 */
        fun restartShared() {
            val r = shared ?: return
            Thread({ r.restart(HotkeySettings.raw()) }, "global-hotkey-restart").apply { isDaemon = true }.start()
        }
    }
}

/**
 * 設定の「どこからでも話しかけるキー」の節。[onChanged] は保存の後に呼ぶ(登録し直す。画面のスレッドで重い事をしない)。
 */
@Composable
fun HotkeySettingsSection(
    model: HotkeySettingsModel = remember { HotkeySettingsModel() },
    status: HotkeyStatus = HotkeyStatus.shared,
    onChanged: () -> Unit = HotkeyRunner::restartShared,
    modifier: Modifier = Modifier,
) {
    var state by remember { mutableStateOf(model.state()) }
    var capturing by remember { mutableStateOf(false) }
    var captureError by remember { mutableStateOf<String?>(null) }
    fun saved() {
        state = model.state()
        onChanged()
    }
    Column(modifier) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp).semantics(mergeDescendants = true) {},
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(Strings["desktop_hotkey_title"], modifier = Modifier.weight(1f))
            Switch(checked = state.enabled, onCheckedChange = {
                model.setEnabled(it)
                saved()
            })
        }
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(Strings["desktop_hotkey_current"], modifier = Modifier.weight(1f))
            Text(state.chord?.label.orEmpty(), color = MaterialTheme.colorScheme.onSurfaceVariant)
            OutlinedButton(onClick = {
                captureError = null
                capturing = true
            }) { Text(Strings["desktop_hotkey_change"]) }
            TextButton(onClick = {
                model.reset()
                captureError = null
                saved()
            }) { Text(Strings["desktop_hotkey_reset"]) }
        }
        if (capturing) {
            val focus = remember { FocusRequester() }
            Text(
                Strings["desktop_hotkey_press_now"],
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp).focusRequester(focus).focusable().onPreviewKeyEvent { e ->
                    if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent true
                    val code = e.key.nativeKeyCode
                    val plain = !e.isCtrlPressed && !e.isAltPressed && !e.isShiftPressed && !e.isMetaPressed
                    if (code == ESC && plain) {
                        capturing = false
                        return@onPreviewKeyEvent true
                    }
                    val spec = HotkeyCapture.spec(code, e.isCtrlPressed, e.isAltPressed, e.isShiftPressed, e.isMetaPressed) ?: return@onPreviewKeyEvent true
                    when (val r = model.change(spec)) {
                        is HotkeyParse.Ok -> {
                            capturing = false
                            captureError = null
                            saved()
                        }
                        is HotkeyParse.Error -> captureError = HotkeySettingsText.captureError(r)
                    }
                    true
                },
            )
            LaunchedEffect(Unit) { focus.requestFocus() }
        }
        val message = captureError ?: HotkeySettingsText.failure(state, status.last)
        message?.let {
            Text(
                it,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
            )
        }
    }
}

private const val ESC = 0x1B
