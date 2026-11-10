package jp.kz2601.metamon.voice

import jp.kz2601.metamon.consent.ConsentPrompt
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.SpeechTranscriber
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.TranscriberStatus
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

@OptIn(ExperimentalCoroutinesApi::class)
class VoiceViewModelTest {
    private val initial = AppDocument(
        appId = "app-1",
        version = 0,
        dsl = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"voice",
               "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}""",
        ),
    )
    private val generated = AppDocument(
        appId = "app-1",
        version = 1,
        dsl = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
               "screens":[{"id":"home","title":"家計簿","components":[]}]}""",
        ),
    )

    /** 呼ばれた内容を残し、応答は [chatTurns] / [morphResult] / [gate] で決める偽物。 */
    private inner class FakeAI : AIClient {
        val morphCalls = mutableListOf<String>()
        val chatCalls = mutableListOf<List<ChatMessage>>()
        var resets = 0
        var progress: List<GenerateProgress> = emptyList()
        var morphError: Throwable? = null
        var gate: CompletableDeferred<Unit>? = null
        val chatTurns = ArrayDeque<Result<ChatTurn>>()

        override suspend fun morph(transcript: String, appId: String, currentVersion: Int, locale: String?, mode: GenerateMode): MorphResult =
            error("not used")

        override suspend fun morphStreaming(
            transcript: String,
            appId: String,
            currentVersion: Int,
            locale: String?,
            mode: GenerateMode,
            progress: (GenerateProgress) -> Unit,
        ): MorphResult {
            morphCalls += transcript
            this.progress.forEach(progress)
            gate?.await()
            morphError?.let { throw it }
            return MorphResult(document = generated, summary = "家計簿ができました", envelopeJson = "{}")
        }

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn {
            chatCalls += messages
            return chatTurns.removeFirst().getOrThrow()
        }

        override suspend fun resetChat(appId: String) {
            resets++
        }
    }

    private fun TestScope.vm(
        transcriber: MockSpeechTranscriber = MockSpeechTranscriber(),
        ai: AIClient = FakeAI(),
        clarify: Boolean = false,
    ) = VoiceViewModel(
        transcriber = transcriber,
        ai = ai,
        appId = { "app-1" },
        initial = initial,
        clarifyEnabled = clarify,
        // backgroundScope の仕事は advanceUntilIdle が待たないので、同じスケジューラの別のスコープで動かす
        scope = CoroutineScope(StandardTestDispatcher(testScheduler) + Job()),
    )

    @Test
    fun withoutConsentNothingIsSentAndTheHeldUtteranceResumesAfterAgree() = runTest {
        val consent = AiConsentStore(InMemoryKeyValueStore())
        val prompt = ConsentPrompt(consent)
        val ai = FakeAI()
        val vm = VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { "app-1" },
            initial = initial,
            clarifyEnabled = false,
            requireConsent = prompt::require,
            scope = CoroutineScope(StandardTestDispatcher(testScheduler) + Job()),
        )
        vm.submit("家計簿")
        advanceUntilIdle()
        assertTrue(ai.morphCalls.isEmpty())
        assertTrue(prompt.visible.value)
        prompt.agree() // 同意して閉じたら話し直さずに送る(iOS の resumeUtteranceAfterConsent)
        advanceUntilIdle()
        assertEquals(listOf("家計簿"), ai.morphCalls)
    }

    @Test
    fun generateNowSkipsChatEvenWhenClarifyIsOn() = runTest {
        val ai = FakeAI()
        val vm = vm(ai = ai, clarify = true)
        vm.generateNow("  家計簿アプリが欲しい ")
        advanceUntilIdle()
        assertEquals(listOf("家計簿アプリが欲しい"), ai.morphCalls)
        assertTrue(ai.chatCalls.isEmpty())
        assertEquals(generated, vm.state.value.document)
    }

    @Test
    fun showScreenSwapsInitialScreenOnlyWhenItExists() = runTest {
        val vm = vm()
        vm.generateNow("家計簿")
        advanceUntilIdle()
        assertFalse(vm.showScreen("missing"))
        assertEquals("home", vm.state.value.document.dsl.initialScreen)
        assertTrue(vm.showScreen("home"))
        assertEquals("home", vm.state.value.document.dsl.initialScreen)
    }

    @Test
    fun voiceToGenerateToDone() = runTest {
        val speech = MockSpeechTranscriber(results = listOf(Result.success("家計簿アプリが欲しい")))
        val ai = FakeAI().apply {
            gate = CompletableDeferred()
            progress = listOf(
                GenerateProgress.App("家計簿", listOf("ホーム", "グラフ"), 0),
                GenerateProgress.Component(StreamedComponent(attempt = 0, index = 0, type = "text", label = "今月")),
            )
        }
        val vm = vm(speech, ai)
        runCurrent()
        assertEquals(VoicePhase.IDLE, vm.state.value.phase)

        vm.onMicTap()
        runCurrent()
        assertEquals(VoicePhase.LISTENING, vm.state.value.phase)
        speech.emitPartial("家計簿")
        runCurrent()
        assertEquals("家計簿", vm.state.value.partial)

        vm.onMicTap()
        runCurrent()
        // 途中経過が届いたので GENERATING。結果を待っている
        val generating = vm.state.value
        assertEquals(VoicePhase.GENERATING, generating.phase)
        assertTrue(generating.isGenerating)
        assertEquals("家計簿アプリが欲しい", generating.transcript)
        assertEquals("家計簿", generating.progress?.appName)
        assertEquals(1, generating.progress?.components?.size)
        assertEquals(listOf("家計簿アプリが欲しい"), ai.morphCalls)

        ai.gate!!.complete(Unit)
        advanceUntilIdle()
        val done = vm.state.value
        assertEquals(VoicePhase.DONE, done.phase)
        assertEquals(generated, done.document)
        assertFalse(done.isInitial)
        assertEquals("家計簿ができました", done.summary)
        assertNull(done.progress)
    }

    @Test
    fun sendingBeforeAnyProgress() = runTest {
        val ai = FakeAI().apply { gate = CompletableDeferred() }
        val vm = vm(ai = ai)
        vm.submit("  筋トレの記録  ")
        runCurrent()
        assertEquals(VoicePhase.SENDING, vm.state.value.phase)
        assertEquals("筋トレの記録", vm.state.value.transcript)
        // 生成中は次の依頼を受けない
        vm.submit("別の依頼")
        runCurrent()
        assertEquals(listOf("筋トレの記録"), ai.morphCalls)
        vm.cancelGenerate()
        runCurrent()
        assertEquals(VoicePhase.IDLE, vm.state.value.phase)
        assertEquals(initial, vm.state.value.document)
    }

    @Test
    fun transcriberFailureShowsVoiceErrorAndRecovers() = runTest {
        val speech = MockSpeechTranscriber(results = listOf(Result.failure(TranscriberFailure.Silent(4.0)), Result.success("読んだ本のメモ")))
        val ai = FakeAI()
        val vm = vm(speech, ai)
        vm.onMicTap()
        runCurrent()
        vm.onMicTap()
        runCurrent()
        assertEquals(VoicePhase.ERROR, vm.state.value.phase)
        assertEquals(VoiceError.Voice(TranscriberFailure.Silent(4.0)), vm.state.value.error)
        assertTrue(ai.morphCalls.isEmpty())

        // 話し直すと録音の失敗のカードは消える
        vm.onMicTap()
        runCurrent()
        assertEquals(VoicePhase.LISTENING, vm.state.value.phase)
        assertNull(vm.state.value.error)
        vm.onMicTap()
        advanceUntilIdle()
        assertEquals(VoicePhase.DONE, vm.state.value.phase)
        assertEquals(listOf("読んだ本のメモ"), ai.morphCalls)
    }

    @Test
    fun recognizerEndingBySelfTranscribesWithoutTap() = runTest {
        val speech = MockSpeechTranscriber(results = listOf(Result.success("早起きチェック")))
        val ai = FakeAI()
        val vm = vm(speech, ai)
        vm.onMicTap()
        runCurrent()
        speech.endBySelf()
        advanceUntilIdle()
        assertEquals(listOf("早起きチェック"), ai.morphCalls)
        assertEquals(VoicePhase.DONE, vm.state.value.phase)
    }

    /** iOS と同じ最小の口だけの実装(feat/android-mock の MockTranscriber と同じ形)でも動く。 */
    @Test
    fun minimalTranscriberWithoutFlows() = runTest {
        val minimal = object : SpeechTranscriber {
            override val isReady = true
            override suspend fun prepare() = Unit
            override suspend fun startRecording() = Unit
            override suspend fun stopAndTranscribe() = "サークルの出欠"
            override fun cancelRecording() = Unit
        }
        val ai = FakeAI()
        val vm = VoiceViewModel(
            minimal,
            ai,
            { "app-1" },
            initial,
            clarifyEnabled = false,
            scope = CoroutineScope(StandardTestDispatcher(testScheduler) + Job()),
        )
        vm.onMicTap()
        advanceUntilIdle()
        // listening が流れなくても勝手に止まらない
        assertEquals(VoicePhase.LISTENING, vm.state.value.phase)
        vm.onMicTap()
        advanceUntilIdle()
        assertEquals(listOf("サークルの出欠"), ai.morphCalls)
    }

    @Test
    fun startFailures() = runTest {
        val speech = MockSpeechTranscriber(status = TranscriberStatus.Unavailable("x"))
        val vm = vm(speech)
        runCurrent()
        assertFalse(vm.state.value.voiceAvailable)
        vm.onMicTap()
        runCurrent()
        assertEquals(VoiceError.Voice(TranscriberFailure.OnDeviceRecognitionUnavailable), vm.state.value.error)

        vm.dismissError()
        assertEquals(VoicePhase.IDLE, vm.state.value.phase)
        vm.onPermissionDenied()
        assertEquals(VoiceError.Voice(TranscriberFailure.MicrophoneDenied), vm.state.value.error)
        assertEquals(VoicePhase.ERROR, vm.state.value.phase)
    }

    @Test
    fun screenHiddenWhileListeningDiscardsRecording() = runTest {
        val speech = MockSpeechTranscriber()
        val vm = vm(speech)
        vm.onMicTap()
        runCurrent()
        vm.onScreenHidden()
        runCurrent()
        assertEquals(VoicePhase.IDLE, vm.state.value.phase)
        assertEquals(1, speech.cancelCount)
    }

    @Test
    fun generateFailureIsClassifiedAndRetried() = runTest {
        val ai = FakeAI().apply { morphError = IOException("offline") }
        val vm = vm(ai = ai)
        vm.submit("おこづかい帳")
        advanceUntilIdle()
        assertEquals(VoicePhase.ERROR, vm.state.value.phase)
        assertEquals(VoiceError.Request(FailureKind.Offline, VoiceError.Origin.GENERATE), vm.state.value.error)

        ai.morphError = null
        vm.retry()
        advanceUntilIdle()
        assertEquals(VoicePhase.DONE, vm.state.value.phase)
        assertEquals(listOf("おこづかい帳", "おこづかい帳"), ai.morphCalls)
    }

    @Test
    fun serverErrorKinds() = runTest {
        val ai = FakeAI().apply { morphError = ApiError.Http(503, "") }
        val vm = vm(ai = ai)
        vm.submit("サブスクの管理")
        advanceUntilIdle()
        val error = vm.state.value.error as VoiceError.Request
        assertTrue(error.kind is FailureKind.Busy)
    }

    @Test
    fun clarifyQuestionThenReadyBuilds() = runTest {
        val ai = FakeAI()
        ai.chatTurns += Result.success(ChatTurn(kind = ChatTurn.Kind.QUESTION, text = "何を記録しますか?", suggestions = listOf("支出", "収入")))
        ai.chatTurns += Result.success(ChatTurn(kind = ChatTurn.Kind.READY, text = "まとめました", transcript = "支出を記録する家計簿"))
        val vm = vm(ai = ai, clarify = true)

        vm.submit("家計簿")
        runCurrent()
        assertEquals(VoicePhase.IDLE, vm.state.value.phase)
        assertEquals(ChatTurn.Kind.QUESTION, vm.state.value.chatTurn?.kind)
        assertEquals(2, vm.state.value.chat.size)
        assertEquals(1, ai.resets) // 新しい会話は先にサーバー側を消す

        vm.answer("支出")
        runCurrent()
        assertEquals(listOf("家計簿", "何を記録しますか?", "支出"), ai.chatCalls.last().map { it.text })
        assertEquals(ChatTurn.Kind.READY, vm.state.value.chatTurn?.kind)
        // 文字で進めたので自動では作らない
        assertFalse(vm.state.value.autoBuildPending)
        assertTrue(ai.morphCalls.isEmpty())

        vm.buildFromChat()
        advanceUntilIdle()
        assertEquals(listOf("支出を記録する家計簿"), ai.morphCalls)
        assertEquals(VoicePhase.DONE, vm.state.value.phase)
        assertTrue(vm.state.value.chat.isEmpty())
    }

    @Test
    fun clarifyByVoiceAutoBuildsAfterDelay() = runTest {
        val speech = MockSpeechTranscriber(results = listOf(Result.success("水の量")))
        val ai = FakeAI()
        ai.chatTurns += Result.success(ChatTurn(kind = ChatTurn.Kind.READY, text = "OK", transcript = "毎日の水の量"))
        val vm = vm(speech, ai, clarify = true)
        vm.onMicTap()
        runCurrent()
        vm.onMicTap()
        runCurrent()
        assertTrue(vm.state.value.autoBuildPending)
        advanceTimeBy(VoiceViewModel.AUTO_BUILD_DELAY_MS - 1)
        assertTrue(ai.morphCalls.isEmpty())
        advanceUntilIdle()
        assertEquals(listOf("毎日の水の量"), ai.morphCalls)
    }

    @Test
    fun clarifyProposalAndRestart() = runTest {
        val ai = FakeAI()
        val option = ChatOption(title = "シンプル", instruction = "シンプルな家計簿")
        ai.chatTurns += Result.success(ChatTurn(kind = ChatTurn.Kind.PROPOSAL, text = "案です", options = listOf(option)))
        ai.chatTurns += Result.failure(IOException("down"))
        val vm = vm(ai = ai, clarify = true)
        vm.submit("家計簿")
        runCurrent()
        vm.restartChat()
        runCurrent()
        assertTrue(vm.state.value.chat.isEmpty())
        assertEquals(2, ai.resets)

        vm.submit("家計簿")
        runCurrent()
        assertEquals(VoiceError.Request(FailureKind.Offline, VoiceError.Origin.CHAT), vm.state.value.error)
        assertEquals(1, vm.state.value.chat.size) // 発話は残る(もう一度で送り直す)
        ai.chatTurns += Result.success(ChatTurn(kind = ChatTurn.Kind.PROPOSAL, text = "案です", options = listOf(option)))
        vm.retry()
        runCurrent()
        assertEquals(ChatTurn.Kind.PROPOSAL, vm.state.value.chatTurn?.kind)
        vm.choose(option)
        advanceUntilIdle()
        assertEquals(listOf("シンプルな家計簿"), ai.morphCalls)
    }

    @Test
    fun buildNowSendsGoAheadThenBuildsFromHistory() = runTest {
        val ai = FakeAI()
        ai.chatTurns += Result.success(ChatTurn(kind = ChatTurn.Kind.QUESTION, text = "誰と使いますか?"))
        ai.chatTurns += Result.failure(IOException("down"))
        val vm = vm(ai = ai, clarify = true)
        vm.submit("割り勘")
        runCurrent()
        vm.buildNow()
        advanceUntilIdle()
        // 返事が来なくても、これまでの発話(go-ahead 語は除く)で作る
        assertEquals("このまま作って", ai.chatCalls.last().last().text)
        assertEquals(listOf("割り勘"), ai.morphCalls)
    }

    @Test
    fun editAfterGenerateSkipsChat() = runTest {
        val ai = FakeAI()
        val vm = vm(ai = ai, clarify = true)
        ai.chatTurns += Result.success(ChatTurn(kind = ChatTurn.Kind.READY, text = "OK", transcript = "家計簿"))
        vm.submit("家計簿")
        runCurrent()
        vm.buildFromChat()
        advanceUntilIdle()
        assertFalse(vm.state.value.isInitial)

        vm.openVoice()
        assertTrue(vm.state.value.voiceOpen)
        vm.submit("ダークモードにして")
        runCurrent()
        assertFalse(vm.state.value.voiceOpen) // 生成が始まったらシートを閉じる
        advanceUntilIdle()
        assertEquals(listOf("家計簿", "ダークモードにして"), ai.morphCalls)
        assertEquals(1, ai.chatCalls.size)
    }

    @Test
    fun fallbackTranscriptSkipsAnsweredQuestionsAndGoAhead() = runTest {
        val vm = vm()
        val chat = listOf(
            ChatBubble(true, "家計簿"),
            ChatBubble(false, "何を?", ChatTurn.Kind.QUESTION),
            ChatBubble(true, "グラフは作れる?"),
            ChatBubble(false, "作れます", ChatTurn.Kind.ANSWER),
            ChatBubble(true, "支出だけ"),
            ChatBubble(true, "このまま作って"),
        )
        assertEquals("家計簿。支出だけ", vm.fallbackTranscript(chat))
    }

    // ランタイムとのつなぎ(#881)

    @Test
    fun adoptShowsTheAppTheRuntimeOpenedWithoutGenerating() = runTest {
        val ai = FakeAI()
        val vm = vm(ai = ai)
        vm.generateNow("家計簿")
        advanceUntilIdle()
        val other = generated.copy(appId = "app-2", version = 5)

        vm.adopt(other)

        assertEquals(other, vm.state.value.document)
        assertNull("前のアプリの要約は出さない", vm.state.value.summary)
        assertEquals(1, ai.morphCalls.size)
    }

    @Test
    fun resultIsDroppedWhenTheAppChangedWhileGenerating() = runTest {
        var current = "app-1"
        val ai = FakeAI().apply { gate = CompletableDeferred() }
        val vm = VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { current },
            initial = initial,
            clarifyEnabled = false,
            scope = CoroutineScope(StandardTestDispatcher(testScheduler) + Job()),
        )
        vm.generateNow("家計簿")
        runCurrent()
        current = "app-2" // 生成を待つ間に一覧から別のアプリへ
        ai.gate!!.complete(Unit)
        advanceUntilIdle()

        assertEquals(initial, vm.state.value.document)
        assertEquals(VoicePhase.IDLE, vm.state.value.phase)
    }
}
