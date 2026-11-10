package jp.kz2601.metamon.voice

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.SpeechTranscriber
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.TranscriberStatus
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.dropWhile
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * 音声の画面の状態(iOS の `VoiceInputView` の phase + `AppRuntime` の morph / 対話の部分)。
 * 「話す → 文字 → (相談)→ 生成 → 変身」を 1 か所で持つ。画面(Compose)は [state] を描いて操作を渡すだけ。
 * - 新規作成(初期画面)は対話 ON なら先に相談(#75 / #186)。生成後のアプリの変更はすぐ生成する(相談は android-058)
 * - 失敗は状態カード(トーストは使わない・H9)
 *
 * @param scope テストで差し替える。null なら viewModelScope
 */
class VoiceViewModel(
    private val transcriber: SpeechTranscriber,
    private val ai: AIClient,
    private val appId: () -> String,
    initial: AppDocument,
    /** generate / chat の locale。null = サーバーが発話から決める(iOS の「自動」) */
    private val locale: () -> String? = { null },
    private val clarifyEnabled: Boolean = true,
    private val autoBuild: Boolean = true,
    /** 「このまま作る」で送る言葉(backend `buildNowPhrases` に一致する UI の言語の文) */
    private val buildNowUtterance: String = "このまま作って",
    private val log: (String) -> Unit = {},
    /** サーバーの版の履歴(undo / redo)。null = 端末の履歴だけ(モック) */
    private val versions: AppVersions? = null,
    scope: CoroutineScope? = null,
) : ViewModel() {
    private val scope: CoroutineScope = scope ?: viewModelScope

    private val _state = MutableStateFlow(
        VoiceUiState(document = initial, voiceAvailable = transcriber.isAvailable),
    )
    val state: StateFlow<VoiceUiState> = _state

    private var listenJobs = emptyList<Job>()
    private var generateJob: Job? = null
    private var chatJob: Job? = null
    private var autoBuildJob: Job? = null
    private var chatEpoch = 0
    private var lastInput: InputSource? = null

    /** 状態カードの「もう一度」でやり直す操作 */
    private var retryAction: (() -> Unit)? = null

    /** 端末の版の履歴(サーバーが履歴を返せない時の undo) */
    private val localHistory = ArrayDeque<AppDocument>()
    private var redoPoint: RedoPoint? = null

    init {
        this.scope.launch {
            transcriber.status.collect { s -> _state.update { it.copy(voiceAvailable = s !is TranscriberStatus.Unavailable) } }
        }
        this.scope.launch { transcriber.prepare() }
    }

    // MARK: - マイク

    /** マイクのタップ(権限は画面が先に確かめる)。待っている間は始め、聞いている間は止める。 */
    fun onMicTap() {
        when (_state.value.phase) {
            VoicePhase.LISTENING -> stopListening()
            else -> if (_state.value.phase.canStart && !_state.value.chatPending) startListening()
        }
    }

    /** 権限のダイアログで断られた */
    fun onPermissionDenied() = showError(VoiceError.Voice(TranscriberFailure.MicrophoneDenied), retry = null)

    /** 画面が閉じた・裏に回った。録音中なら捨てる(iOS の onDisappear) */
    fun onScreenHidden() {
        if (_state.value.phase != VoicePhase.LISTENING) return
        transcriber.cancelRecording()
        stopListenJobs()
        _state.update { it.copy(phase = VoicePhase.IDLE, partial = "") }
    }

    private fun startListening() {
        clearVoiceError()
        scope.launch {
            try {
                transcriber.startRecording()
            } catch (e: CancellationException) {
                throw e
            } catch (e: TranscriberFailure) {
                showError(VoiceError.Voice(e), retry = null)
                return@launch
            }
            _state.update { it.copy(phase = VoicePhase.LISTENING, partial = "") }
            listenJobs = listOf(
                scope.launch { transcriber.partial.collect { p -> _state.update { it.copy(partial = p) } } },
                // 認識器が無音で自分から止まったら、タップを待たずに文字にする
                scope.launch {
                    transcriber.listening.dropWhile { !it }.first { !it } // true になってから false に戻った時だけ
                    if (_state.value.phase == VoicePhase.LISTENING) stopListening()
                },
            )
        }
    }

    private fun stopListening() {
        if (_state.value.phase != VoicePhase.LISTENING) return
        _state.update { it.copy(phase = VoicePhase.TRANSCRIBING) }
        stopListenJobs()
        scope.launch {
            val text = try {
                transcriber.stopAndTranscribe().trim()
            } catch (e: CancellationException) {
                throw e
            } catch (e: TranscriberFailure) {
                showError(VoiceError.Voice(e), retry = null)
                return@launch
            }
            if (text.isEmpty()) {
                showError(VoiceError.Voice(TranscriberFailure.NotHeard), retry = null)
                return@launch
            }
            _state.update { it.copy(partial = "") }
            submit(text, InputSource.VOICE)
        }
    }

    private fun stopListenJobs() {
        listenJobs.forEach { it.cancel() }
        listenJobs = emptyList()
    }

    // MARK: - 依頼

    /** 文字入力・例文のチップ・声の書き起こし。新規作成で対話 ON なら相談へ、それ以外はすぐ生成する。 */
    fun submit(text: String, source: InputSource = InputSource.TEXT) {
        val t = text.trim()
        val s = _state.value
        if (t.isEmpty() || s.isGenerating || s.chatPending) return
        if (clarifyEnabled && (s.isInitial || s.isChatting)) say(t, source) else generate(t)
    }

    /** 相談を飛ばしてすぐ生成する(DEBUG のつまみ `METAMON_AUTO_TRANSCRIPT` 用) */
    fun generateNow(text: String) {
        val t = text.trim()
        if (t.isNotEmpty()) generate(t)
    }

    /**
     * 今のアプリの最初の画面を差し替える(DEBUG のつまみ `METAMON_AUTO_NAV` 用・遷移 #357 が入るまで)。
     * その画面が無ければ何もせず false。
     */
    fun showScreen(screenId: String): Boolean {
        val dsl = _state.value.document.dsl
        if (dsl.screen(screenId) == null) return false
        _state.update { it.copy(document = it.document.copy(dsl = it.document.dsl.copy(initialScreen = screenId))) }
        return true
    }

    /**
     * ランタイムが開いたアプリ(起動時の開き直し・切り替え・新しいアプリ・ホットリロード)に合わせる。生成はしない。
     * 別のアプリになったら、前のアプリの要約・相談・元に戻す履歴は捨てる(前のアプリの版を移った先に当てない)。
     */
    fun adopt(doc: AppDocument) {
        val s = _state.value
        if (doc == s.document) return
        if (doc.appId == s.document.appId) {
            _state.update { it.copy(document = doc) }
            return
        }
        chatEpoch++
        chatJob?.cancel()
        cancelAutoBuild()
        localHistory.clear()
        redoPoint = null
        val undoable = canUndo(doc)
        _state.update {
            it.copy(
                document = doc, summary = null, chat = emptyList(), chatTurn = null, chatPending = false, transcript = "",
                summaryAction = null, undone = null, canUndo = undoable, canRedo = false, notice = null,
            )
        }
    }

    /** 生成後のアプリの上で音声のシートを開く(マイク FAB・voiceInput の部品) */
    fun openVoice() = _state.update { it.copy(voiceOpen = true) }

    fun closeVoice() {
        onScreenHidden()
        _state.update { it.copy(voiceOpen = false) }
    }

    fun dismissSummary() = _state.update { it.copy(summary = null, summaryAction = null, undone = null) }

    fun dismissNotice() = _state.update { it.copy(notice = null) }

    fun dismissError() = _state.update {
        it.copy(error = null, phase = if (it.phase == VoicePhase.ERROR) VoicePhase.IDLE else it.phase)
    }

    fun retry() {
        val action = retryAction ?: return
        dismissError()
        action()
    }

    /** 生成をやめる(生成中の表示の「キャンセル」) */
    fun cancelGenerate() {
        generateJob?.cancel()
        generateJob = null
        _state.update { it.copy(phase = VoicePhase.IDLE, progress = null) }
    }

    private fun generate(text: String) {
        if (generateJob?.isActive == true) return
        cancelAutoBuild()
        val before = _state.value.document
        retryAction = { generate(text) }
        _state.update {
            it.copy(phase = VoicePhase.SENDING, transcript = text, error = null, progress = null, summary = null, voiceOpen = false)
        }
        val target = appId()
        log("generate: appId=$target version=${before.version} transcript=$text")
        generateJob = scope.launch {
            try {
                val result = ai.morphStreaming(text, target, before.version, locale()) { onProgress(it) }
                if (appId() != target) {
                    // 待つ間に別のアプリへ切り替わった: 移った先に前のアプリの結果を出さない(iOS の morph result dropped)
                    log("generate result dropped: app changed $target → ${appId()}")
                    _state.update { it.copy(phase = VoicePhase.IDLE, progress = null, transcript = "") }
                    return@launch
                }
                val hadChat = _state.value.isChatting
                chatEpoch++
                if (!before.dsl.isVoiceOnly && !result.unchanged) remember(before)
                redoPoint = null
                val undoable = canUndo(result.document)
                _state.update {
                    it.copy(
                        phase = VoicePhase.DONE,
                        document = result.document,
                        summary = result.summary?.trim()?.ifEmpty { null },
                        summaryAction = if (undoable) UndoAction.UNDO else null,
                        undone = null,
                        canUndo = undoable,
                        canRedo = false,
                        progress = null,
                        transcript = "",
                        chat = emptyList(),
                        chatTurn = null,
                        chatPending = false,
                    )
                }
                log("generate ok: version=${result.document.version} mode=${result.mode} unchanged=${result.unchanged}")
                if (hadChat) scope.launch { ai.resetChat(appId()) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Throwable) {
                log("generate failed: $e")
                showError(VoiceError.Request(FailureKind.classify(e), VoiceError.Origin.GENERATE), retry = retryAction)
            }
        }
    }

    private fun onProgress(p: GenerateProgress) = _state.update { s ->
        if (!s.isGenerating) return@update s
        val current = s.progress ?: BuildProgress()
        val next = when (p) {
            is GenerateProgress.Restart -> BuildProgress()
            is GenerateProgress.App -> current.copy(appName = p.name, tabs = p.tabs)
            is GenerateProgress.Component -> current.copy(components = current.components + p.component)
            is GenerateProgress.Summary -> current
        }
        s.copy(phase = VoicePhase.GENERATING, progress = next)
    }

    // MARK: - 元に戻す / やり直す(#895・iOS の `AppRuntime.undo` / `redo`)

    /**
     * 直前の版に戻す。サーバーの履歴(`GET /versions` → `POST /versions/{n}/restore`)が使えなければ端末の履歴。
     * [expectedVersion] はボタンを押した時点の版。押した後に版が変わっていたら(連打の 2 回目など)何もしない。
     */
    fun undo(expectedVersion: Int? = null) {
        val s = _state.value
        if (s.undoing || s.isGenerating || (expectedVersion != null && expectedVersion != s.document.version)) return
        val before = s.document
        _state.update { it.copy(undoing = true, notice = null) }
        scope.launch {
            versions?.let { store ->
                try {
                    val list = store.versions(appId())
                    val current = list.firstOrNull()
                    if (current != null) {
                        val target = list.firstOrNull { it < current } ?: return@launch finishUndo(UndoNotice.NO_EARLIER)
                        val doc = store.restore(appId(), target)
                        log("undo ok: restored $target -> version ${doc.version}")
                        // restore は新しい版を作るので、「やり直す」は戻す前の版(current)をもう一度 restore すればよい
                        return@launch applyVersion(doc, RedoPoint(current, before, viaServer = true), UndoAction.UNDO)
                    }
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Throwable) {
                    log("undo via server failed, falling back to local history: $e")
                }
            }
            val previous = localHistory.removeLastOrNull() ?: return@launch finishUndo(UndoNotice.NO_EARLIER)
            applyVersion(previous, RedoPoint(before.version, before, viaServer = false), UndoAction.UNDO)
        }
    }

    /** 戻す前の版に戻る。サーバー経由で戻したならその版を restore、端末の履歴で戻したなら当て直す。 */
    fun redo(expectedVersion: Int? = null) {
        val s = _state.value
        val point = redoPoint ?: return
        if (s.undoing || s.isGenerating || (expectedVersion != null && expectedVersion != s.document.version)) return
        val current = s.document
        _state.update { it.copy(undoing = true, notice = null) }
        scope.launch {
            if (point.viaServer) {
                val doc = try {
                    versions?.restore(appId(), point.version) ?: return@launch finishUndo(UndoNotice.REDO_FAILED)
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Throwable) {
                    log("redo failed: $e")
                    return@launch finishUndo(UndoNotice.REDO_FAILED)
                }
                applyVersion(doc, null, UndoAction.REDO)
            } else {
                remember(current)
                applyVersion(point.document, null, UndoAction.REDO)
            }
        }
    }

    private fun applyVersion(doc: AppDocument, redo: RedoPoint?, done: UndoAction) {
        // 戻す間に別のアプリへ切り替わった: 前のアプリの版を移った先に出さない
        if (doc.appId != _state.value.document.appId) return _state.update { it.copy(undoing = false) }
        redoPoint = redo
        val undoable = canUndo(doc)
        _state.update {
            it.copy(
                document = doc,
                undoing = false,
                summary = null,
                undone = done,
                summaryAction = if (redo != null) UndoAction.REDO else if (undoable) UndoAction.UNDO else null,
                canUndo = undoable,
                canRedo = redo != null,
                error = null,
            )
        }
    }

    private fun finishUndo(notice: UndoNotice) = _state.update { it.copy(undoing = false, notice = notice) }

    private fun remember(doc: AppDocument) {
        localHistory.addLast(doc)
        while (localHistory.size > LOCAL_HISTORY_LIMIT) localHistory.removeFirst()
    }

    private fun canUndo(doc: AppDocument): Boolean =
        !doc.dsl.isVoiceOnly && ((versions != null && doc.version > 1) || localHistory.isNotEmpty())

    // MARK: - 相談(chat_first・iOS の AppRuntime+Clarify)

    /** 質問のチップ(suggestions)をタップした */
    fun answer(text: String) = submit(text, InputSource.TEXT)

    /** 案のカードの「これでいく」 */
    fun choose(option: ChatOption) {
        if (_state.value.chatPending) return
        val text = option.instructionText.trim()
        if (text.isNotEmpty()) generate(text)
    }

    /** ready の「これでいく」。transcript が無ければこれまでの発話をつなぐ */
    fun buildFromChat() {
        val s = _state.value
        val text = s.chatTurn?.transcript?.trim()?.ifEmpty { null } ?: fallbackTranscript(s.chat)
        if (text.isNotEmpty()) generate(text)
    }

    /** 「このまま作る」: ready でなければ go-ahead 語を送り、返事を待って作る(返事が来なくてもこれまでの発話で作る) */
    fun buildNow() {
        val s = _state.value
        if (!s.isChatting || s.chatPending || s.isGenerating) return
        if (s.chatTurn?.kind == ChatTurn.Kind.READY) {
            buildFromChat()
            return
        }
        appendUser(buildNowUtterance, InputSource.TEXT)
        chatJob = scope.launch {
            chatRound()
            dismissError()
            buildFromChat()
        }
    }

    /** 「最初から」 */
    fun restartChat() {
        val had = _state.value.isChatting
        chatEpoch++
        chatJob?.cancel()
        cancelAutoBuild()
        lastInput = null
        _state.update {
            it.copy(
                chat = emptyList(),
                chatTurn = null,
                chatPending = false,
                phase = if (it.phase == VoicePhase.SENDING) VoicePhase.IDLE else it.phase,
                error = if ((it.error as? VoiceError.Request)?.origin == VoiceError.Origin.CHAT) null else it.error,
            )
        }
        if (had) scope.launch { ai.resetChat(appId()) }
    }

    private fun say(text: String, source: InputSource) {
        appendUser(text, source)
        chatJob = scope.launch { chatRound() }
    }

    private fun appendUser(text: String, source: InputSource) {
        cancelAutoBuild()
        lastInput = source
        _state.update {
            it.copy(chat = it.chat + ChatBubble(fromUser = true, text = text), transcript = "", error = null)
        }
    }

    /** 今の履歴を送って返事を待つ。失敗は状態カード(origin = chat)で null。 */
    private suspend fun chatRound(): ChatTurn? {
        val epoch = chatEpoch
        val history = _state.value.chat
        retryAction = { chatJob = scope.launch { chatRound() } }
        _state.update { it.copy(phase = VoicePhase.SENDING, chatPending = true, error = null) }
        return try {
            // 新しい会話はサーバーに残った古い会話を消してから(iOS は turn で検出してやり直す。ここは先に消す)
            if (history.size == 1) ai.resetChat(appId())
            val messages = history.map {
                ChatMessage(if (it.fromUser) ChatMessage.Role.USER else ChatMessage.Role.ASSISTANT, it.text)
            }
            val turn = ai.chat(appId(), messages, locale(), ChatMode.CREATE)
            if (epoch != chatEpoch) return null
            log("chat: turn=${turn.turn} kind=${turn.kind} text=${turn.text}")
            _state.update {
                it.copy(
                    phase = VoicePhase.IDLE,
                    chatPending = false,
                    chatTurn = turn,
                    chat = it.chat + ChatBubble(fromUser = false, text = turn.text, kind = turn.kind),
                )
            }
            scheduleAutoBuildIfNeeded(turn)
            turn
        } catch (e: CancellationException) {
            throw e
        } catch (e: Throwable) {
            if (epoch != chatEpoch) return null
            log("chat failed: $e")
            showError(VoiceError.Request(FailureKind.classify(e), VoiceError.Origin.CHAT), retry = retryAction)
            null
        }
    }

    /** 声だけで新規作成を進めていて ready が来たら、少し待ってから作る(#160)。 */
    private fun scheduleAutoBuildIfNeeded(turn: ChatTurn) {
        if (!autoBuild || turn.kind != ChatTurn.Kind.READY || lastInput != InputSource.VOICE) return
        val epoch = chatEpoch
        _state.update { it.copy(autoBuildPending = true) }
        autoBuildJob = scope.launch {
            delay(AUTO_BUILD_DELAY_MS)
            if (epoch != chatEpoch || !_state.value.autoBuildPending) return@launch
            _state.update { it.copy(autoBuildPending = false) }
            buildFromChat()
        }
    }

    private fun cancelAutoBuild() {
        autoBuildJob?.cancel()
        autoBuildJob = null
        if (_state.value.autoBuildPending) _state.update { it.copy(autoBuildPending = false) }
    }

    /** ready が無い時の依頼文: go-ahead 語と、答えをもらった質問(直後が answer)を除いたユーザーの発話をつなぐ。 */
    internal fun fallbackTranscript(chat: List<ChatBubble>): String = chat.indices
        .filter { i ->
            val b = chat[i]
            val answered = i + 1 < chat.size && chat[i + 1].kind == ChatTurn.Kind.ANSWER
            b.fromUser && b.text != buildNowUtterance && !answered
        }
        .joinToString("。") { chat[it].text }

    // MARK: - 状態カード

    private fun showError(error: VoiceError, retry: (() -> Unit)?) {
        if ((error as? VoiceError.Request)?.kind == FailureKind.Cancelled) {
            _state.update { it.copy(phase = VoicePhase.IDLE, chatPending = false, progress = null) }
            return
        }
        retryAction = retry
        _state.update { it.copy(phase = VoicePhase.ERROR, error = error, chatPending = false, progress = null, partial = "") }
    }

    private fun clearVoiceError() {
        if (_state.value.error is VoiceError.Voice) _state.update { it.copy(error = null) }
    }

    override fun onCleared() {
        transcriber.cancelRecording()
    }

    companion object {
        /** ready を見せてから自動で作るまで(iOS の `ClarifyFlow.autoBuildDelay`) */
        const val AUTO_BUILD_DELAY_MS = 1_500L

        /** 端末に覚えておく版の数 */
        const val LOCAL_HISTORY_LIMIT = 20
    }
}
