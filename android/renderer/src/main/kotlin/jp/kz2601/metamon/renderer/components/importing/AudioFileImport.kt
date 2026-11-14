package jp.kz2601.metamon.renderer.components.importing

import android.net.Uri
import kotlinx.coroutines.flow.StateFlow

/** 録音ファイル用の音声モデルの具合(iOS の `DictationReadiness` の録音ファイル版)。 */
sealed interface AudioModelReadiness {
    /** 文字にできる */
    data object Ready : AudioModelReadiness

    /** 用意している(落としている・読み込んでいる)。[fraction] は 0..1(分からなければ null) */
    data class Preparing(val fraction: Float?) : AudioModelReadiness

    /** 端末にまだ無い(落とす前に大きさを示して確かめる) */
    data object NeedsModel : AudioModelReadiness

    /** 用意に失敗した。[prepareAudioFileTranscription][AudioFileTranscriptionHost.prepareAudioFileTranscription] でやり直す */
    data class Failed(val reason: AudioModelFailure) : AudioModelReadiness

    /** この端末では使えない(whisper.cpp が動かない) */
    data object Unavailable : AudioModelReadiness
}

enum class AudioModelFailure { NETWORK, STORAGE, CHECKSUM, OTHER }

/** 録音ファイルを文字にできなかった理由(キャンセルは CancellationException)。 */
class AudioFileTranscriptionException(val kind: Kind, cause: Throwable? = null) : Exception(kind.name, cause) {
    enum class Kind { NOT_READY, UNREADABLE, FAILED }
}

/**
 * `fileImport` の accept audio: 選んだ録音ファイルを端末内で文字にする口(iOS の `AudioFileTranscriptionHost`)。
 * :app が whisper.cpp(:whisper)で実装する。音声も文字も端末の外に出さない。持たないホスト(プレビュー・テスト)では部品が「使えません」を出す。
 */
interface AudioFileTranscriptionHost {
    val audioFileReadiness: StateFlow<AudioModelReadiness>

    /** 落とすモデルの大きさ(MB)。確認の文に出す */
    val modelMegabytes: Int

    /** モデルが無い・失敗した時: 用意を始める(確認は部品が先に出す)。 */
    fun prepareAudioFileTranscription()

    /** 端末内で文字にする([progress] は 0..1)。声が無ければ ""。失敗は [AudioFileTranscriptionException]。 */
    suspend fun transcribeAudioFile(uri: Uri, progress: (Double) -> Unit): String
}

/** 録音ファイルの上限と下見(iOS の `AudioFileImport`)。上限は backend の `dsl.MaxImportAudioMinutes` / `MaxImportAudioMB` と同じ。 */
object AudioFileImport {
    const val MAX_SECONDS: Double = 30 * 60.0
    const val MAX_BYTES: Long = 200L * 1_000_000

    enum class Problem { TOO_LARGE, TOO_LONG, UNREADABLE, EMPTY }

    /** 大きさ・長さが上限に収まるか。収まれば null。 */
    fun check(bytes: Long?, seconds: Double?): Problem? {
        if (bytes != null && bytes > MAX_BYTES) return Problem.TOO_LARGE
        if (seconds == null || !seconds.isFinite()) return Problem.UNREADABLE
        if (seconds <= 0) return Problem.EMPTY
        if (seconds > MAX_SECONDS) return Problem.TOO_LONG
        return null
    }

    /** 長さ → (分, 秒)。分が 0 なら秒だけ出す。 */
    fun minutesAndSeconds(seconds: Double): Pair<Int, Int> {
        val s = maxOf(0, Math.round(seconds).toInt())
        return s / 60 to s % 60
    }

    private val audioExtensions = setOf("m4a", "mp3", "wav", "aac", "ogg", "oga", "opus", "flac", "3gp", "amr", "caf", "mp4", "webm")

    /** MIME が audio/ で始まるか、MIME が曖昧な時は拡張子で録音ファイルとみなす。 */
    fun isAudio(fileName: String, mime: String?): Boolean {
        if (mime?.startsWith("audio/") == true) return true
        val generic = mime == null || mime == "application/octet-stream"
        return generic && fileName.substringAfterLast('.', "").lowercase() in audioExtensions
    }
}
