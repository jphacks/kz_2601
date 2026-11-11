package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 変更の相談で「何も変わらなかった」時は、会話を残して理由を足し、シートを開き直す(#1177・iOS の `presentUnchanged`)。 */
@OptIn(ExperimentalCoroutinesApi::class)
class ChatUnchangedTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val built = AppDocument(
        appId = "a1",
        version = 2,
        dsl = AppDSL.decode("""{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home","screens":[{"id":"home","components":[]}]}"""),
    )

    private class FakeAI(private val result: MorphResult) : AIClient {
        val morphCalls = mutableListOf<String>()
        val turns = ArrayDeque<ChatTurn>()
        var resets = 0

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
            return result
        }

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = turns.removeFirst()

        override suspend fun resetChat(appId: String) {
            resets++
        }
    }

    private fun TestScope.vm(ai: FakeAI, clarify: Boolean = true): VoiceViewModel {
        val dispatcher = StandardTestDispatcher(testScheduler)
        return VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { "a1" },
            initial = built,
            clarifyEnabled = { clarify },
            chatHistory = ChatHistoryStore(tmp.root),
            io = dispatcher,
            unchangedFallback = "FALLBACK",
            scope = CoroutineScope(dispatcher + Job()),
        )
    }

    private fun ready(transcript: String) = ChatTurn(kind = ChatTurn.Kind.READY, text = "変えます", turn = 1, transcript = transcript)

    private fun unchanged(summary: String?) = MorphResult(document = built, summary = summary, envelopeJson = "{}", unchanged = true)

    @Test
    fun unchangedEditFromChatKeepsTheConversationAndAddsTheReason() = runTest {
        val ai = FakeAI(unchanged("その画面はありません")).apply { turns += ready("設定の画面を青に") }
        val vm = vm(ai)
        vm.submit("設定を青に")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()

        val s = vm.state.value
        assertEquals(listOf("設定の画面を青に"), ai.morphCalls)
        assertEquals(
            listOf(true to "設定を青に", false to "変えます", false to "その画面はありません"),
            s.chat.map { it.fromUser to it.text },
        )
        assertTrue("シートを開き直す", s.voiceOpen)
        assertNull(s.chatTurn)
        assertTrue("反映の区切りは入れない", s.history.isEmpty())
        assertNull("バナーは出さない", s.summary)
        assertFalse(s.canUndo)
        assertFalse(s.isGenerating)
        assertEquals(built, s.document)
        assertEquals("会話は続くのでサーバーの会話も消さない", 0, ai.resets)
    }

    @Test
    fun unchangedWithoutReasonUsesTheFallback() = runTest {
        val ai = FakeAI(unchanged(" ")).apply { turns += ready("設定の画面を青に") }
        val vm = vm(ai)
        vm.submit("設定を青に")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()
        assertEquals("FALLBACK", vm.state.value.chat.last().text)
    }

    @Test
    fun theConversationContinuesAfterUnchanged() = runTest {
        val ai = FakeAI(unchanged("その画面はありません")).apply {
            turns += ready("設定の画面を青に")
            turns += ChatTurn(kind = ChatTurn.Kind.QUESTION, text = "どの画面ですか", turn = 2)
        }
        val vm = vm(ai)
        vm.submit("設定を青に")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()
        vm.submit("ホームの画面です")
        advanceUntilIdle()
        assertEquals("どの画面ですか", vm.state.value.chat.last().text)
        assertEquals(5, vm.state.value.chat.size)
    }

    @Test
    fun unchangedWithoutChatShowsTheReasonInTheBanner() = runTest {
        val ai = FakeAI(unchanged("その画面はありません"))
        val vm = vm(ai, clarify = false)
        vm.submit("設定を青に")
        advanceUntilIdle()
        val s = vm.state.value
        assertEquals("その画面はありません", s.summary)
        assertNull("元に戻すは出さない", s.summaryAction)
        assertTrue(s.chat.isEmpty())
        assertTrue(s.history.isEmpty())
    }
}
