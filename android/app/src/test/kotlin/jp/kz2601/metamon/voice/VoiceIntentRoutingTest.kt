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
import kotlinx.coroutines.Job
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** 発話の意図を AI に送らず画面へ回す(#884・iOS の `AppRuntime.morph` の分岐)。 */
class VoiceIntentRoutingTest {
    private fun appDsl(name: String, voiceOnly: Boolean = false): AppDSL = AppDSL.decode(
        if (voiceOnly) {
            """{"schemaVersion":1,"app":{"name":"$name","voiceEntry":"fullscreen"},"initialScreen":"voice",
               "screens":[{"id":"voice","components":[{"type":"voiceInput"}]}]}"""
        } else {
            """{"schemaVersion":1,"app":{"name":"$name"},"initialScreen":"home",
               "screens":[{"id":"home","title":"$name","components":[]}]}"""
        },
    )

    private inner class FakeAI : AIClient {
        val sent = mutableListOf<Pair<String, String>>()
        var version = 0

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
            sent += transcript to appId
            return MorphResult(AppDocument(appId, ++version, appDsl(transcript)), summary = "ok", envelopeJson = "{}")
        }

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = error("not used")

        override suspend fun resetChat(appId: String) = Unit
    }

    private class FakeRequests(private val fresh: () -> AppDocument) : VoiceAppRequests {
        val calls = mutableListOf<String>()
        override fun openAppList() {
            calls += "list"
        }

        override fun switchApp(name: String) {
            calls += "switch:$name"
        }

        override fun startNewApp(): AppDocument {
            calls += "new"
            return fresh()
        }
    }

    private var currentAppId = "app-1"

    private fun TestScope.vm(ai: FakeAI, requests: VoiceAppRequests?) = VoiceViewModel(
        transcriber = MockSpeechTranscriber(),
        ai = ai,
        appId = { currentAppId },
        initial = AppDocument("app-1", 0, appDsl("Tailor It", voiceOnly = true)),
        clarifyEnabled = { false },
        appRequests = requests,
        scope = CoroutineScope(StandardTestDispatcher(testScheduler) + Job()),
    )

    @Test
    fun listAndSwitchGoToTheScreenNotTheAI() = runTest {
        val ai = FakeAI()
        val requests = FakeRequests { error("not used") }
        val vm = vm(ai, requests)
        vm.submit("アプリ一覧", InputSource.VOICE)
        vm.submit("家計簿に切り替えて", InputSource.VOICE)
        vm.submit("Switch to another app", InputSource.TEXT)
        advanceUntilIdle()
        assertEquals(listOf("list", "switch:家計簿", "list"), requests.calls)
        assertTrue("AI には送らない", ai.sent.isEmpty())
    }

    @Test
    fun undoUsesTheVersionHistory() = runTest {
        val ai = FakeAI()
        val vm = vm(ai, FakeRequests { error("not used") })
        vm.generateNow("家計簿")
        advanceUntilIdle()
        vm.generateNow("家計簿・グラフ付き")
        advanceUntilIdle()
        vm.submit("元に戻して", InputSource.VOICE)
        advanceUntilIdle()
        assertEquals("家計簿", vm.state.value.document.dsl.app.name)
        assertEquals(2, ai.sent.size)
    }

    @Test
    fun newAppStartsFreshThenSendsTheRequest() = runTest {
        val ai = FakeAI()
        val requests = FakeRequests {
            currentAppId = "app-2"
            AppDocument("app-2", 0, appDsl("Tailor It", voiceOnly = true))
        }
        val vm = vm(ai, requests)
        vm.generateNow("家計簿")
        advanceUntilIdle()
        vm.submit("別のアプリ。散歩メモが欲しい", InputSource.VOICE)
        advanceUntilIdle()
        assertEquals(listOf("new"), requests.calls)
        assertEquals("別のアプリ。散歩メモが欲しい" to "app-2", ai.sent.last())
        assertEquals("app-2", vm.state.value.document.appId)
    }

    @Test
    fun withoutRequestsOnlyUndoIsCaught() = runTest {
        val ai = FakeAI()
        val vm = vm(ai, null)
        vm.submit("家計簿に切り替えて", InputSource.VOICE)
        advanceUntilIdle()
        assertEquals(listOf("家計簿に切り替えて" to "app-1"), ai.sent)
    }
}
