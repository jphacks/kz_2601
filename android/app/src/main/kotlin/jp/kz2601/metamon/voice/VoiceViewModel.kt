package jp.kz2601.metamon.voice

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.purchase.CreationGate
import jp.kz2601.metamon.purchase.PaywallReason
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatPaidFeature
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.SpeechTranscriber
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.TranscriberStatus
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.http.RateLimitInfo
import jp.kz2601.metamon.services.privacy.PublicDataPrompt
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.dropWhile
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.time.Duration
import java.time.Instant
import kotlin.coroutines.CoroutineContext

/**
 * 音声の画面の状態(iOS の `VoiceInputView` の phase + `AppRuntime` の morph / 対話の部分)。
 * 「話す → 文字 → (相談)→ 生成 → 変身」を 1 か所で持つ。画面(Compose)は [state] を描いて操作を渡すだけ。
 * - 対話 ON なら新規作成(初期画面)も作ったアプリの変更も先に相談(#75 / #186)。OFF なら一発で反映
 * - 会話はアプリごとに端末へ残す(#210・[ChatHistoryStore])。反映・切り替えで閉じた会話も次に開いた時に見せる
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
    /** 相談してから作るか(設定・既定 ON)。毎回読むので設定の切り替えがすぐ効く */
    private val clarifyEnabled: () -> Boolean = { true },
    /** 声だけで新しく作っている時、ready で自動で作るか(#160) */
    private val autoBuild: () -> Boolean = { true },
    /** 「このまま作る」で送る言葉(backend `buildNowPhrases` に一致する UI の言語の文) */
    private val buildNowUtterance: String = "このまま作って",
    private val log: (String) -> Unit = {},
    /** サーバーの版の履歴(undo / redo)。null = 端末の履歴だけ(モック) */
    private val versions: AppVersions? = null,
    /**
     * AI に送る直前の同意の確認(#888)。未同意なら同意の画面を出して false。同意して閉じたら resume で続けて送る。
     */
    private val requireConsent: (resume: () -> Unit) -> Boolean = { true },
    /** アプリごとの会話の履歴(既定はメモリだけ) */
    private val chatHistory: ChatHistoryStore = ChatHistoryStore(null),
    /** 過去の変更の要約を AI に渡す時の見出し(UI の言語) */
    private val historyHeader: String = "(これまでに反映した変更)",
    /** 生成しても何も変わらず、サーバーが理由を返さなかった時の文(画面は strings から渡す・#1177) */
    private val unchangedFallback: String = "変更できませんでした。何をどう変えるか、もう少し詳しく教えてください。",
    /** 履歴の読み書き。順番が入れ替わらないよう 1 本ずつ */
    private val io: CoroutineContext = Dispatchers.IO.limitedParallelism(1),
    /** 挨拶・AI の質問・完成の要約の読み上げ(#43・設定で ON の時だけ読む) */
    private val speech: SpeechReader = SpeechReader.NONE,
    /** 「今日の残り」を読む口(`GET /v1/me/usage`)。null = 出さない(モック) */
    private val usage: PlanUsageSource? = null,
    /** 新規作成の無料枠と 402 の課金画面(#1261)。null = 数えない(テスト・審査用の版) */
    private val creationGate: CreationGate? = null,
    private val clock: () -> Instant = Instant::now,
    /** ai の応答のヘッダーから入る「今日の残り」(#1203・iOS の `RemainingTodayStore`)。null = 読まない(モック) */
    private val remainingFeed: StateFlow<RemainingToday?>? = null,
    /** 状態カードの「Pro にする」で課金画面を開く(#1103)。null = 課金を持たない構成(カードにボタンを出さない) */
    private val requestPaywall: ((PaywallReason) -> Unit)? = null,
    /** 発話の「アプリ一覧」「〜に切り替えて」「別のアプリ」を画面へ渡す口(#884)。null = 拾わず AI へ。画面が出ている間だけ入れる */
    var appRequests: VoiceAppRequests? = null,
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

    /** 使用量を最後に読んだ時刻(5 分は読み直さない・iOS の PlanUsageStore と同じ) */
    private var usageReadAt: Instant? = null
    private var usageJob: Job? = null

    /** 0 の上限のカードを出した日(戻る時刻。分からなければ [Instant.MAX] = 起動中に 1 度)。閉じたら同じ日にはもう出さない */
    private var exhaustedCardShownFor: Instant? = null

    init {
        remainingFeed?.let { feed -> this.scope.launch { feed.collect { v -> if (v != null) noteRemaining(v) } } }
        this.scope.launch {
            transcriber.status.collect { s -> _state.update { it.copy(voiceAvailable = s !is TranscriberStatus.Unavailable) } }
        }
        this.scope.launch { transcriber.prepare() }
        reloadHistory(initial.appId)
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
        speech.recordingFinished()
        carried = null
        _state.update { it.copy(phase = VoicePhase.IDLE, partial = "", inputLevel = 0f) }
    }

    // MARK: - 自分で止まった時の確認(#1014)

    /** 「続けて話す」の前に聞き取った分。次の聞き取りとつなぐ */
    private var carried: String? = null

    /** 確認の「送る」 */
    fun confirmPendingVoice() {
        val text = _state.value.pendingVoice ?: return
        _state.update { it.copy(pendingVoice = null) }
        submit(text, InputSource.VOICE)
    }

    /** 確認の「続けて話す」: 聞き取った分を残して聞き直し、次の分とつなぐ */
    fun continuePendingVoice() {
        val text = _state.value.pendingVoice ?: return
        carried = text
        startListening()
    }

    /** 確認の「言い直す」: 聞き取った分を捨てて聞き直す */
    fun retakePendingVoice() {
        if (_state.value.pendingVoice == null) return
        carried = null
        startListening()
    }

    /** 確認を閉じる(送らない) */
    fun dismissPendingVoice() {
        carried = null
        _state.update { it.copy(pendingVoice = null) }
    }

    private fun startListening() {
        clearVoiceError()
        _state.update { it.copy(pendingVoice = null) }
        // 読み上げ中なら即止め、録音が終わるまで読まない(TTS の音をマイクが拾わない)
        speech.stopForRecording()
        scope.launch {
            try {
                transcriber.startRecording()
            } catch (e: CancellationException) {
                throw e
            } catch (e: TranscriberFailure) {
                speech.recordingFinished()
                if (!restoreCarried()) showError(VoiceError.Voice(e), retry = null)
                return@launch
            }
            _state.update { it.copy(phase = VoicePhase.LISTENING, partial = "", inputLevel = 0f) }
            listenJobs = listOf(
                scope.launch { transcriber.partial.collect { p -> _state.update { it.copy(partial = p) } } },
                scope.launch { transcriber.level.collect { l -> _state.update { it.copy(inputLevel = l) } } },
                // 認識器が無音で自分から止まったら、文字にして送る前に確かめる(#1014)
                scope.launch {
                    transcriber.listening.dropWhile { !it }.first { !it } // true になってから false に戻った時だけ
                    if (_state.value.phase == VoicePhase.LISTENING) stopListening(autoStopped = true)
                },
            )
        }
    }

    /** 「続けて話す」の後に聞き取れなかった: 前の分を確認に戻す。戻したら true */
    private fun restoreCarried(): Boolean {
        val back = VoiceConfirm.afterFailure(carried) as? VoiceConfirm.Outcome.Confirm ?: return false
        carried = null
        _state.update { it.copy(phase = VoicePhase.IDLE, partial = "", inputLevel = 0f, pendingVoice = back.text) }
        return true
    }

    private fun stopListening(autoStopped: Boolean = false) {
        if (_state.value.phase != VoicePhase.LISTENING) return
        _state.update { it.copy(phase = VoicePhase.TRANSCRIBING, inputLevel = 0f) }
        stopListenJobs()
        scope.launch {
            val text = try {
                transcriber.stopAndTranscribe().trim()
            } catch (e: CancellationException) {
                throw e
            } catch (e: TranscriberFailure) {
                if (!restoreCarried()) showError(VoiceError.Voice(e), retry = null)
                return@launch
            } finally {
                speech.recordingFinished()
            }
            if (text.isEmpty()) {
                if (!restoreCarried()) showError(VoiceError.Voice(TranscriberFailure.NotHeard), retry = null)
                return@launch
            }
            val outcome = VoiceConfirm.afterTranscribe(autoStopped, carried, text)
            carried = null
            when (outcome) {
                is VoiceConfirm.Outcome.Send -> {
                    _state.update { it.copy(partial = "") }
                    submit(outcome.text, InputSource.VOICE)
                }
                is VoiceConfirm.Outcome.Confirm ->
                    _state.update { it.copy(phase = VoicePhase.IDLE, partial = "", pendingVoice = outcome.text) }
            }
        }
    }

    private fun stopListenJobs() {
        listenJobs.forEach { it.cancel() }
        listenJobs = emptyList()
    }

    // MARK: - 依頼

    /** 文字入力・例文のチップ・声の書き起こし。新規作成で対話 ON なら相談へ、それ以外はすぐ生成する。 */
    fun submit(text: String, source: InputSource = InputSource.TEXT) = send(text, source, intentHandled = false)

    /** @param intentHandled 同意の後の送り直し。意図はもう処理した(「別のアプリ」で appId を作り直さない) */
    private fun send(text: String, source: InputSource, intentHandled: Boolean) {
        val t = text.trim()
        val s = _state.value
        if (t.isEmpty() || s.isGenerating || s.chatPending) return
        if (s.pendingVoice != null) _state.update { it.copy(pendingVoice = null) } // 文字で送ったら確認は閉じる
        // 相談の途中の答えは意図を見ない(「前の状態」などを答えに使えるように)
        if (!intentHandled && !s.isChatting && handleIntent(t)) return
        if (!requireConsent { send(t, source, intentHandled = true) }) return
        if (clarifyEnabled()) say(t, source) else generate(t)
    }

    /**
     * AI に送る前に端末で拾う発話(#884・iOS の `AppRuntime.morph` の分岐)。処理し終えたら true。
     * 「別のアプリ」は新しいアプリに移してから、同じ文を従来どおり送る(false)。
     */
    private fun handleIntent(text: String): Boolean {
        val intent = VoiceIntent.detect(text)
        val requests = appRequests
        if (intent != VoiceIntent.Undo && requests == null) return false
        return when (intent) {
            VoiceIntent.Undo -> {
                log("voice intent: undo")
                closeVoice()
                undo()
                true
            }
            VoiceIntent.ListApps -> {
                log("voice intent: list apps")
                closeVoice()
                requests?.openAppList()
                true
            }
            is VoiceIntent.SwitchApp -> {
                log("voice intent: switch app")
                closeVoice()
                requests?.switchApp(intent.name)
                true
            }
            VoiceIntent.NewApp -> {
                log("voice intent: new app")
                requests?.startNewApp()?.let { adopt(it) }
                false
            }
            VoiceIntent.Morph -> false
        }
    }

    /** 相談を飛ばしてすぐ生成する(DEBUG のつまみ `METAMON_AUTO_TRANSCRIPT` 用) */
    fun generateNow(text: String) {
        val t = text.trim()
        if (t.isNotEmpty()) generate(t)
    }

    /** 公開の記録の確認(#899)の「公開のままにする」: 何もしない */
    fun keepPublic() = _state.update { it.copy(publicDataPrompt = null) }

    /** 「この欄を消す」: 既存の修正(generate)に指示文を送る。出した時の版から変わっていたら送らない(iOS の `removePublicPersonalFields`) */
    fun removePublicPersonalFields(english: Boolean) {
        takePublicDataPrompt()?.let { generate(it.removeInstruction(english)) }
    }

    /** 「自分だけにする」: 既存の修正に指示文を送る(iOS の `makePublicDataPrivate`) */
    fun makePublicDataPrivate(english: Boolean) {
        takePublicDataPrompt()?.let { generate(it.privateInstruction(english)) }
    }

    private fun takePublicDataPrompt(): PublicDataPrompt? {
        val s = _state.value
        val prompt = s.publicDataPrompt ?: return null
        _state.update { it.copy(publicDataPrompt = null) }
        return prompt.takeIf { it.version == s.document.version }
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
        // 今の会話は前のアプリの閉じた会話として残す(iOS の resetChat → archiveLiveChat)
        val hadChat = s.isChatting
        if (hadChat) {
            val archived = ChatHistoryStore.trimmed(s.history + liveEntries(s.chat))
            persist(s.document.appId, archived)
            val previous = s.document.appId
            scope.launch { ai.resetChat(previous) }
        }
        chatEpoch++
        chatJob?.cancel()
        cancelAutoBuild()
        lastInput = null
        localHistory.clear()
        redoPoint = null
        val undoable = canUndo(doc)
        _state.update {
            it.copy(
                document = doc, summary = null, chat = emptyList(), chatTurn = null, chatPending = false, transcript = "",
                summaryAction = null, undone = null, canUndo = undoable, canRedo = false, notice = null,
                history = emptyList(), restartConfirm = false, heldBuild = null, publicDataPrompt = null,
                phase = if (it.chatPending && it.phase == VoicePhase.SENDING) VoicePhase.IDLE else it.phase,
                error = if ((it.error as? VoiceError.Request)?.origin == VoiceError.Origin.CHAT) null else it.error,
            )
        }
        reloadHistory(doc.appId)
    }

    /** 生成後のアプリの上で音声のシートを開く(マイク FAB・voiceInput の部品) */
    fun openVoice() = _state.update { it.copy(voiceOpen = true) }

    /** 入力欄に文を入れて音声の画面を開く(送らない。設定の「頼み方のコツ」・iOS の `composerDraft`) */
    fun putDraft(text: String) = _state.update { it.copy(draft = text, voiceOpen = !it.isInitial || it.voiceOpen) }

    /** 入力欄が [VoiceUiState.draft] を受け取った */
    fun consumeDraft() = _state.update { it.copy(draft = null) }

    fun closeVoice() {
        onScreenHidden()
        _state.update { it.copy(voiceOpen = false) }
    }

    fun dismissSummary() = _state.update { it.copy(summary = null, summaryAction = null, undone = null) }

    fun dismissNotice() = _state.update { it.copy(notice = null) }

    fun dismissError() = _state.update {
        it.copy(error = null, phase = if (it.phase == VoicePhase.ERROR) VoicePhase.IDLE else it.phase)
    }

    /** 状態カードの「Pro にする」の行き先。課金を持たない構成では null */
    fun upgradeHandler(): ((PaywallReason) -> Unit)? = requestPaywall?.let { open -> { reason -> upgrade(reason, open) } }

    /** 上限のカードの「Pro にする」(iOS の `upgradeFromRateLimit`)。カードは閉じずに課金画面を開く */
    private fun upgrade(reason: PaywallReason, open: (PaywallReason) -> Unit) {
        log("[Metamon] IAP: paywall from error card reason=$reason")
        open(reason)
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

    /**
     * 新規作成(初期画面から)は 1 日の無料枠を通す(#1261・iOS の `sendMetered`)。枠切れなら課金画面を出し、買えばそのまま作る。
     * 閉じたら送らず、会話を残して「続きから作る」のカード(対話中でなければ入力欄に依頼を戻す)。成功した時だけ数える。
     */
    private fun generate(text: String) {
        if (generateJob?.isActive == true) return
        if (!requireConsent { generate(text) }) return
        val gate = creationGate
        if (gate == null || !_state.value.document.dsl.isVoiceOnly) return startGenerate(text, metered = false)
        cancelAutoBuild()
        _state.update { it.copy(heldBuild = null) }
        generateJob = scope.launch {
            if (!gate.admitNewApp()) return@launch holdBuild(HeldBuild.LimitReached(text, gate.dailyFreeLimit))
            startGenerate(text, metered = true)
        }
    }

    /** @param paidRetry 402 の後に買って送り直した 1 回(もう一度 402 なら課金画面を出し直さない) */
    private fun startGenerate(text: String, metered: Boolean, paidRetry: Boolean = false) {
        cancelAutoBuild()
        val before = _state.value.document
        retryAction = { generate(text) }
        _state.update {
            it.copy(
                phase = VoicePhase.SENDING, transcript = text, error = null, progress = null, summary = null, voiceOpen = false,
                heldBuild = null,
            )
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
                if (result.unchanged) {
                    val reopenChat = hadChat && !before.dsl.isVoiceOnly
                    if (hadChat && !reopenChat) {
                        // 会話を閉じるので、反映した時と同じく相談を区切る(前の相談の答えを次へ持ち越さない)
                        chatEpoch++
                        lastInput = null
                        scope.launch { ai.resetChat(target) }
                    }
                    return@launch presentUnchanged(result.summary, reopenChat)
                }
                val history = appliedHistory(_state.value, result.document.version, result.summary)
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
                        history = history,
                        publicDataPrompt = PublicDataPrompt.fromMeta(result.publicPersonalData, result.document.version),
                    )
                }
                _state.value.publicDataPrompt?.let { p -> log("public personal data: ${p.fields.map { "${it.entity}.${it.field}(${it.kind})" }}") }
                persist(result.document.appId, history)
                if (metered && !result.document.dsl.isVoiceOnly) creationGate?.recordNewApp()
                log("generate ok: version=${result.document.version} mode=${result.mode} unchanged=${result.unchanged}")
                _state.value.summary?.let { speech.speak(it) } // #43 完成の要約(OFF なら何もしない)
                refreshUsage(force = true)
                if (hadChat) {
                    lastInput = null
                    scope.launch { ai.resetChat(target) }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Throwable) {
                log("generate failed: $e")
                val kind = FailureKind.classify(e)
                if (metered && !paidRetry && kind is FailureKind.ProRequiredKind && !kind.info.creatorNeedsPro && holdForProRequired(text, kind, target)) {
                    return@launch
                }
                showError(VoiceError.Request(kind, VoiceError.Origin.GENERATE), retry = retryAction)
            }
        }
    }

    /**
     * 生成しても何も変わらなかった(#211・#1177・iOS の `presentUnchanged`)。保存されていないので版・元に戻す・反映の区切りは触らない。
     * 変更の相談から来たなら会話を残したまま理由の吹き出しを足してシートを開き直し(続けて話せる)、それ以外は要約のバナーに出す。
     */
    private fun presentUnchanged(summary: String?, reopenChat: Boolean) {
        val reason = summary?.trim()?.ifEmpty { null } ?: unchangedFallback
        log("generate unchanged: reopenChat=$reopenChat")
        _state.update {
            val base = it.copy(phase = VoicePhase.IDLE, progress = null, transcript = "")
            if (reopenChat) {
                base.copy(chat = it.chat + ChatBubble(fromUser = false, text = reason, kind = ChatTurn.Kind.ANSWER), chatTurn = null, voiceOpen = true)
            } else {
                base.copy(
                    phase = VoicePhase.DONE, summary = reason, summaryAction = null, undone = null,
                    chat = emptyList(), chatTurn = null, chatPending = false,
                )
            }
        }
        if (reopenChat) persistLive()
        speech.speak(reason)
        refreshUsage(force = true)
    }

    // MARK: - 止めた作成(#1261・iOS の AppRuntime+HeldBuild)

    /**
     * 402 `pro_required` で新規作成が止まった(何も保存されていない)。課金画面を出し、買えば同じ依頼を送り直す。
     * 閉じたら対話中は会話を残して確認カード(Pro にする / この機能なしで作る)。対話中でなければ false(状態カードに任せる)。
     */
    private suspend fun holdForProRequired(text: String, kind: FailureKind.ProRequiredKind, target: String): Boolean {
        val gate = creationGate ?: return false
        _state.update { it.copy(phase = VoicePhase.IDLE, progress = null) }
        val bought = gate.requestPro(kind.info.features)
        if (appId() != target) return true
        if (bought) {
            startGenerate(text, metered = true, paidRetry = true)
            return true
        }
        if (!_state.value.isChatting) return false
        holdBuild(HeldBuild.ProRequired(text, kind.info.features))
        return true
    }

    /** 作らずに止めた。対話中なら会話を残してカードを出し、そうでなければ入力欄に依頼を戻す(言い直さなくてよい)。 */
    private fun holdBuild(held: HeldBuild) {
        log("generate held: ${held::class.simpleName} chatting=${_state.value.isChatting}")
        _state.update {
            val idle = it.copy(phase = VoicePhase.IDLE, progress = null, transcript = "")
            if (it.isChatting) idle.copy(heldBuild = held) else idle.copy(draft = held.text)
        }
    }

    /** 「続きから作る」: 同じ依頼で作る。まだ枠が無く Pro でもなければ、もう一度課金画面を出す(閉じたらカードに戻る)。 */
    fun continueHeldBuild() {
        val held = _state.value.heldBuild ?: return
        if (_state.value.isGenerating || generateJob?.isActive == true) return
        _state.update { it.copy(heldBuild = null) }
        log("continue held build")
        generate(held.text)
    }

    /** 402 のカードの「Pro にする」: 課金画面を出し、買えば同じ依頼で作る。閉じたらカードのまま。 */
    fun purchaseAndBuild() {
        val held = _state.value.heldBuild as? HeldBuild.ProRequired ?: return
        val gate = creationGate ?: return
        if (_state.value.isGenerating || generateJob?.isActive == true) return
        generateJob = scope.launch {
            if (!gate.requestPro(held.features)) return@launch
            if (_state.value.heldBuild != held) return@launch
            // 変更の相談から出したカード(#1175)は新規作成ではないので数えない
            startGenerate(held.text, metered = _state.value.isInitial, paidRetry = true)
        }
    }

    /**
     * 相談の「これでいく」の前に、有料の機能を含む案なら作らずに確認カードを出す(#1175・iOS の `holdForPaidConfirm`)。
     * Pro・審査用の版([creationGate] が null)・古い backend(requiresPro が無い)では出さない。出したら true。
     */
    private fun holdForPaidConfirm(requiresPro: Boolean?, features: List<ChatPaidFeature>?, text: String, free: String?): Boolean {
        val gate = creationGate ?: return false
        if (requiresPro != true || gate.isPro || !_state.value.isChatting) return false
        cancelAutoBuild()
        log("paid feature confirm: features=${features.orEmpty().map { it.id }}")
        holdBuild(HeldBuild.ProRequired(text, features.orEmpty().map { PaidFeature(it.id, it.title.orEmpty(), it.reason) }, free))
        return true
    }

    /** 402 のカードの「この機能なしで作る」 */
    fun buildWithoutPaidFeatures() {
        val held = _state.value.heldBuild as? HeldBuild.ProRequired ?: return
        if (_state.value.isGenerating || generateJob?.isActive == true) return
        _state.update { it.copy(heldBuild = null) }
        generate(held.freeText)
    }

    /** カードを閉じてチャットに戻る(会話は残る。「これでいく」からまた作れる) */
    fun dismissHeldBuild() = _state.update { it.copy(heldBuild = null) }

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

    /** 複数選択の質問の「決定」: 選んだチップを「、」でつないで 1 つの答えにする(#220) */
    fun answerMany(chips: List<String>) {
        val text = MultiSelect.joinedAnswer(chips)
        if (text.isNotEmpty()) answer(text)
    }

    /** 案のカードの「これでいく」 */
    fun choose(option: ChatOption) {
        val s = _state.value
        if (s.chatPending || s.isGenerating) return
        val text = option.instructionText.trim()
        if (text.isEmpty()) return
        if (holdForPaidConfirm(option.requiresPro, option.paidFeatures, text, option.freeInstruction)) return
        generate(text)
    }

    /** 複数選択の案の「これでいく(N 件)」: 選んだ案を 1 つにまとめて作る(#220) */
    fun chooseMany(options: List<ChatOption>) {
        MultiSelect.combined(options)?.let(::choose)
    }

    /** ready の「これでいく」。transcript が無ければこれまでの発話をつなぐ */
    fun buildFromChat() {
        val s = _state.value
        val transcript = s.chatTurn?.transcript?.trim()?.ifEmpty { null }
        val text = transcript ?: fallbackTranscript(s.chat)
        if (text.isEmpty()) return
        // ready の transcript で作る時だけ確かめる(予備の依頼文には判定の材料が無い・iOS と同じ)
        val turn = s.chatTurn
        if (transcript != null && turn?.kind == ChatTurn.Kind.READY &&
            holdForPaidConfirm(turn.requiresPro, turn.paidFeatures, text, turn.freeTranscript)
        ) {
            return
        }
        generate(text)
    }

    /** 「このまま作る」: ready でなければ go-ahead 語を送り、返事を待って作る(返事が来なくてもこれまでの発話で作る) */
    fun buildNow() {
        val s = _state.value
        if (!s.isChatting || s.chatPending || s.isGenerating) return
        if (!requireConsent { buildNow() }) return
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

    /** 変更の相談の聞き返し中の「このまま反映」: これまでの指示(答えをもらった質問は除く)で変える */
    fun applyNow() {
        val s = _state.value
        if (!s.isChatting || s.chatPending || s.isGenerating) return
        val text = fallbackTranscript(s.chat)
        if (text.isNotEmpty()) generate(text)
    }

    /**
     * 「最初から」(iOS の `requestRestartChat`・M9)。これまでの会話(閉じた会話・反映の区切り)があれば
     * 消してよいかを確かめる。無ければ今の会話だけ消す。
     */
    fun restartChat() {
        if (_state.value.history.isNotEmpty()) {
            _state.update { it.copy(restartConfirm = true) }
        } else {
            restartLiveChat()
        }
    }

    /** 今の会話だけ消す。これまでの区切りは残す */
    fun restartLiveChat() {
        resetLiveChat()
        val s = _state.value
        persist(s.document.appId, s.history)
    }

    /** 確かめた上で「これまでの会話も消す」: 今の会話も、このアプリの履歴も消す(戻せない) */
    fun clearChatHistory() {
        resetLiveChat()
        val appId = _state.value.document.appId
        _state.update { it.copy(history = emptyList()) }
        scope.launch(io) { chatHistory.remove(appId) }
    }

    fun dismissRestartConfirm() = _state.update { it.copy(restartConfirm = false) }

    private fun resetLiveChat() {
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
                restartConfirm = false,
                heldBuild = null,
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
            // 話し直したら止めた作成のカードは閉じる(iOS と同じ)
            it.copy(chat = it.chat + ChatBubble(fromUser = true, text = text), transcript = "", error = null, heldBuild = null)
        }
        persistLive()
    }

    /** 今の履歴を送って返事を待つ。失敗は状態カード(origin = chat)で null。 */
    private suspend fun chatRound(): ChatTurn? {
        val epoch = chatEpoch
        val s = _state.value
        val target = appId()
        val mode = s.chatMode
        // 過去の変更の要約 + 最後の反映より後 + 今の会話(#210)
        val messages = ChatContext.messages(s.history, s.chat.map(::message), historyHeader)
        retryAction = { chatJob = scope.launch { chatRound() } }
        _state.update { it.copy(phase = VoicePhase.SENDING, chatPending = true, error = null) }
        return try {
            val turn = if (messages.size == 1) {
                startConversation(target, messages, mode)
            } else {
                ai.chat(target, messages, locale(), mode)
            }
            if (epoch != chatEpoch) return null
            // 発話・返事の中身はログに出さない(種類と数だけ)
            log("chat: mode=$mode turn=${turn.turn} kind=${turn.kind} options=${turn.options?.size ?: 0} multi=${turn.isMultiSelect}")
            _state.update {
                it.copy(
                    phase = VoicePhase.IDLE,
                    chatPending = false,
                    chatTurn = turn,
                    chat = it.chat + ChatBubble(fromUser = false, text = turn.text, kind = turn.kind),
                )
            }
            persistLive()
            speech.speak(turn.text) // #43 AI の質問(録音中・OFF なら読まない)
            refreshUsage(force = true)
            scheduleAutoBuildIfNeeded(turn, mode)
            turn
        } catch (e: CancellationException) {
            throw e
        } catch (e: Throwable) {
            if (epoch != chatEpoch) return null
            val kind = FailureKind.classify(e)
            log("chat failed: $kind")
            showError(VoiceError.Request(kind, VoiceError.Origin.CHAT), retry = retryAction)
            null
        }
    }

    /**
     * 新しい会話の 1 発目(iOS の `ClarifyFlow.startConversation`)。DELETE を先に待たない。
     * 1 件だけ送って `turn != 1` なら、サーバーに残った古い会話に追記されている: その時だけ消して送り直す。
     */
    private suspend fun startConversation(appId: String, messages: List<ChatMessage>, mode: ChatMode): ChatTurn {
        val turn = ai.chat(appId, messages, locale(), mode)
        if (turn.turn == 1) return turn
        log("chat: stale conversation on server -> cleared and resent")
        ai.resetChat(appId)
        return ai.chat(appId, messages, locale(), mode)
    }

    /** 声だけで新規作成を進めていて ready が来たら、少し待ってから作る(#160)。変更の相談は必ずボタンで。 */
    private fun scheduleAutoBuildIfNeeded(turn: ChatTurn, mode: ChatMode) {
        if (!autoBuild() || mode != ChatMode.CREATE || turn.kind != ChatTurn.Kind.READY || lastInput != InputSource.VOICE) return
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

    // MARK: - 会話の履歴(#210・iOS の AppRuntime+ChatHistory)

    private fun reloadHistory(appId: String) {
        scope.launch {
            val loaded = withContext(io) { chatHistory.load(appId) }
            _state.update { if (it.document.appId == appId) it.copy(history = loaded) else it }
        }
    }

    /** 反映した所に区切りを入れた履歴(今の会話は閉じた会話に移す)。対話を使っていないアプリには入れない */
    private fun appliedHistory(s: VoiceUiState, version: Int, summary: String?): List<ChatHistoryEntry> {
        val archived = s.history + liveEntries(s.chat)
        if (archived.isEmpty()) return archived
        return ChatHistoryStore.trimmed(archived + ChatHistoryEntry(ChatHistoryEntry.Kind.APPLIED, summary?.trim().orEmpty(), version))
    }

    /** 端末に保存する(閉じた会話 + 今の会話)。読み直すと全部が閉じた会話になる */
    private fun persistLive() {
        val s = _state.value
        persist(s.document.appId, s.history + liveEntries(s.chat))
    }

    private fun persist(appId: String, entries: List<ChatHistoryEntry>) {
        scope.launch(io) { chatHistory.save(entries, appId) }
    }

    private fun liveEntries(chat: List<ChatBubble>) = chat.map {
        ChatHistoryEntry(if (it.fromUser) ChatHistoryEntry.Kind.USER else ChatHistoryEntry.Kind.ASSISTANT, it.text)
    }

    private fun message(b: ChatBubble) = ChatMessage(if (b.fromUser) ChatMessage.Role.USER else ChatMessage.Role.ASSISTANT, b.text)

    // MARK: - 挨拶・今日の残り(#892)

    /** 初期画面に来た経路(挨拶の出し分け)。切り替えの「+ 新しいアプリ」など */
    fun noteHomeEntry(entry: Greeting.HomeEntry) = _state.update { it.copy(homeEntry = entry) }

    /** 初期画面の挨拶を読む(相談中・生成中は読まない)。読んだら true */
    fun speakGreeting(text: String): Boolean {
        val s = _state.value
        if (!s.isInitial || s.isChatting || s.isBusy || s.phase == VoicePhase.LISTENING) return false
        return speech.speak(text)
    }

    /** 画面を離れた・設定で切った時に読み上げを止める */
    fun stopSpeech() = speech.stop()

    /** 「今日の残り」を読み直す。[force] でなければ 5 分は読み直さない。読めなければ前の値のまま */
    fun refreshUsage(force: Boolean = false) {
        val source = usage ?: return
        val now = clock()
        val last = usageReadAt
        if (!force && last != null && Duration.between(last, now) < USAGE_TTL) return
        if (usageJob?.isActive == true) return
        usageReadAt = now
        usageJob = scope.launch {
            val me = try {
                source.myUsage()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Throwable) {
                log("usage failed: ${e.javaClass.simpleName}")
                return@launch
            } ?: return@launch
            val today = me.remainingToday(observedAt = now)
            _state.update {
                // 後から届いた古い値では上書きしない
                val keep = it.remainingToday?.let { cur -> today != null && cur.observedAt.isAfter(today.observedAt) } == true
                it.copy(
                    remainingToday = if (keep) it.remainingToday else today ?: it.remainingToday,
                    isPro = me.isPro,
                    myUsage = if (keep) it.myUsage ?: me else me,
                )
            }
            showExhaustedCardIfNeeded()
        }
    }

    /** ai の応答のヘッダーから届いた「今日の残り」(#1203)。古い値では上書きしない */
    private fun noteRemaining(value: RemainingToday) {
        _state.update {
            val cur = it.remainingToday
            if (cur != null && cur.observedAt.isAfter(value.observedAt)) it else it.copy(remainingToday = value)
        }
        showExhaustedCardIfNeeded()
    }

    /**
     * 残りが 0 になったら上限のカードを同じ日に 1 度だけ出す(iOS の `RemainingTodayBadge.showExhaustedCardIfNeeded`)。
     * ほかのカードが出ている・生成や相談の途中は出さない(終わった後の usage の読み直しでもう一度見る)。
     */
    private fun showExhaustedCardIfNeeded() {
        val s = _state.value
        val display = RemainingTodayDisplay.of(s.remainingToday, clock(), s.isPro) as? RemainingTodayDisplay.Exhausted ?: return
        if (s.error != null || s.isGenerating || s.chatPending) return
        val key = display.resetAt ?: Instant.MAX
        if (exhaustedCardShownFor == key) return
        exhaustedCardShownFor = key
        val tier = if (s.isPro) "pro" else s.myUsage?.tier ?: "signed_in"
        val info = RateLimitInfo(RateLimitInfo.Scope.USER, tier = tier, limit = s.remainingToday?.limit, remaining = 0, resetAt = display.resetAt)
        retryAction = null
        _state.update { it.copy(error = VoiceError.Request(FailureKind.RateLimited(info), VoiceError.Origin.GENERATE)) }
    }

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
        speech.stop()
    }

    companion object {
        /** ready を見せてから自動で作るまで(iOS の `ClarifyFlow.autoBuildDelay`) */
        const val AUTO_BUILD_DELAY_MS = 1_500L

        /** 端末に覚えておく版の数 */
        const val LOCAL_HISTORY_LIMIT = 20

        /** 使用量を読み直すまで */
        val USAGE_TTL: Duration = Duration.ofMinutes(5)
    }
}
