package jp.kz2601.metamon.feedback

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.runtime.RuntimeState
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.voice.UndoAction
import jp.kz2601.metamon.voice.VoiceErrorCopy
import jp.kz2601.metamon.voice.VoicePhase
import jp.kz2601.metamon.voice.VoiceUiState
import jp.kz2601.metamon.voice.bodyText
import kotlinx.coroutines.flow.StateFlow

/**
 * 画面に出したのと同時に TalkBack に読ませる文(H8・iOS の `VoiceOverAnnouncer` を呼ぶ所と同じ)。
 * 前の状態と今の状態を比べ、新しく出たものだけを返す。消えた時・同じものの置き直しでは読まない。
 */
object TalkBackAnnouncements {
    /** 音声の画面: AI の質問・返事、完成の要約(無ければ「できました」)、元に戻した・やり直した、状態カード */
    fun voice(old: VoiceUiState?, new: VoiceUiState, english: Boolean = false): List<UiText> {
        if (old == null) return emptyList() // 開いた時点で出ているものは読まない
        val out = mutableListOf<UiText>()
        // AI の質問・返事(吹き出しが 1 つ増えた)
        val reply = new.chat.lastOrNull()
        if (new.chat.size > old.chat.size && reply != null && !reply.fromUser) out += UiText.Raw(reply.text)
        // 完成(生成が終わった。開き直し・ホットリロードの版の変化では読まない)
        if (new.phase == VoicePhase.DONE && old.phase != VoicePhase.DONE) out += completed(old, new)
        // 元に戻した・やり直した(続けて戻した時は版で見分ける)
        val undone = new.undone
        if (undone != null && (undone != old.undone || old.document.version != new.document.version)) {
            out += UiText.Res(if (undone == UndoAction.UNDO) R.string.feedback_undone else R.string.feedback_redone)
        }
        // 状態カード
        val error = new.error
        if (error != null && error != old.error) {
            val copy = VoiceErrorCopy.of(error)
            out += UiText.Res(R.string.a11y_error_card, UiText.Res(copy.title), copy.bodyText(english))
        }
        return out.filter { it !is UiText.Raw || it.text.isNotBlank() }
    }

    /** 生成したアプリの上: トースト、上部のバナー(ホットリロード #1108)、「削除しました・元に戻す」 */
    fun runtime(old: RuntimeState?, new: RuntimeState): List<UiText> {
        if (old == null) return emptyList()
        val out = mutableListOf<UiText>()
        val toast = new.toast
        if (toast != null && toast != old.toast) out += toast
        val banner = new.banner
        if (banner != null && banner.id != old.banner?.id) out += banner.text
        val undo = new.recordUndo
        if (undo != null && undo.id != old.recordUndo?.id) out += UiText.Res(R.string.feedback_deleted)
        return out
    }

    /** 完成の知らせ。要約があれば要約、無ければ「「名前」ができました」/「変更しました」(iOS の `completedAnnouncement`) */
    private fun completed(old: VoiceUiState, new: VoiceUiState): UiText {
        new.summary?.trim()?.takeIf { it.isNotEmpty() }?.let { return UiText.Raw(it) }
        val name = new.document.dsl.app.name.trim()
        return if (old.isInitial && !new.isInitial && name.isNotEmpty()) {
            UiText.Res(R.string.a11y_completed_created, name)
        } else {
            UiText.Res(R.string.a11y_completed_changed)
        }
    }
}

/** 音声の画面とランタイムの状態を見て、新しく出た知らせを TalkBack に読ませる。読む文はログに出さない。 */
@Composable
fun TalkBackAnnouncer(voice: StateFlow<VoiceUiState>, runtime: StateFlow<RuntimeState>) {
    val resources = LocalContext.current.resources
    val english by rememberUpdatedState(LocalConfiguration.current.locales[0].language == "en")
    val announce = rememberAnnouncer()
    LaunchedEffect(voice, announce) {
        var old: VoiceUiState? = voice.value
        voice.collect { new ->
            TalkBackAnnouncements.voice(old, new, english).forEach { announce(it.resolve(resources)) }
            old = new
        }
    }
    LaunchedEffect(runtime, announce) {
        var old: RuntimeState? = runtime.value
        runtime.collect { new ->
            TalkBackAnnouncements.runtime(old, new).forEach { announce(it.resolve(resources)) }
            old = new
        }
    }
}
