package jp.kz2601.metamon.desktop.chat

import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatPaidFeature
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** 会話の吹き出し 1 つ。[kind] は AI の返事の種類(利用者の発話は null)。 */
data class ChatBubble(val fromUser: Boolean, val text: String, val kind: ChatTurn.Kind? = null)

/** 作る時に `/v1/generate` へ渡す依頼(案・ready から)。有料の機能を含むかは画面が確かめる。 */
data class ChatBuild(
    val instruction: String,
    val requiresPro: Boolean = false,
    val paidFeatures: List<ChatPaidFeature> = emptyList(),
    /** 有料の機能を外した依頼(無ければ null) */
    val freeInstruction: String? = null,
    /** 会話の appId(作る時も同じ id を使う・#2163)。会話の画面が渡す時に入れる。空なら作る側で新しく振る */
    val appId: String = "",
)

/** 会話の状態(画面はこれだけを見て描く)。 */
data class ChatState(
    val bubbles: List<ChatBubble> = emptyList(),
    /** 最後に届いた返事 */
    val turn: ChatTurn? = null,
    /** 複数選択で選んだ物(質問はチップの文字・案は optionId)。押した順 */
    val selected: List<String> = emptyList(),
    /** 返事を待っている */
    val pending: Boolean = false,
    /** 失敗(402 / 429 / 5xx など)。文言は画面の側で決める */
    val error: FailureKind? = null,
) {
    val kind: ChatTurn.Kind? get() = turn?.kind

    /** 質問のチップ。待っている間は出さない */
    val suggestions: List<String> get() = if (pending) emptyList() else turn?.suggestions.orEmpty()

    /** 案のカード。待っている間は出さない */
    val options: List<ChatOption> get() = if (pending) emptyList() else turn?.options.orEmpty()

    val isMultiSelect: Boolean get() = turn?.isMultiSelect == true

    val isReady: Boolean get() = !pending && turn?.kind == ChatTurn.Kind.READY

    val canSend: Boolean get() = !pending
}

/**
 * 作る前の相談(`/v1/chat`)の状態と往復(#2380・#2162 の 1)。画面は持たない(描くのは ChatScreen)。
 * Android の VoiceViewModel の相談の部分と同じ決まり:
 * - 手元の全履歴を毎回送る。1 発目で `turn != 1` ならサーバーに残った古い会話なので消して送り直す
 * - 複数選択のチップは「、」で 1 つの答え、案は「・」の箇条書きで 1 つの依頼にまとめる
 * - 失敗は [FailureKind] に分けて状態に置く(送った文は吹き出しに残し、[retry] で同じ履歴を送り直す)
 */
