package jp.kz2601.metamon.whisper

import android.content.Context
import android.net.Uri
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.withContext

/**
 * 端末内の文字起こし(iOS の `WhisperKitRecognizer` の文字にする部分)。モデルは [models] が用意し、初めて使う時に読み込む。
 * 最後に使ってから [idleMillis] 経つとメモリから外す(端末のファイルは残す)。
 * 音も文字も端末の外に出さない。どの関数もメインスレッドを塞がない(重い所は IO のスレッド)。
 */
class WhisperTranscriber(
    private val context: Context,
    val models: WhisperModelManager,
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Default),
    idleMillis: Long = IDLE_UNLOAD_MILLIS,
) {
    private val engine = IdleReleasing(
        scope = scope,
        idleMillis = idleMillis,
        load = { WhisperEngine.load(models.ensure()) },
        release = { it.release() },
    )

    /** この端末で whisper.cpp が動くか(.so が読めるか)。 */
    val isSupported: Boolean get() = WhisperEngine.isSupported

    /** 16kHz モノラルの float を文字にする。短すぎ・無音なら ""。モデルの用意の失敗は [ModelException] / [WhisperUnavailableException]。 */
    suspend fun transcribe(samples: FloatArray, language: String): String =
        if (!WhisperAudio.hasVoice(samples)) "" else engine.use { it.transcribe(samples, language) }

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
        engine.use { e -> transcribeFileWith(e, uri, language, totalSeconds, progress) }
    }

    private suspend fun transcribeFileWith(
        e: WhisperEngine,
        uri: Uri,
        language: String,
        totalSeconds: Double?,
        progress: suspend (Double) -> Unit,
    ): String {
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
        return WhisperAudio.join(parts)
    }

    /** 読み込んだモデルをメモリから外す(端末のファイルは残す)。文字にしている途中なら終わってから外す。 */
    suspend fun unload() = engine.releaseWhenIdle()

    companion object {
        /** 最後に使ってからメモリから外すまで(続けて読み込む時に数秒の読み込みを繰り返さない長さ)。 */
        const val IDLE_UNLOAD_MILLIS: Long = 2 * 60 * 1000L
    }
}
