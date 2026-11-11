package jp.kz2601.metamon.whisper

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File
import kotlinx.coroutines.currentCoroutineContext

/** whisper.cpp が使えない(.so が読めない・モデルが壊れている)。 */
class WhisperUnavailableException(message: String) : Exception(message)

/**
 * 読み込んだ 1 つのモデル(whisper_context)。同時には 1 つずつ文字にする。
 * 文字にしている途中で呼んだ coroutine がキャンセルされたら、whisper.cpp の abort_callback で早めにやめる。
 */
class WhisperEngine private constructor(
    private var ctx: Long,
    private val io: CoroutineDispatcher,
) {
    private val mutex = Mutex()

    /**
     * 16kHz モノラルの float を [language]("ja" | "en")で文字にする。声が無ければ ""。
     * キャンセルは CancellationException。
     */
    suspend fun transcribe(samples: FloatArray, language: String): String = mutex.withLock {
        check(ctx != 0L) { "closed" }
        currentCoroutineContext().ensureActive()
        val abort = WhisperNative.newAbort()
        try {
            val bytes = coroutineScope {
                // 呼び手がキャンセルされたら native に知らせる(終わった後に立っても害は無い)
                val watcher = launch {
                    try {
                        awaitCancellation()
                    } finally {
                        WhisperNative.requestAbort(abort)
                    }
                }
                val out = withContext(io) { WhisperNative.transcribe(ctx, abort, samples, whisperLanguage(language), threads()) }
                watcher.cancel()
                out
            }
            currentCoroutineContext().ensureActive()
            bytes?.toString(Charsets.UTF_8)?.trim() ?: throw WhisperUnavailableException("whisper_full failed")
        } finally {
            WhisperNative.freeAbort(abort)
        }
    }

    /** モデルを手放す(文字にしている途中なら終わるのを待つ)。 */
    suspend fun release() = mutex.withLock {
        if (ctx != 0L) {
            WhisperNative.freeContext(ctx)
            ctx = 0L
        }
    }

    companion object {
        /** .so を読める端末か。 */
        val isSupported: Boolean get() = WhisperNative.loaded

        /** モデルを読み込む(数秒かかる)。失敗は [WhisperUnavailableException]。 */
        suspend fun load(model: File, io: CoroutineDispatcher = Dispatchers.IO): WhisperEngine {
            if (!WhisperNative.loaded) throw WhisperUnavailableException("native library not loaded")
            val ctx = withContext(io) { WhisperNative.initContext(model.absolutePath) }
            if (ctx == 0L) throw WhisperUnavailableException("cannot load model ${model.name}")
            return WhisperEngine(ctx, io)
        }

        /** UI の言語 → whisper の言語。日本語と英語だけ(それ以外は日本語)。 */
        fun whisperLanguage(language: String): String = if (language.lowercase().startsWith("en")) "en" else "ja"

        /** 大きいコアの数の目安(多すぎると小さいコアに足を引っぱられる)。 */
        private fun threads(): Int = Runtime.getRuntime().availableProcessors().coerceIn(1, 4)
    }
}