class ChatSession(
    private val ai: AIClient,
    val appId: String,
    val mode: ChatMode = ChatMode.CREATE,
    private val locale: () -> String? = { null },
) {
    private val _state = MutableStateFlow(ChatState())
    val state: StateFlow<ChatState> = _state.asStateFlow()

    /** 「最初から」で上がる。古い往復の返事を捨てる */
    private var epoch = 0

    /** 文を送る。空・待っている間は送らず false。 */
    suspend fun send(text: String): Boolean {
        val t = text.trim()
        if (t.isEmpty() || _state.value.pending) return false
        _state.update { it.copy(bubbles = it.bubbles + ChatBubble(fromUser = true, text = t), selected = emptyList(), error = null) }
        return round()
    }

    /** 失敗の後の「もう一度」: 吹き出しを足さずに同じ履歴を送り直す。 */
    suspend fun retry(): Boolean {
        val s = _state.value
        if (s.pending || s.error == null || s.bubbles.lastOrNull()?.fromUser != true) return false
        return round()
    }

    /** 複数選択のチップ・案を選ぶ / 外す。単一選択の時は何もしない(押したらすぐ [send] / [choose])。 */
    fun toggle(choice: String) {
        if (!_state.value.isMultiSelect || _state.value.pending) return
        _state.update { s -> s.copy(selected = if (choice in s.selected) s.selected - choice else s.selected + choice) }
    }

    /** 複数選択の質問の「決定」: 選んだチップを「、」でつないで送る。 */
    suspend fun submitSelection(): Boolean {
        val s = _state.value
        if (s.kind == ChatTurn.Kind.PROPOSAL) return false
        val text = joinedAnswer(s.selected.filter { it in s.suggestions })
        return text.isNotEmpty() && send(text)
    }

    /** 案のカードの「これでいく」。作る依頼を返す(作るのは呼んだ側)。 */
    fun choose(option: ChatOption): ChatBuild? {
        if (_state.value.pending) return null
        val text = option.instructionText.trim().ifEmpty { return null }
        return ChatBuild(text, option.requiresPro == true, option.paidFeatures.orEmpty(), option.freeInstruction?.trim()?.ifEmpty { null })
    }

    /** 複数選択の案の「これでいく(N 件)」: 選んだ案(並びは案の順)を 1 つの依頼にする。 */
    fun chooseSelected(): ChatBuild? {
        val s = _state.value
        val picked = s.options.filter { it.optionId in s.selected }
        return combined(picked)?.let(::choose)
    }

    /** ready の「作る」。transcript が無い・ready でない時はこれまでの発話をつなぐ(答えをもらった質問は除く)。 */
    fun build(): ChatBuild? {
        val s = _state.value
        if (s.pending) return null
        val turn = s.turn
        val transcript = turn?.transcript?.trim()?.ifEmpty { null }
        if (turn?.kind == ChatTurn.Kind.READY && transcript != null) {
            return ChatBuild(transcript, turn.requiresPro == true, turn.paidFeatures.orEmpty(), turn.freeTranscript?.trim()?.ifEmpty { null })
        }
        return fallbackTranscript(s.bubbles).ifEmpty { null }?.let { ChatBuild(it) }
    }

    fun dismissError() = _state.update { it.copy(error = null) }

    /** 「最初から」: 手元の会話を消し、サーバーの会話も消す(失敗は握りつぶす)。待っている返事は捨てる。 */
    suspend fun restart() {
        epoch++
        _state.value = ChatState()
        ai.resetChat(appId)
    }

    private suspend fun round(): Boolean {
        val e = epoch
        val messages = _state.value.bubbles.map(::message)
        _state.update { it.copy(pending = true, error = null) }
        return try {
            val turn = if (messages.size == 1) startConversation(messages) else ai.chat(appId, messages, locale(), mode)
            if (e != epoch) return false
            _state.update {
                it.copy(
                    pending = false,
                    turn = turn,
                    selected = emptyList(),
                    bubbles = it.bubbles + ChatBubble(fromUser = false, text = turn.text, kind = turn.kind),
                )
            }
            true
        } catch (c: CancellationException) {
            if (e == epoch) _state.update { it.copy(pending = false) }
            throw c
        } catch (t: Throwable) {
            if (e == epoch) _state.update { it.copy(pending = false, error = FailureKind.classify(t)) }
            false
        }
    }

    private suspend fun startConversation(messages: List<ChatMessage>): ChatTurn {
        val turn = ai.chat(appId, messages, locale(), mode)
        if (turn.turn == 1) return turn
        ai.resetChat(appId)
        return ai.chat(appId, messages, locale(), mode)
    }

    companion object {
        /** 選んだチップを 1 つの答えにする(「、」区切り・押した順)。Android の MultiSelect.joinedAnswer と同じ */
        fun joinedAnswer(chips: List<String>): String = chips.map { it.trim() }.filter { it.isNotEmpty() }.joinToString("、")

        /** 選んだ案の指示をつなぐ。1 件ならそのまま、複数なら「・」の箇条書き(改行区切り) */
        fun joinedInstruction(instructions: List<String>): String {
            val items = instructions.map { it.trim() }.filter { it.isNotEmpty() }
            if (items.size <= 1) return items.firstOrNull().orEmpty()
            return items.joinToString("\n") { "・$it" }
        }

        /** 選んだ案を 1 つにまとめる。有料の機能は和集合、無料版は各案の無料版(無ければ元の指示)。Android の MultiSelect.combined と同じ */
        fun combined(options: List<ChatOption>): ChatOption? {
            val first = options.firstOrNull() ?: return null
            if (options.size == 1) return first
            val pro = options.any { it.requiresPro == true }
            val features = options.flatMap { it.paidFeatures.orEmpty() }.distinctBy(ChatPaidFeature::id)
            val free = options.map { if (it.requiresPro == true) it.freeInstruction.orEmpty() else it.instructionText }
            return ChatOption(
                id = options.joinToString("+") { it.optionId },
                title = options.joinToString("、") { it.title },
                instruction = joinedInstruction(options.map { it.instructionText }),
                requiresPro = if (pro) true else null,
                paidFeatures = if (pro) features else null,
                freeInstruction = if (pro) joinedInstruction(free) else null,
            )
        }

        /** ready が無い時の依頼文: 答えをもらった質問(直後が answer)を除いた利用者の発話を「。」でつなぐ。 */
        fun fallbackTranscript(chat: List<ChatBubble>): String = chat.indices
            .filter { i ->
                val answered = i + 1 < chat.size && chat[i + 1].kind == ChatTurn.Kind.ANSWER
                chat[i].fromUser && !answered
            }
            .joinToString("。") { chat[it].text }

        private fun message(b: ChatBubble) = ChatMessage(if (b.fromUser) ChatMessage.Role.USER else ChatMessage.Role.ASSISTANT, b.text)
    }
}
