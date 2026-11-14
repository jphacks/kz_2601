package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.store.AppDataDir
import jp.kz2601.metamon.whisper.IdleReleasing
import jp.kz2601.metamon.whisper.ModelState
import jp.kz2601.metamon.whisper.OkHttpModelSource
import jp.kz2601.metamon.whisper.WhisperAudio
import jp.kz2601.metamon.whisper.WhisperEngine
import jp.kz2601.metamon.whisper.WhisperModelManager
import jp.kz2601.metamon.whisper.WhisperModels
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import java.io.File

/** 16kHz モノラルの float を文字にする(本物は [WhisperCppTranscriber]・テストは偽物)。音も文字も端末の外に出さない。 */
interface Transcriber {
    /** この PC で使えるか(whisper.cpp のライブラリが読めるか)。 */
    val isAvailable: Boolean

    /** モデルをまだ落としていなければ、その大きさ(MB)。用意できていれば null。 */
    val pendingDownloadMegabytes: Int?

    /**
     * [language]("ja" | "en")で文字にする。声が無ければ ""。
     * モデルの用意の失敗は [jp.kz2601.metamon.whisper.ModelException]、ライブラリ・モデルが読めない時は [jp.kz2601.metamon.whisper.WhisperUnavailableException]。
     */
    suspend fun transcribe(samples: FloatArray, language: String): String
}

/**
 * whisper.cpp(JNI)で文字にする。モデルは初回に [VoiceModels.dir] へ落とし(Android の WhisperModels と同じ物・同じハッシュ)、
 * 最後に使ってから [idleMillis] 経つとメモリから外す(ファイルは残す)。
 */
class WhisperCppTranscriber(
    private val models: WhisperModelManager = WhisperModelManager(VoiceModels.dir(), source = OkHttpModelSource()),
    private val modelOverride: File? = VoiceModels.override(),
    scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Default),
    idleMillis: Long = IDLE_UNLOAD_MILLIS,
) : Transcriber {
    private val engine = IdleReleasing(
        scope = scope,
        idleMillis = idleMillis,
        load = { WhisperEngine.load(modelOverride ?: models.ensure()) },
        release = { it.release() },
    )

    override val isAvailable: Boolean get() = WhisperEngine.isSupported

    override val pendingDownloadMegabytes: Int?
        get() = if (modelOverride != null || models.state.value is ModelState.Ready) null else models.spec.megabytes

    override suspend fun transcribe(samples: FloatArray, language: String): String =
        if (!WhisperAudio.hasVoice(samples)) "" else engine.use { it.transcribe(samples, language) }

    companion object {
        /** Android の WhisperTranscriber と同じ(続けて話す時に数秒の読み込みを繰り返さない長さ)。 */
        const val IDLE_UNLOAD_MILLIS: Long = 2 * 60 * 1000L
    }
}

/**
 * 音声モデルの置き場。モデルは git にも配る形にも入れず、初回に落とす(Android の WhisperModels と同じ物)。
 * - 既定: 端末の保存先([AppDataDir])の下の `models/`(Windows `%APPDATA%\TailorIt\models`・Linux `~/.local/share/tailorit/models`)
 * - `TAILORIT_WHISPER_MODEL=<ggml のファイル>`(か `-Dtailorit.whisper.model`)で、手元のモデルを落とさずに使う(試す時・社内配布)
 * サインアウトで端末のデータを消す時もモデルは消さない(個人の情報ではなく、落とし直すと 60MB かかる)。
 */
object VoiceModels {
    fun dir(base: File = AppDataDir.resolve()): File = File(base, "models")

    fun override(
        props: (String) -> String? = System::getProperty,
        env: (String) -> String? = System::getenv,
    ): File? = (props("tailorit.whisper.model")?.takeIf { it.isNotBlank() } ?: env("TAILORIT_WHISPER_MODEL")?.takeIf { it.isNotBlank() })?.let(::File)

    val DEFAULT = WhisperModels.DEFAULT
}
