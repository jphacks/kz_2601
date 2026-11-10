package jp.kz2601.metamon.services.speech

import jp.kz2601.metamon.services.mock.MockSpeechTranscriber
import jp.kz2601.metamon.services.speech.SpeechErrorMapping.failure
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

class SpeechErrorMappingTest {
    @Test
    fun permissionAndAudio() {
        assertEquals(TranscriberFailure.MicrophoneDenied, failure(SpeechErrorMapping.ERROR_INSUFFICIENT_PERMISSIONS, 2.0, true))
        assertEquals(TranscriberFailure.NoInputAvailable, failure(SpeechErrorMapping.ERROR_AUDIO, 2.0, true))
        assertEquals(TranscriberFailure.NoInputAvailable, failure(SpeechErrorMapping.ERROR_RECOGNIZER_BUSY, 0.0, false))
    }

    @Test
    fun noSpeechIsTooShortThenSilentThenNotHeard() {
        // iOS と同じ順: 短すぎ → 無音 → 聞き取れない
        assertEquals(TranscriberFailure.TooShort(0.3), failure(SpeechErrorMapping.ERROR_NO_MATCH, 0.3, true))
        assertEquals(TranscriberFailure.TooShort(0.1), failure(SpeechErrorMapping.ERROR_CLIENT, 0.1, false))
        assertEquals(TranscriberFailure.Silent(5.0), failure(SpeechErrorMapping.ERROR_SPEECH_TIMEOUT, 5.0, false))
        assertEquals(TranscriberFailure.Silent(2.0), failure(SpeechErrorMapping.ERROR_NO_MATCH, 2.0, false))
        assertEquals(TranscriberFailure.NotHeard, failure(SpeechErrorMapping.ERROR_NO_MATCH, 2.0, true))
        assertEquals(TranscriberFailure.NotHeard, SpeechErrorMapping.emptyResult(SpeechErrorMapping.MINIMUM_SECONDS, true))
    }

    @Test
    fun languageAndNetworkNeverFallBackToServer() {
        assertEquals(TranscriberFailure.OnDeviceRecognitionUnavailable, failure(SpeechErrorMapping.ERROR_LANGUAGE_NOT_SUPPORTED, 1.0, true))
        assertEquals(TranscriberFailure.NotReady, failure(SpeechErrorMapping.ERROR_LANGUAGE_UNAVAILABLE, 1.0, true))
        listOf(
            SpeechErrorMapping.ERROR_NETWORK,
            SpeechErrorMapping.ERROR_NETWORK_TIMEOUT,
            SpeechErrorMapping.ERROR_SERVER,
            SpeechErrorMapping.ERROR_SERVER_DISCONNECTED,
        ).forEach { assertEquals(TranscriberFailure.OnDeviceRecognitionUnavailable, failure(it, 1.0, true)) }
        assertEquals(TranscriberFailure.Other(99), failure(99, 1.0, true))
    }

    @Test
    fun languageTags() {
        assertEquals("ja-JP", speechLanguageTag("ja"))
        assertEquals("en-US", speechLanguageTag("en"))
        assertEquals("ja-JP", speechLanguageTag("fr"))
        assertTrue(SpeechErrorMapping.containsLanguage(listOf("en-US", "ja-JP"), "ja-JP"))
        assertTrue(SpeechErrorMapping.containsLanguage(listOf("ja"), "ja-JP"))
        assertTrue(SpeechErrorMapping.containsLanguage(listOf("en_GB"), "en-US"))
        assertFalse(SpeechErrorMapping.containsLanguage(listOf("en-US"), "ja-JP"))
    }

    @Test
    fun mockTranscriber() = runTest {
        val mock = MockSpeechTranscriber(results = listOf(Result.success(" 筋トレの記録 "), Result.success("")))
        mock.startRecording()
        assertTrue(mock.listening.value)
        assertEquals("筋トレの記録", mock.stopAndTranscribe())
        assertFalse(mock.listening.value)
        mock.startRecording()
        try {
            mock.stopAndTranscribe()
            fail("空は NotHeard")
        } catch (e: TranscriberFailure) {
            assertEquals(TranscriberFailure.NotHeard, e)
        }
        val unavailable = MockSpeechTranscriber(status = TranscriberStatus.Unavailable("x"))
        assertFalse(unavailable.isAvailable)
        try {
            unavailable.startRecording()
            fail()
        } catch (e: TranscriberFailure) {
            assertEquals(TranscriberFailure.OnDeviceRecognitionUnavailable, e)
        }
    }
}
