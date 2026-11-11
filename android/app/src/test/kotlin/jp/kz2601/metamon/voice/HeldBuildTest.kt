package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.purchase.CreationGate
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ErrorCodes
import jp.kz2601.metamon.services.http.PaidFeature
import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import jp.kz2601.metamon.services.purchase.PaidFeatureCheck
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 新規作成の無料枠切れ・402 で止めた作成を、言い直さずに続きから作る(#1261・iOS の HeldBuildTests)。 */
@OptIn(ExperimentalCoroutinesApi::class)
class HeldBuildTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val voiceOnly = AppDocument(
        appId = "a1",
        version = 1,
        dsl = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"Tailor It","voiceEntry":"fullscreen"},"initialScreen":"voice",
               "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}""",
        ),
    )
    private val built = AppDocument(
        appId = "a1",
        version = 2,
        dsl = AppDSL.decode("""{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home","screens":[{"id":"home","components":[]}]}"""),
    )

    private class FakeAI(private val built: AppDocument) : AIClient {
        val morphCalls = mutableListOf<String>()
        val turns = ArrayDeque<ChatTurn>()

        /** 次の generate を 402 で止める回数 */
        var proRequired = 0

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
            if (proRequired > 0) {
                proRequired--
                val feature = JsonObject(mapOf("id" to JsonPrimitive("sharedData"), "title" to JsonPrimitive("共有")))
                throw ApiError.Server(402, ErrorCodes.PRO_REQUIRED, "pro", JsonObject(mapOf("features" to JsonArray(listOf(feature)))))
            }
            return MorphResult(document = built, summary = "できました", envelopeJson = "{}")
        }

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = turns.removeFirst()

        override suspend fun resetChat(appId: String) = Unit
    }

    /** 課金画面の代わり。[buy] が true なら「買った」 */
    private class FakeGate : CreationGate {
        var allowed = true
        var buy = false
        var admits = 0
        var recorded = 0
        val proRequests = mutableListOf<List<PaidFeature>>()
        override val dailyFreeLimit: Int = 3

        override suspend fun admitNewApp(): Boolean {
            admits++
            return allowed || buy
        }

        override fun recordNewApp() {
            recorded++
        }

        override suspend fun requestPro(features: List<PaidFeature>): Boolean {
            proRequests += features
            return buy
        }
    }

    private fun TestScope.vm(ai: FakeAI, gate: FakeGate, clarify: Boolean = true): VoiceViewModel {
        val dispatcher = StandardTestDispatcher(testScheduler)
        return VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { "a1" },
            initial = voiceOnly,
            clarifyEnabled = { clarify },
            chatHistory = ChatHistoryStore(tmp.root),
            io = dispatcher,
            creationGate = gate,
            scope = CoroutineScope(dispatcher + Job()),
        )
    }

    private fun ready(transcript: String) = ChatTurn(kind = ChatTurn.Kind.READY, text = "作ります", turn = 1, transcript = transcript)

    @Test
    fun limitReachedKeepsTheConversationAndContinues() = runTest {
        val ai = FakeAI(built).apply { turns += ready("家計簿を作って") }
        val gate = FakeGate().apply { allowed = false }
        val vm = vm(ai, gate)
        vm.submit("家計簿")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()

        assertTrue("送っていない", ai.morphCalls.isEmpty())
        assertEquals(HeldBuild.LimitReached("家計簿を作って", 3), vm.state.value.heldBuild)
        assertTrue("会話は残る", vm.state.value.isChatting)
        assertFalse(vm.state.value.isGenerating)

        // 買った(枠が戻った)ので続きから作る
        gate.buy = true
        vm.continueHeldBuild()
        advanceUntilIdle()
        assertEquals(listOf("家計簿を作って"), ai.morphCalls)
        assertNull(vm.state.value.heldBuild)
        assertEquals(built, vm.state.value.document)
        assertEquals("できた時だけ数える", 1, gate.recorded)
    }

    @Test
    fun limitReachedWithoutChatPutsTheRequestBackInTheInput() = runTest {
        val ai = FakeAI(built)
        val gate = FakeGate().apply { allowed = false }
        val vm = vm(ai, gate, clarify = false)
        vm.submit("家計簿を作って")
        advanceUntilIdle()
        assertTrue(ai.morphCalls.isEmpty())
        assertNull(vm.state.value.heldBuild)
        assertEquals("家計簿を作って", vm.state.value.draft)
        assertEquals(0, gate.recorded)
    }

    @Test
    fun speakingAgainClosesTheCard() = runTest {
        val ai = FakeAI(built).apply {
            turns += ready("家計簿を作って")
            turns += ready("家計簿を作って 2")
        }
        val gate = FakeGate().apply { allowed = false }
        val vm = vm(ai, gate)
        vm.submit("家計簿")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()
        assertTrue(vm.state.value.heldBuild != null)
        vm.submit("やっぱり色を青に")
        assertNull(vm.state.value.heldBuild)
    }

    @Test
    fun editsAreNotMetered() = runTest {
        val ai = FakeAI(built.copy(version = 3))
        val gate = FakeGate().apply { allowed = false }
        val vm = vm(ai, gate, clarify = false)
        vm.adopt(built)
        vm.submit("色を青に")
        advanceUntilIdle()
        assertEquals(0, gate.admits)
        assertEquals(listOf("色を青に"), ai.morphCalls)
        assertEquals(0, gate.recorded)
    }

    @Test
    fun proRequiredBuyResendsTheSameRequest() = runTest {
        val ai = FakeAI(built).apply {
            turns += ready("みんなで使う掲示板")
            proRequired = 1
        }
        val gate = FakeGate().apply { buy = true }
        val vm = vm(ai, gate)
        vm.submit("掲示板")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()
        assertEquals(listOf("sharedData"), gate.proRequests.single().map { it.id })
        assertEquals(listOf("みんなで使う掲示板", "みんなで使う掲示板"), ai.morphCalls)
        assertEquals(built, vm.state.value.document)
    }

    @Test
    fun proRequiredClosedShowsThePaidCard() = runTest {
        val ai = FakeAI(built).apply {
            turns += ready("みんなで使う掲示板")
            proRequired = 1
        }
        val gate = FakeGate()
        val vm = vm(ai, gate)
        vm.submit("掲示板")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()
        val held = vm.state.value.heldBuild as HeldBuild.ProRequired
        assertEquals("みんなで使う掲示板", held.text)
        assertNull("状態カードは出さない", vm.state.value.error)
        assertTrue(vm.state.value.isChatting)

        // この機能なしで作る
        vm.buildWithoutPaidFeatures()
        advanceUntilIdle()
        assertEquals("みんなで使う掲示板\n" + PaidFeatureCheck.FALLBACK_FREE_SUFFIX, ai.morphCalls.last())
        assertEquals(built, vm.state.value.document)
    }

    @Test
    fun proRequiredAgainAfterBuyingShowsTheErrorInsteadOfLooping() = runTest {
        val ai = FakeAI(built).apply {
            turns += ready("みんなで使う掲示板")
            proRequired = 2
        }
        val gate = FakeGate().apply { buy = true }
        val vm = vm(ai, gate)
        vm.submit("掲示板")
        advanceUntilIdle()
        vm.buildFromChat()
        advanceUntilIdle()
        assertEquals(1, gate.proRequests.size)
        assertEquals(2, ai.morphCalls.size)
        assertTrue(vm.state.value.error != null)
    }
}
