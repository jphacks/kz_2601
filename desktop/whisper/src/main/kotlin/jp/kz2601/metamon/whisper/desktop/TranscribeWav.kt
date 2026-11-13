package jp.kz2601.metamon.whisper.desktop

import jp.kz2601.metamon.whisper.OkHttpModelSource
import jp.kz2601.metamon.whisper.StreamingResampler
import jp.kz2601.metamon.whisper.WhisperEngine
import jp.kz2601.metamon.whisper.WhisperModelManager
import kotlinx.coroutines.runBlocking
import java.io.File
import javax.sound.sampled.AudioFormat
import javax.sound.sampled.AudioSystem
import kotlin.system.exitProcess

/**
 * 手元の確かめ(`./gradlew :whisper:transcribeWav --args="<wav> [ja|en]"`)。wav を whisper.cpp で文字にし、読み込みと文字にする時間を出す。
 * モデルは `TAILORIT_WHISPER_MODEL`(ファイル)があればそれ、無ければ `-Dtailorit.whisper.modelDir`(既定 build/whisper-models)に落とす。
 */
fun main(args: Array<String>) {
    val wav = args.firstOrNull()?.let(::File) ?: run {
        System.err.println("使い方: transcribeWav <wav> [ja|en]")
        exitProcess(2)
    }
    val language = args.getOrNull(1) ?: "ja"
    if (!WhisperEngine.isSupported) {
        System.err.println("ライブラリを読めません(desktop/whisper/native/build.sh で作る): ${NativeLibrary.lastError}")
        exitProcess(1)
    }
    val samples = readWav(wav)
    runBlocking {
        val model = System.getenv("TAILORIT_WHISPER_MODEL")?.takeIf { it.isNotBlank() }?.let(::File)
            ?: WhisperModelManager(File(System.getProperty("tailorit.whisper.modelDir") ?: "build/whisper-models"), source = OkHttpModelSource()).ensure()
        val t0 = System.nanoTime()
        val engine = WhisperEngine.load(model)
        val t1 = System.nanoTime()
        val text = engine.transcribe(samples, language)
        val t2 = System.nanoTime()
        engine.release()
        println("音: %.2f 秒 / モデル: %s".format(samples.size / 16_000.0, model.name))
        println("読み込み: %d ms / 文字にする: %d ms".format((t1 - t0) / 1_000_000, (t2 - t1) / 1_000_000))
        println(text)
    }
}

/** wav(などの Java Sound が読む形)→ 16kHz モノラルの float。 */
fun readWav(file: File): FloatArray {
    AudioSystem.getAudioInputStream(file).use { raw ->
        val src = raw.format
        val pcm = AudioFormat(AudioFormat.Encoding.PCM_SIGNED, src.sampleRate, 16, src.channels, src.channels * 2, src.sampleRate, false)
        AudioSystem.getAudioInputStream(pcm, raw).use { s ->
            val bytes = s.readAllBytes()
            return Pcm.toWhisper(bytes, bytes.size, pcm, StreamingResampler(src.sampleRate.toInt()))
        }
    }
}
