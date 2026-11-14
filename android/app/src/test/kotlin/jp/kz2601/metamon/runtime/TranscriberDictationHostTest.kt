package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.renderer.components.audio.DictationFailure
import jp.kz2601.metamon.renderer.components.audio.DictationReadiness
import jp.kz2601.metamon.services.TranscriberFailure
import jp.kz2601.metamon.services.TranscriberStatus
import org.junit.Assert.assertEquals
import org.junit.Test

/** transcribeField の話して入力(#864): 認識器の状態・失敗 → 部品の表示。 */
class TranscriberDictationHostTest {
    @Test
    fun readiness() {
        assertEquals(DictationReadiness.READY, TranscriberDictationHost.readinessOf(TranscriberStatus.Ready))
        assertEquals(DictationReadiness.READY, TranscriberDictationHost.readinessOf(TranscriberStatus.Recognizing))
        assertEquals(DictationReadiness.PREPARING, TranscriberDictationHost.readinessOf(TranscriberStatus.Downloading))
        assertEquals(DictationReadiness.NEEDS_MODEL, TranscriberDictationHost.readinessOf(TranscriberStatus.NotDownloaded))
        assertEquals(DictationReadiness.UNAVAILABLE, TranscriberDictationHost.readinessOf(TranscriberStatus.Unavailable("x")))
    }

    @Test
    fun failures() {
        assertEquals(DictationFailure.MIC_DENIED, TranscriberDictationHost.failureOf(TranscriberFailure.MicrophoneDenied))
        assertEquals(DictationFailure.NOT_READY, TranscriberDictationHost.failureOf(TranscriberFailure.NotReady))
        assertEquals(DictationFailure.NOT_HEARD, TranscriberDictationHost.failureOf(TranscriberFailure.TooShort(0.2)))
        assertEquals(DictationFailure.NOT_HEARD, TranscriberDictationHost.failureOf(TranscriberFailure.Silent(3.0)))
        assertEquals(DictationFailure.NOT_HEARD, TranscriberDictationHost.failureOf(TranscriberFailure.NotHeard))
        assertEquals(DictationFailure.UNAVAILABLE, TranscriberDictationHost.failureOf(TranscriberFailure.OnDeviceRecognitionUnavailable))
        assertEquals(DictationFailure.FAILED, TranscriberDictationHost.failureOf(TranscriberFailure.NoInputAvailable))
        assertEquals(DictationFailure.FAILED, TranscriberDictationHost.failureOf(TranscriberFailure.Other(5)))
    }
}
