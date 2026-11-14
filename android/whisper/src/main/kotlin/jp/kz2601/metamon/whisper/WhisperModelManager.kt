package jp.kz2601.metamon.whisper

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.Closeable
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.security.MessageDigest
import kotlin.coroutines.cancellation.CancellationException

/** 音声モデルの状態(画面の表示用)。 */
sealed interface ModelState {
    /** 端末に無い(まだ落としていない・やめた) */
    data object Absent : ModelState

    /** 落としている。[fraction] は 0..1(大きさが分からない間は null) */
    data class Downloading(val fraction: Float?) : ModelState

    /** 落とし終えて、ハッシュを確かめている */
    data object Verifying : ModelState

    data class Ready(val file: File) : ModelState

    /** 失敗。もう一度 [WhisperModelManager.ensure] を呼べばやり直す */
    data class Failed(val failure: ModelFailure) : ModelState
}

/** モデルを用意できなかった理由。 */
sealed interface ModelFailure {
    /** 通信できない・途中で切れた・サーバーが断った */
    data object Network : ModelFailure

    /** 端末の空きが足りない */
    data class NotEnoughStorage(val requiredBytes: Long, val availableBytes: Long) : ModelFailure

    /** 落としたものがハッシュ・大きさに合わない(壊れた・差し替えられた) */
    data object Checksum : ModelFailure

    /** 端末に書けない(空き以外) */
    data object Storage : ModelFailure
}

class ModelException(val failure: ModelFailure, cause: Throwable? = null) : Exception(failure.toString(), cause)

/** モデルの取得元(本物は [OkHttpModelSource]・テストは偽物)。[open] は IO のスレッドで呼ぶ。 */
fun interface ModelSource {
    /** 失敗は IOException */
    fun open(url: String): ModelStream
}

class ModelStream(
    /** Content-Length(分からなければ null) */
    val contentLength: Long?,
    val input: InputStream,
    private val onClose: () -> Unit = {},
) : Closeable {
    override fun close() {
        try {
            input.close()
        } finally {
            onClose()
        }
    }
}

class OkHttpModelSource(private val client: OkHttpClient = OkHttpClient()) : ModelSource {
    override fun open(url: String): ModelStream {
        val response = client.newCall(Request.Builder().url(url).build()).execute()
        if (!response.isSuccessful) {
            response.close()
            throw IOException("HTTP ${response.code}")
        }
        val body = response.body
        return ModelStream(body.contentLength().takeIf { it >= 0 }, body.byteStream()) { response.close() }
    }
}

/**
 * 音声モデルを端末(アプリの領域の [dir])に用意する。APK には入れない。
 * - 落とす前に空きを確かめる([spec] の大きさ + [STORAGE_MARGIN_BYTES])
 * - `.part` に書きながら SHA-256 を取り、大きさとハッシュが合った時だけ本来の名前に変える(途中の物を使わない)
 * - 呼んだ coroutine のキャンセルで止まり、`.part` を消して [ModelState.Absent] に戻る
 * - 同時に呼ばれても 1 回だけ落とす
 */
