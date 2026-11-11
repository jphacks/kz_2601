package jp.kz2601.metamon.runtime

import android.content.Context
import android.net.Uri
import jp.kz2601.metamon.renderer.components.importing.AudioFileTranscriptionException
import jp.kz2601.metamon.renderer.components.importing.AudioFileTranscriptionHost
import jp.kz2601.metamon.renderer.components.importing.AudioModelFailure
import jp.kz2601.metamon.renderer.components.importing.AudioModelReadiness
import jp.kz2601.metamon.whisper.AudioDecodeException
import jp.kz2601.metamon.whisper.AudioFileDecoder
import jp.kz2601.metamon.whisper.ModelException
import jp.kz2601.metamon.whisper.ModelFailure
import jp.kz2601.metamon.whisper.ModelState
import jp.kz2601.metamon.whisper.OkHttpModelSource
import jp.kz2601.metamon.whisper.WhisperModelManager
import jp.kz2601.metamon.whisper.WhisperTranscriber
import jp.kz2601.metamon.whisper.WhisperUnavailableException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

/**
 * fileImport の録音ファイルを whisper.cpp で端末内で文字にする(#1180・iOS の `AppRuntime` の `AudioFileTranscriptionHost`)。
 * モデルは初回に落とす(部品が大きさを示して確かめてから [prepareAudioFileTranscription])。音声も文字も端末の外に出さない。
 */
class WhisperAudioFileHost(
    private val context: Context,
    private val transcriber: WhisperTranscriber,
    private val scope: CoroutineScope,
    private val language: () -> String,
) : AudioFileTranscriptionHost {
    private var prepareJob: Job? = null

    override val audioFileReadiness: StateFlow<AudioModelReadiness> =
        transcriber.models.state
            .map { readinessOf(it, transcriber.isSupported) }
            .stateIn(scope, SharingStarted.Eagerly, readinessOf(transcriber.models.state.value, transcriber.isSupported))

    override val modelMegabytes: Int get() = transcriber.models.spec.megabytes

    override fun prepareAudioFileTranscription() {
        if (!transcriber.isSupported || prepareJob?.isActive == true) return
        prepareJob = scope.launch {
            // 失敗は models.state(Failed)に出る。部品はそれを見て「もう一度」を出す
            runCatching { transcriber.models.ensure() }
        }
    }

    override suspend fun transcribeAudioFile(uri: Uri, progress: (Double) -> Unit): String {
        if (transcriber.models.state.value !is ModelState.Ready) throw AudioFileTranscriptionException(AudioFileTranscriptionException.Kind.NOT_READY)
        return try {
            val seconds = withContext(Dispatchers.IO) { AudioFileDecoder.durationSeconds(context, uri) }
            transcriber.transcribeFile(uri, language(), seconds) { p -> withContext(Dispatchers.Main) { progress(p) } }
        } catch (e: AudioDecodeException) {
            throw AudioFileTranscriptionException(AudioFileTranscriptionException.Kind.UNREADABLE, e)
        } catch (e: ModelException) {
            throw AudioFileTranscriptionException(AudioFileTranscriptionException.Kind.NOT_READY, e)
        } catch (e: WhisperUnavailableException) {
            throw AudioFileTranscriptionException(AudioFileTranscriptionException.Kind.FAILED, e)
        }
    }

    companion object {
        /** 端末のアプリの領域(no_backup: 端末のバックアップに 60MB を載せない)に置く。 */
        fun create(context: Context, scope: CoroutineScope, language: () -> String): WhisperAudioFileHost {
            val dir = File(context.noBackupFilesDir, "whisper-models")
            val models = WhisperModelManager(dir, source = OkHttpModelSource())
            return WhisperAudioFileHost(context.applicationContext, WhisperTranscriber(context.applicationContext, models), scope, language)
        }

        /** モデルの状態 → 部品の表示。 */
        fun readinessOf(state: ModelState, supported: Boolean): AudioModelReadiness = if (!supported) {
            AudioModelReadiness.Unavailable
        } else {
            when (state) {
                is ModelState.Ready -> AudioModelReadiness.Ready
                ModelState.Absent -> AudioModelReadiness.NeedsModel
                is ModelState.Downloading -> AudioModelReadiness.Preparing(state.fraction)
                ModelState.Verifying -> AudioModelReadiness.Preparing(null)
                is ModelState.Failed -> AudioModelReadiness.Failed(failureOf(state.failure))
            }
        }

        fun failureOf(f: ModelFailure): AudioModelFailure = when (f) {
            ModelFailure.Network -> AudioModelFailure.NETWORK
            is ModelFailure.NotEnoughStorage -> AudioModelFailure.STORAGE
            ModelFailure.Checksum -> AudioModelFailure.CHECKSUM
            ModelFailure.Storage -> AudioModelFailure.OTHER
        }
    }
}
