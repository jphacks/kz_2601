package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.http.FailureKind

/**
 * 「話す → 文字 → 生成 → 変身」の段階(#359)。
 * IDLE → LISTENING → TRANSCRIBING → SENDING(送った・最初の途中経過の前)→ GENERATING(途中経過が届いている)→ DONE / ERROR。
 * 相談(chat)の往復も SENDING を通り、返事が来たら IDLE に戻る。DONE / ERROR からはそのまま話し直せる。
 */
enum class VoicePhase {
    IDLE,
    LISTENING,
    TRANSCRIBING,
    SENDING,
    GENERATING,
    DONE,
    ERROR,
    ;

    /** マイクで話し始められる */
    val canStart: Boolean get() = this == IDLE || this == DONE || this == ERROR
}

/** 入力の手段(iOS の `ClarifyFlow.InputSource`)。声だけで進めた create は ready で自動で作る(#160)。 */
enum class InputSource { VOICE, TEXT }

/** 対話の吹き出し 1 つ。 */
data class ChatBubble(val fromUser: Boolean, val text: String, val kind: ChatTurn.Kind? = null)

/** 生成の途中経過(#636)。どれも仮置き。 */
data class BuildProgress(
    val appName: String? = null,
    val tabs: List<String> = emptyList(),
    val components: List<StreamedComponent> = emptyList(),
)

/** 状態カード(#60・H9)に出す失敗。文言は [VoiceErrorCopy]。 */
sealed interface VoiceError {
    /** 録音・文字起こしの失敗 */
    data class Voice(val failure: TranscriberFailure) : VoiceError

    /** ai への依頼の失敗 */
    data class Request(val kind: FailureKind, val origin: Origin) : VoiceError

    enum class Origin { CHAT, GENERATE }
}

data class VoiceUiState(
    val document: AppDocument,
    val phase: VoicePhase = VoicePhase.IDLE,
    /** 録音中の途中の聞き取り */
    val partial: String = "",
    /** 送った(送る)依頼文。生成中の表示に使う */
    val transcript: String = "",
    val progress: BuildProgress? = null,
    /** 直近の生成の一文要約(無ければ null) */
    val summary: String? = null,
    val error: VoiceError? = null,
    val chat: List<ChatBubble> = emptyList(),
    val chatTurn: ChatTurn? = null,
    /** 相談の返事を待っている(SENDING のうち chat の分) */
    val chatPending: Boolean = false,
    /** 声だけで ready まで来て、少し待ってから自動で作るところ */
    val autoBuildPending: Boolean = false,
    /** 生成後のアプリの上で音声のシートを開いているか */
    val voiceOpen: Boolean = false,
    /** 声で入力できる見込みがあるか(false なら文字入力を勧める) */
    val voiceAvailable: Boolean = true,
) {
    /** 初期状態(音声画面だけ)= 新規作成 */
    val isInitial: Boolean get() = document.dsl.isVoiceOnly
    val isChatting: Boolean get() = chat.isNotEmpty()

    /** 生成の依頼中(相談の往復は含まない) */
    val isGenerating: Boolean get() = (phase == VoicePhase.SENDING && !chatPending) || phase == VoicePhase.GENERATING

    val isBusy: Boolean get() = phase == VoicePhase.TRANSCRIBING || phase == VoicePhase.SENDING || phase == VoicePhase.GENERATING
}