class WhisperModelManager(
    private val dir: File,
    val spec: WhisperModelSpec = WhisperModels.DEFAULT,
    private val source: ModelSource,
    private val freeBytes: (File) -> Long = { it.usableSpace },
    private val io: CoroutineDispatcher = Dispatchers.IO,
) {
    private val mutex = Mutex()
    private val target = File(dir, spec.fileName)
    private val partial = File(dir, spec.fileName + ".part")
    private val _state = MutableStateFlow(initialState())
    val state: StateFlow<ModelState> = _state.asStateFlow()

    /** 落とす必要があるか(画面が大きさを示して確かめる) */
    val needsDownload: Boolean get() = _state.value !is ModelState.Ready

    private fun initialState(): ModelState {
        // 前回の途中の物は使わない(大きさだけ合っていてもハッシュを確かめていない)
        partial.delete()
        return if (target.isFile && target.length() == spec.bytes) ModelState.Ready(target) else ModelState.Absent
    }

    /** 用意できたモデルのファイルを返す。無ければ落とす。失敗は [ModelException]。 */
    suspend fun ensure(): File = mutex.withLock {
        (_state.value as? ModelState.Ready)?.let { if (it.file.isFile) return@withLock it.file }
        try {
            download()
        } catch (e: CancellationException) {
            partial.delete()
            _state.value = ModelState.Absent
            throw e
        } catch (e: ModelException) {
            partial.delete()
            _state.value = ModelState.Failed(e.failure)
            throw e
        }
    }

    /** 端末のモデルを消す(設定の「データを消す」など)。 */
    suspend fun delete() = mutex.withLock {
        withContext(io) {
            target.delete()
            partial.delete()
        }
        _state.value = ModelState.Absent
    }

    private suspend fun download(): File = withContext(io) {
        if (!dir.isDirectory && !dir.mkdirs()) throw ModelException(ModelFailure.Storage)
        partial.delete()
        val available = freeBytes(dir)
        val required = spec.bytes + STORAGE_MARGIN_BYTES
        if (available < required) throw ModelException(ModelFailure.NotEnoughStorage(required, available))

        _state.value = ModelState.Downloading(0f)
        val digest = MessageDigest.getInstance("SHA-256")
        var written = 0L
        val stream = try {
            source.open(spec.url)
        } catch (e: IOException) {
            throw ModelException(ModelFailure.Network, e)
        }
        stream.use { s ->
            val total = s.contentLength ?: spec.bytes
            if (total != spec.bytes) throw ModelException(ModelFailure.Checksum)
            val out = try {
                partial.outputStream()
            } catch (e: IOException) {
                throw storageFailure(e, required)
            }
            out.use { o ->
                val buffer = ByteArray(BUFFER_BYTES)
                var lastReported = 0f
                while (true) {
                    currentCoroutineContext().ensureActive()
                    val n = try {
                        s.input.read(buffer)
                    } catch (e: IOException) {
                        throw ModelException(ModelFailure.Network, e)
                    }
                    if (n < 0) break
                    written += n
                    if (written > spec.bytes) throw ModelException(ModelFailure.Checksum)
                    try {
                        o.write(buffer, 0, n)
                    } catch (e: IOException) {
                        throw storageFailure(e, required)
                    }
                    digest.update(buffer, 0, n)
                    val fraction = written.toFloat() / total
                    if (fraction - lastReported >= PROGRESS_STEP) {
                        lastReported = fraction
                        _state.value = ModelState.Downloading(fraction)
                    }
                }
            }
        }
        _state.value = ModelState.Verifying
        if (written != spec.bytes) throw ModelException(if (written < spec.bytes) ModelFailure.Network else ModelFailure.Checksum)
        if (digest.digest().toHex() != spec.sha256.lowercase()) throw ModelException(ModelFailure.Checksum)
        target.delete()
        if (!partial.renameTo(target)) throw ModelException(ModelFailure.Storage)
        _state.value = ModelState.Ready(target)
        target
    }

    /** 書けなかった: 空きが無いなら [ModelFailure.NotEnoughStorage]、それ以外は [ModelFailure.Storage]。 */
    private fun storageFailure(e: IOException, required: Long): ModelException {
        val available = freeBytes(dir)
        val noSpace = available < required || e.message?.contains("ENOSPC") == true || e.message?.contains("No space", ignoreCase = true) == true
        return ModelException(if (noSpace) ModelFailure.NotEnoughStorage(required, available) else ModelFailure.Storage, e)
    }

    companion object {
        /** モデルの大きさに足して残しておく空き(端末を一杯にしない) */
        const val STORAGE_MARGIN_BYTES: Long = 50L * 1_000_000
        private const val BUFFER_BYTES = 64 * 1024
        private const val PROGRESS_STEP = 0.01f
    }
}

internal fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it) }
