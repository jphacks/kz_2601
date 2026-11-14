package jp.kz2601.metamon.renderer.components.ai

import androidx.compose.runtime.staticCompositionLocalOf
import kotlinx.coroutines.flow.StateFlow

/**
 * aiChat の話して入力と読み上げ(iOS の `DictationHost` と `LeafSpeech`)。声は端末の外へ送らない(端末内の認識・端末の声)。
 * :renderer は :services を知らないので :app が差し込む([LocalAiChatVoice])。無ければマイクとスピーカーを出さない。
 */
interface AiChatVoice {
    /** 話して入力の準備の状態。null は使えない(マイクを出さない) */
    val readiness: StateFlow<DictationReadiness?>

    /** マイク FAB の音声入力が動いているか(その間は録音しない) */
    val voiceInputActive: StateFlow<Boolean>

    /** 録音を始める(読み上げは止める)。始められなければ投げる */
    suspend fun startDictation()

    /** 止めて文字にする。聞き取れなければ投げる */
    suspend fun finishDictation(): String

    /** 録音を捨てる */
    fun cancelDictation()

    /** 音声モデルを用意する */
    fun prepareDictation()

    /** 読み上げ中の持ち主(吹き出しの `owner/turnId`)。読んでいなければ null */
    val speakingOwner: StateFlow<String?>

    fun speak(text: String, owner: String)

    fun stop(owner: String)
}

/** aiChat の声の口。既定は無し(プレビュー・テスト・未対応のホスト)。 */
val LocalAiChatVoice = staticCompositionLocalOf<AiChatVoice?> { null }
