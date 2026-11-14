package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.renderer.components.importing.AudioModelFailure
import jp.kz2601.metamon.renderer.components.importing.AudioModelReadiness
import jp.kz2601.metamon.whisper.ModelFailure
import jp.kz2601.metamon.whisper.ModelState
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

/** #1180 録音ファイルの文字起こし: モデルの状態 → fileImport の表示。 */
class WhisperAudioFileHostTest {
    @Test
    fun modelStateMapsToTheComponentsReadiness() {
        fun r(s: ModelState) = WhisperAudioFileHost.readinessOf(s, supported = true)
        assertEquals(AudioModelReadiness.NeedsModel, r(ModelState.Absent))
        assertEquals(AudioModelReadiness.Preparing(0.4f), r(ModelState.Downloading(0.4f)))
        assertEquals(AudioModelReadiness.Preparing(null), r(ModelState.Verifying))
        assertEquals(AudioModelReadiness.Ready, r(ModelState.Ready(File("m.bin"))))
        assertEquals(AudioModelReadiness.Failed(AudioModelFailure.NETWORK), r(ModelState.Failed(ModelFailure.Network)))
    }

    @Test
    fun unsupportedDeviceIsUnavailableWhateverTheModel() {
        assertEquals(AudioModelReadiness.Unavailable, WhisperAudioFileHost.readinessOf(ModelState.Ready(File("m.bin")), supported = false))
        assertEquals(AudioModelReadiness.Unavailable, WhisperAudioFileHost.readinessOf(ModelState.Absent, supported = false))
    }

    @Test
    fun failuresKeepTheirKind() {
        assertEquals(AudioModelFailure.NETWORK, WhisperAudioFileHost.failureOf(ModelFailure.Network))
        assertEquals(AudioModelFailure.STORAGE, WhisperAudioFileHost.failureOf(ModelFailure.NotEnoughStorage(100, 10)))
        assertEquals(AudioModelFailure.CHECKSUM, WhisperAudioFileHost.failureOf(ModelFailure.Checksum))
        assertEquals(AudioModelFailure.OTHER, WhisperAudioFileHost.failureOf(ModelFailure.Storage))
    }
}
