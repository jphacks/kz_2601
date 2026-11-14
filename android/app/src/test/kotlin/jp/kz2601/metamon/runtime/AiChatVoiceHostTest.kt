package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.renderer.components.audio.AudioClaims
import jp.kz2601.metamon.renderer.components.audio.DictationException
import jp.kz2601.metamon.renderer.components.audio.DictationFailure
import jp.kz2601.metamon.renderer.components.audio.DictationHost
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import jp.kz2601.metamon.renderer.components.ai.DictationReadiness as AiReadiness
import jp.kz2601.metamon.renderer.components.audio.DictationReadiness as AudioReadiness

/** aiChat の話して入力と読み上げ(#1168): 録音の開始・終了、権限、マイク FAB との取り合い、読み上げの寿命。 */
class AiChatVoiceHostTest {
    private class FakeDictation : DictationHost {
        override val readiness = MutableStateFlow(AudioReadiness.READY)
        override val isVoiceInputActive = MutableStateFlow(false)
        override val listening: StateFlow<Boolean> = MutableStateFlow(false)
        var started = 0
        var cancelled = 0
        var result = "こんにちは"
        var startFailure: Exception? = null

        override fun prepareDictation() = Unit
        override suspend fun startDictation() {
            startFailure?.let { throw it }
            started++
        }

        override suspend fun finishDictation() = result
        override fun cancelDictation() {
            cancelled++
        }
    }

    private class FakeEngine : AiChatSpeechEngine {
        val spoken = mutableListOf<Triple<String, String, String>>()
        var stops = 0
        var shutdowns = 0
        lateinit var finish: (String) -> Unit
        var max = Int.MAX_VALUE
        /** 次の読み上げを受け付けない(TextToSpeech.speak が ERROR)。実機のエンジンと同じく、その場で終わりを知らせる */
        var rejectNext = false
        override val maxInputLength: Int get() = max
        override fun setOnFinished(listener: (utteranceId: String) -> Unit) {
            finish = listener
        }

        override fun speak(text: String, languageTag: String, utteranceId: String) {
            spoken += Triple(text, languageTag, utteranceId)
            if (rejectNext) {
                rejectNext = false
                finish(utteranceId)
            }
        }

        override fun stop() {
            stops++
        }

        override fun shutdown() {
            shutdowns++
        }
    }

    private class ClaimsGate(val claims: AudioClaims = AudioClaims()) : AiChatAudioGate {
        override fun claim(owner: String, use: AudioClaims.Use, stop: () -> Unit) = claims.claim(owner, use, stop)
        override fun release(owner: String) = claims.release(owner)
    }

    private val dictation = FakeDictation()
    private val engine = FakeEngine()
    private val gate = ClaimsGate()
    private var language = "ja"
    private var micGranted = true
    private var micAsked = 0
    private var othersStopped = 0
    private val host = AiChatVoiceHost(
        dictation, engine, gate, CoroutineScope(Dispatchers.Unconfined),
        uiLanguage = { language },
        requestMic = { micAsked++; micGranted },
        stopOthers = { othersStopped++ },
    )

    @Test
    fun readinessHidesMicWhenOnDeviceRecognitionIsUnavailable() {
        assertEquals(AiReadiness.READY, host.readiness.value)
        dictation.readiness.value = AudioReadiness.NEEDS_MODEL
        assertEquals(AiReadiness.NEEDS_MODEL, host.readiness.value)
        dictation.readiness.value = AudioReadiness.PREPARING
        assertEquals(AiReadiness.PREPARING, host.readiness.value)
        dictation.readiness.value = AudioReadiness.UNAVAILABLE
        assertNull(host.readiness.value)
    }

    @Test
    fun startAndFinishTakesAndReleasesTheMic() = runTest {
        host.startDictation()
        assertEquals(1, dictation.started)
        assertEquals(1, micAsked)
        assertEquals(AudioClaims.Use.DICTATION, gate.claims.current)
        assertEquals("こんにちは", host.finishDictation())
        assertNull(gate.claims.current)
    }

    @Test
    fun cancelReleasesTheMic() = runTest {
        host.startDictation()
        host.cancelDictation()
        assertEquals(1, dictation.cancelled)
        assertNull(gate.claims.current)
        host.cancelDictation() // 録音していなければ何もしない
        assertEquals(1, dictation.cancelled)
    }

    @Test
    fun deniedMicDoesNotRecord() = runTest {
        micGranted = false
        val failure = runCatching { host.startDictation() }.exceptionOrNull()
        assertEquals(DictationFailure.MIC_DENIED, (failure as DictationException).failure)
        assertEquals(0, dictation.started)
        assertNull(gate.claims.current)
    }

    @Test
    fun voiceInputSheetBlocksRecording() = runTest {
        dictation.isVoiceInputActive.value = true
        assertTrue(host.voiceInputActive.value)
        try {
            host.startDictation()
            fail("should not start while the mic FAB voice input is active")
        } catch (e: DictationException) {
            assertEquals(DictationFailure.FAILED, e.failure)
        }
        assertEquals(0, dictation.started)
        assertEquals(0, micAsked)
    }

