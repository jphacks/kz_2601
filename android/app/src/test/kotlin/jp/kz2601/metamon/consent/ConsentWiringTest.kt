package jp.kz2601.metamon.consent

import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.renderer.components.ai.AiButtonMetric
import jp.kz2601.metamon.runtime.AiTextAccess
import jp.kz2601.metamon.runtime.AppAiHost
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.consent.AiButtonMetricsRecorder
import jp.kz2601.metamon.services.consent.AiConsentRequired
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.consent.ConsentGatedAiClient
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/** 同意の保存 → AiTextGate(aiButton)・生成の砦・同意の画面のつなぎ(#888)。 */
class ConsentWiringTest {
    private val consent = AiConsentStore(InMemoryKeyValueStore())
    private val prompt = ConsentPrompt(consent)

    /** 送った回数だけ数える ai(送ってはいけない時に呼ばれたら分かる) */
    private class CountingAi : AIClient {
        var texts = 0
        var morphs = 0
        override suspend fun morph(transcript: String, appId: String, currentVersion: Int, locale: String?, mode: GenerateMode): MorphResult {
            morphs++
            throw UnsupportedOperationException()
        }

        override suspend fun morphStreaming(
            transcript: String,
            appId: String,
            currentVersion: Int,
            locale: String?,
            mode: GenerateMode,
            progress: (GenerateProgress) -> Unit,
        ): MorphResult = morph(transcript, appId, currentVersion, locale, mode)

        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn =
            throw UnsupportedOperationException()

        override suspend fun resetChat(appId: String) = Unit

        override suspend fun text(request: AiTextRequest): AiTextResult {
            texts++
            return AiTextResult(AiTextShape.LINE, "ok")
        }
    }

    private val base = CountingAi()
    private val ai = ConsentGatedAiClient(base) { consent.isGranted }
    private val host = AppAiHost(
        ai = ai,
        appId = { "app1" },
        access = { AiTextAccess(consentGranted = consent.isGranted, isPro = false, enforced = false) },
        onNeedsConsent = { prompt.require() },
    )
    private val request = AiTextRequest(instruction = "", shape = AiTextShape.LINE, input = "x", locale = "ja", task = "summarize")

    @Test
    fun aiButtonAsksForConsentAndSendsNothingUntilGranted() = runBlocking {
        try {
            host.generateText(request)
            fail("同意が無いのに送った")
        } catch (e: AiTextError) {
            assertEquals(AiTextError.NeedsConsent, e)
        }
        assertEquals(0, base.texts)
        assertTrue("同意の画面を出す", prompt.visible.value)

        prompt.agree()
        assertFalse(prompt.visible.value)
        assertEquals("ok", host.generateText(request).text)
        assertEquals(1, base.texts)
    }

    @Test
    fun revokingStopsAiButtonAndGeneration() = runBlocking {
        consent.grant()
        host.generateText(request)
        consent.revoke()
        try {
            host.generateText(request)
            fail()
        } catch (e: AiTextError) {
            assertEquals(AiTextError.NeedsConsent, e)
        }
        try {
            ai.morph("x", "app1", 0)
            fail()
        } catch (_: AiConsentRequired) {
        }
        assertEquals(1, base.texts)
        assertEquals(0, base.morphs)
    }

    @Test
    fun heldUtteranceIsSentAfterAgreeButDroppedOnLater() {
        var sent = 0
        assertFalse(prompt.require { sent++ })
        prompt.later()
        assertEquals(0, sent)
        assertFalse(prompt.visible.value)

        assertFalse(prompt.require { sent++ })
        prompt.agree()
        assertEquals(1, sent)
        assertTrue(prompt.require { sent++ }) // 同意済みはそのまま送る(画面は出さない)
        assertFalse(prompt.visible.value)
    }

    @Test
    fun launchPromptOnlyFirstTime() {
        prompt.showOnLaunchIfNeeded()
        assertTrue(prompt.visible.value)
        prompt.later()
        prompt.showOnLaunchIfNeeded()
        assertFalse(prompt.visible.value)
    }

    @Test
    fun rendererMetricsAreRecorded() {
        var now = Instant.parse("2026-11-10T03:00:00Z")
        val recorder = AiButtonMetricsRecorder(InMemoryKeyValueStore(), clock = { now }, zone = { ZoneId.of("Asia/Tokyo") })
        recorder.record(AiButtonMetric.Shown("modelNotReady"))
        recorder.record(AiButtonMetric.Ran("classify", "invalidResponse"))
        now = now.plusSeconds(24 * 3600)
        val day = recorder.pendingReport()!!.days.single()
        assertEquals("model_not_ready", day.availability)
        assertEquals(mapOf("invalid_response" to 1), day.tasks.single().failures)
    }

    @Test
    fun resetDropsTheHeldUtteranceSoTheNextPersonsConsentDoesNotSendIt() {
        var sent = 0
        assertFalse(prompt.require { sent++ })
        assertTrue(prompt.visible.value)

        prompt.reset() // サインアウトの後片付け
        assertFalse(prompt.visible.value)
        prompt.agree() // 次の人が同意
        assertEquals("前の人の止めた発話を送らない", 0, sent)
    }
}
