package jp.kz2601.metamon.renderer.components.audio

import androidx.compose.runtime.staticCompositionLocalOf
import kotlinx.coroutines.flow.StateFlow

/** 話して入力の準備の具合(iOS の `DictationReadiness`)。 */
enum class DictationReadiness { READY, PREPARING, NEEDS_MODEL, UNAVAILABLE }

/** 話して入力の失敗の種類(文言は :renderer の strings_audio.xml)。 */
enum class DictationFailure { MIC_DENIED, NOT_READY, NOT_HEARD, UNAVAILABLE, FAILED }

class DictationException(val failure: DictationFailure) : Exception(failure.name)

/**
 * `transcribeField` が声を文字にする口(iOS の `DictationHost`)。:app が本体の音声入力と同じ認識器で実装する。
 * 声は端末の外へ出さず、録音も残さない。
 */
interface DictationHost {
    val readiness: StateFlow<DictationReadiness>

    /** マイク FAB の音声入力のシートが出ている間 true(その間は録音を捨てる)。 */
    val isVoiceInputActive: StateFlow<Boolean>

    /** 聞いている間 true。認識器が無音で自分から止まったら false に戻る(部品はそれを見て [finishDictation])。 */
    val listening: StateFlow<Boolean>

    /** 言語のモデルの有無を確かめる・無ければ取りに行く。 */
    fun prepareDictation()

    /** 失敗は [DictationException]。 */
    suspend fun startDictation()

    /** 失敗は [DictationException]。 */
    suspend fun finishDictation(): String

    fun cancelDictation()
}

/** マイクの権限を聞く口。:app が権限の窓口(PermissionCenter)で差す。無ければ OS の今の状態だけを見る。 */
fun interface MicrophoneAccess {
    fun request(onResult: (granted: Boolean) -> Unit)
}

val LocalMicrophoneAccess = staticCompositionLocalOf<MicrophoneAccess?> { null }