    @Test
    fun recognizerFailureReleasesTheMic() = runTest {
        dictation.startFailure = DictationException(DictationFailure.NOT_READY)
        val failure = runCatching { host.startDictation() }.exceptionOrNull()
        assertEquals(DictationFailure.NOT_READY, (failure as DictationException).failure)
        assertNull(gate.claims.current)
        host.speak("読める", "o/1") // 録音中の扱いが残っていない
        assertEquals("o/1", host.speakingOwner.value)
    }

    @Test
    fun recordingStopsSpeechAndBlocksNewSpeech() = runTest {
        host.speak("前の返事", "o/1")
        assertEquals("o/1", host.speakingOwner.value)
        host.startDictation()
        assertNull(host.speakingOwner.value)
        assertEquals(1, engine.stops)
        assertEquals(1, othersStopped)
        host.speak("録音中の返事", "o/2")
        assertNull(host.speakingOwner.value)
        assertEquals(1, engine.spoken.size)
    }

    @Test
    fun speakFollowsUiLanguageAndFinishes() {
        host.speak("  こんにちは ", "o/1")
        assertEquals(Triple("こんにちは", "ja-JP", "o/1"), engine.spoken.last())
        engine.finish("other") // 別の読み上げの終わりは無視
        assertEquals("o/1", host.speakingOwner.value)
        engine.finish("o/1")
        assertNull(host.speakingOwner.value)
        assertNull(gate.claims.current)
        language = "en"
        host.speak("hello", "o/2")
        assertEquals("en-US", engine.spoken.last().second)
        host.speak("   ", "o/3")
        assertEquals(2, engine.spoken.size)
    }

    @Test
    fun stopOnlyForTheSpeakingOwner() {
        host.speak("a", "o/1")
        host.stop("o/2")
        assertEquals("o/1", host.speakingOwner.value)
        host.stop("o/1")
        assertNull(host.speakingOwner.value)
        assertEquals(1, engine.stops)
    }

    @Test
    fun anotherAudioOwnerStopsSpeech() {
        host.speak("a", "o/1")
        gate.claims.claim("leaf-speech", AudioClaims.Use.SPEECH) {}
        assertNull(host.speakingOwner.value)
    }

    @Test
    fun releaseStopsAndShutsDownOnce() = runTest {
        host.speak("a", "o/1")
        host.release()
        assertNull(host.speakingOwner.value)
        assertEquals(1, engine.shutdowns)
        host.release()
        assertEquals(1, engine.shutdowns)
        host.speak("b", "o/2")
        assertEquals(1, engine.spoken.size)
        assertFalse(runCatching { host.startDictation() }.isSuccess)
    }

    @Test
    fun releaseWhileRecordingCancels() = runTest {
        host.startDictation()
        host.release()
        assertEquals(1, dictation.cancelled)
        assertNull(gate.claims.current)
    }

    @Test
    fun longTextIsClippedToTheEngineLimit() {
        engine.max = 5
        host.speak("  あいうえおかきくけこ ", "o/1")
        assertEquals("あいうえお", engine.spoken.last().first)
    }

    @Test
    fun rejectedSpeechFreesTheSpeakerAtOnce() {
        engine.rejectNext = true
        host.speak("a", "o/1")
        assertNull("受け付けられなかった読み上げは読み上げ中のまま残らない", host.speakingOwner.value)
        assertNull(gate.claims.current)
        host.speak("b", "o/2") // 次はふつうに読める
        assertEquals("o/2", host.speakingOwner.value)
    }

    @Test
    fun clipDoesNotSplitASurrogatePair() {
        assertEquals("ab", AiChatTts.clip("ab\uD83D\uDE00c", 3))
        assertEquals("ab\uD83D\uDE00", AiChatTts.clip("ab\uD83D\uDE00c", 4))
        assertEquals("abc", AiChatTts.clip("abc", 3))
        assertEquals("abc", AiChatTts.clip("abc", 0)) // 上限が分からない時は切らない
    }

    @Test
    fun missingLanguageFallsBackToTheDefault() {
        val tried = mutableListOf<String>()
        // -1 = LANG_MISSING_DATA・-2 = LANG_NOT_SUPPORTED・0 以上 = 使える
        fun results(vararg pairs: Pair<String, Int>): (String) -> Int = { l -> tried += l; pairs.toMap().getValue(l) }
        assertEquals("ja", AiChatTts.applyLanguage("ja", "en", results("ja" to 0, "en" to 0)))
        assertEquals(listOf("ja"), tried)
        tried.clear()
        assertEquals("en", AiChatTts.applyLanguage("ja", "en", results("ja" to -1, "en" to 1)))
        assertEquals(listOf("ja", "en"), tried)
        assertEquals("en", AiChatTts.applyLanguage("ja", "en", results("ja" to -2, "en" to 0)))
        assertNull(AiChatTts.applyLanguage("ja", "en", results("ja" to -2, "en" to -1)))
        tried.clear()
        assertNull("既定と同じ言語は試し直さない", AiChatTts.applyLanguage("ja", "ja", results("ja" to -1)))
        assertEquals(listOf("ja"), tried)
    }
}
