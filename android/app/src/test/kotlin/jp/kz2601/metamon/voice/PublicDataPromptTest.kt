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
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** #899(iOS の PublicDataPromptTests): 生成で個人の情報が公開の記録に入った時だけ確認を出し、ボタンは修正の指示文を送る。 */
@OptIn(ExperimentalCoroutinesApi::class)
class PublicDataPromptTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val dsl = AppDSL.decode("""{"schemaVersion":1,"app":{"name":"予約"},"initialScreen":"home","screens":[{"id":"home","components":[]}]}""")
    private val phone = Json.parseToJsonElement(
        """{"entity":"reservations","entityName":"予約","field":"phone","fieldName":"電話番号","kind":"phone"}""",
    ) as JsonObject

    private class FakeAI(private val personal: List<JsonObject>, private val dsl: AppDSL) : AIClient {
        val sent = mutableListOf<String>()

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
            sent += transcript
            // 最初の生成だけ個人の情報が公開に入る(直した後は入らない)
            val fields = if (sent.size == 1) personal else emptyList()
            return MorphResult(AppDocument("a1", currentVersion + 1, dsl), summary = "できました", publicPersonalData = fields, envelopeJson = "{}")
        }

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = error("not used")

        override suspend fun resetChat(appId: String) {}
    }

    private fun TestScope.vm(ai: FakeAI): VoiceViewModel {
        val dispatcher = StandardTestDispatcher(testScheduler)
        return VoiceViewModel(
            transcriber = MockSpeechTranscriber(),
            ai = ai,
            appId = { "a1" },
            initial = AppDocument("a1", 1, dsl),
            clarifyEnabled = { false },
            chatHistory = ChatHistoryStore(tmp.root),
            io = dispatcher,
            scope = CoroutineScope(dispatcher + Job()),
        )
    }

    @Test
    fun promptOnlyWhenServerReportsPublicPersonalData() = runTest {
        val none = FakeAI(emptyList(), dsl)
        val v0 = vm(none)
        v0.generateNow("家計簿")
        advanceUntilIdle()
        assertNull("旧サーバー・個人の情報が無い時は出さない", v0.state.value.publicDataPrompt)

        val vm = vm(FakeAI(listOf(phone), dsl))
        vm.generateNow("美容室の予約")
        advanceUntilIdle()
        val p = assertNotNullAnd(vm.state.value.publicDataPrompt)
        assertEquals("電話番号", p.fieldNames())
        assertEquals(vm.state.value.document.version, p.version)
        assertEquals("要約はそのまま", "できました", vm.state.value.summary)
    }

    @Test
    fun keepPublicSendsNothing() = runTest {
        val ai = FakeAI(listOf(phone), dsl)
        val vm = vm(ai)
        vm.generateNow("美容室の予約")
        advanceUntilIdle()
        vm.keepPublic()
        advanceUntilIdle()
        assertNull(vm.state.value.publicDataPrompt)
        assertEquals(listOf("美容室の予約"), ai.sent)
    }

    @Test
    fun removeAndPrivateSendEditInstructions() = runTest {
        val ai = FakeAI(listOf(phone), dsl)
        val vm = vm(ai)
        vm.generateNow("美容室の予約")
        advanceUntilIdle()
        vm.removePublicPersonalFields(english = false)
        advanceUntilIdle()
        assertEquals("予約の「電話番号」の欄を消して", ai.sent.last())
        assertNull("直した後の生成では出ない", vm.state.value.publicDataPrompt)

        val ai2 = FakeAI(listOf(phone), dsl)
        val vm2 = vm(ai2)
        vm2.generateNow("美容室の予約")
        advanceUntilIdle()
        vm2.makePublicDataPrivate(english = true)
        advanceUntilIdle()
        assertEquals("Keep 予約 to only me (make it private)", ai2.sent.last())
    }

    @Test
    fun staleVersionSendsNothing() = runTest {
        val ai = FakeAI(listOf(phone), dsl)
        val vm = vm(ai)
        vm.generateNow("美容室の予約")
        advanceUntilIdle()
        vm.adopt(vm.state.value.document.copy(version = 99)) // 別の版に変わった
        vm.removePublicPersonalFields(english = false)
        advanceUntilIdle()
        assertEquals(listOf("美容室の予約"), ai.sent)
        assertNull(vm.state.value.publicDataPrompt)
    }

    private fun <T : Any> assertNotNullAnd(v: T?): T {
        assertNotNull(v)
        return v!!
    }
}
