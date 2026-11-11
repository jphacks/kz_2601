package jp.kz2601.metamon.whisper

import android.content.Context
import android.net.Uri
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/**
 * 端末内の文字起こし(iOS の `WhisperKitRecognizer` の文字にする部分)。モデルは [models] が用意し、初めて使う時に読み込む。
 * 音も文字も端末の外に出さない。どの関数もメインスレッドを塞がない(重い所は IO のスレッド)。
 */
class WhisperTranscriber(
    private val context: Context,
    val models: WhisperModelManager,
) {
    private val engineMutex = Mutex()
    private var engine: WhisperEngine? = null

    /** この端末で whisper.cpp が動くか(.so が読めるか)。 */
    val isSupported: Boolean get() = WhisperEngine.isSupported

    /** モデルを用意して読み込む(無ければ落とす)。失敗は [ModelException] / [WhisperUnavailableException]。 */
    suspend fun engine(): WhisperEngine = engineMutex.withLock {
        engine ?: WhisperEngine.load(models.ensure()).also { engine = it }
    }

    /** 16kHz モノラルの float を文字にする。短すぎ・無音なら ""。 */
    suspend fun transcribe(samples: FloatArray, language: String): String =
        if (!WhisperAudio.hasVoice(samples)) "" else engine().transcribe(samples, language)

    /**
     * 録音ファイルを 30 秒ずつ読み・文字にしてつなぐ(IO のスレッドで)。[progress] は 0..1(長さが分からなければ最後に 1 だけ)。
     * キャンセルで止まる(CancellationException)。読めないファイルは [AudioDecodeException]。声が無ければ ""。
     */
    suspend fun transcribeFile(
        uri: Uri,
        language: String,
        totalSeconds: Double?,
        progress: suspend (Double) -> Unit,
    ): String = withContext(Dispatchers.IO) {
        val e = engine()
        val parts = mutableListOf<String>()
        val blocks = BlockAccumulator()
        var doneSamples = 0L
        progress(0.0)
        suspend fun run(block: FloatArray) {
            if (WhisperAudio.hasVoice(block)) parts += e.transcribe(block, language)
            doneSamples += block.size
            if (totalSeconds != null && totalSeconds > 0) {
                progress((doneSamples.toDouble() / WhisperAudio.SAMPLE_RATE / totalSeconds).coerceIn(0.0, 1.0))
            }
        }
        AudioFileDecoder.decode(context, uri) { chunk -> blocks.add(chunk).forEach { run(it) } }
        blocks.finish()?.let { run(it) }
        progress(1.0)
        WhisperAudio.join(parts)
    }

    /** 読み込んだモデルをメモリから外す(端末のファイルは残す)。 */
    suspend fun unload() = engineMutex.withLock {
        engine?.release()
        engine = null
    }
}
