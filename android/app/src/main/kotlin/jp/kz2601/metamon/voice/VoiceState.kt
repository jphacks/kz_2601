package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.purchase.PaidFeatureCheck

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
    /** 録音中の声の大きさ 0..1(#1014・マイクの周りの輪) */
    val inputLevel: Float = 0f,
    /** 認識器が無音で自分から止まった時の聞き取り。送る前に確かめる(#1014・null = 確認なし) */
    val pendingVoice: String? = null,
    /** 送った(送る)依頼文。生成中の表示に使う */
    val transcript: String = "",
    val progress: BuildProgress? = null,
    /** 直近の生成の一文要約(無ければ null) */
    val summary: String? = null,
    /** 要約のバナーに添える「元に戻す」/「やり直す」(#895) */
    val summaryAction: UndoAction? = null,
    /** 直前にやったのが undo / redo(バナーの「元に戻しました」/「やり直しました」) */
    val undone: UndoAction? = null,
    /** 「元に戻す」を押せる(作ったアプリで、サーバーに前の版があるか端末の履歴がある) */
    val canUndo: Boolean = false,
    /** 戻した直後だけ true。新しい版が当たったら false */
    val canRedo: Boolean = false,
    /** 戻している・やり直している最中(ボタンを押せなくする) */
    val undoing: Boolean = false,
    /** undo / redo の結果のトースト */
    val notice: UndoNotice? = null,
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
    /** このアプリの閉じた会話と反映の区切り(#210)。今の会話([chat])は含まない */
    val history: List<ChatHistoryEntry> = emptyList(),
    /** 「最初から」で、これまでの会話も消すかを確かめている */
    val restartConfirm: Boolean = false,
    /** 初期画面に来た経路(挨拶の出し分け・#892) */
    val homeEntry: Greeting.HomeEntry = Greeting.HomeEntry.LAUNCH,
    /** ai の「今日の残り」(#630)。分からなければ null */
    val remainingToday: RemainingToday? = null,
    val isPro: Boolean = false,
    /** 最後に読んだ `GET /v1/me/usage`(設定の「使っている量」もこれを読む・#317)。分からなければ null */
    val myUsage: MyUsage? = null,
    /** 入力欄に入れる文(設定の「頼み方のコツ」で選んだ例・#1161)。入れたら [VoiceViewModel.consumeDraft] で消す */
    val draft: String? = null,
    /** 無料枠切れ・402 で作らずに止めた新規作成(#1261)。会話の下にカードを出す */
    val heldBuild: HeldBuild? = null,
) {
    /** 初期状態(音声画面だけ)= 新規作成 */
    val isInitial: Boolean get() = document.dsl.isVoiceOnly
    val isChatting: Boolean get() = chat.isNotEmpty()

    /** 今の相談の種類。初期画面 = 新しく作る、作ったアプリの上 = 変える */
    val chatMode: ChatMode get() = if (isInitial) ChatMode.CREATE else ChatMode.EDIT

    /** 吹き出しの画面を出すか。今の会話が無くても、閉じた会話があれば見せる */
    val showsChat: Boolean get() = isChatting || history.isNotEmpty()

    /** 生成の依頼中(相談の往復は含まない) */
    val isGenerating: Boolean get() = (phase == VoicePhase.SENDING && !chatPending) || phase == VoicePhase.GENERATING

    val isBusy: Boolean get() = phase == VoicePhase.TRANSCRIBING || phase == VoicePhase.SENDING || phase == VoicePhase.GENERATING
}

/**
 * 作らずに止めた新規作成(#1261・iOS の `HeldBuild` / `PaidConfirmation`)。会話は消さずに残し、同じ依頼で続きから作る。
 * [text] は作るはずだった依頼文(ready の transcript / 案の instruction)。
 */
sealed interface HeldBuild {
    val text: String

    /** 今日の無料作成を使い切った(課金画面を閉じた)。「続きから作る」 */
    data class LimitReached(override val text: String, val dailyFreeLimit: Int) : HeldBuild

    /**
     * サーバーが 402 `pro_required` で止めた(課金画面を閉じた)か、相談の「これでいく」の案が有料の機能を含む(#1175・iOS の `PaidConfirmation`)。
     * 「Pro にする」/「この機能なしで作る」。[free] はサーバーが返した無料版の依頼文(freeInstruction / freeTranscript)。
     */
    data class ProRequired(override val text: String, val features: List<PaidFeature>, val free: String? = null) : HeldBuild {
        /** 有料の機能を外して作る依頼文(サーバーが返さなければ有料の機能を抜く指示を足す) */
        val freeText: String get() = free?.trim()?.ifEmpty { null } ?: (text + "\n" + PaidFeatureCheck.FALLBACK_FREE_SUFFIX)
    }
}
