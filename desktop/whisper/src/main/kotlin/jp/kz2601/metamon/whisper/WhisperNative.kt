package jp.kz2601.metamon.whisper

import jp.kz2601.metamon.whisper.desktop.NativeLibrary

/**
 * whisper.cpp の JNI の口(desktop/whisper/native/whisper_jni.cpp)。直接は使わず [WhisperEngine] を通す。
 * 関数は android/whisper の WhisperNative と同じ。違うのは読み方だけ([NativeLibrary] の順で探す)。
 */
internal object WhisperNative {
    /** ライブラリを読めたか(作っていない・この OS 向けが無い・JVM のテストでは false)。 */
    val loaded: Boolean by lazy { NativeLibrary.load() }

    external fun initContext(modelPath: String): Long

    external fun freeContext(ctx: Long)

    external fun newAbort(): Long

    external fun requestAbort(abort: Long)

    external fun freeAbort(abort: Long)

    /** 16kHz モノラルの float → UTF-8 のバイト列。失敗・やめた時は null。 */
    external fun transcribe(ctx: Long, abort: Long, samples: FloatArray, language: String, threads: Int): ByteArray?
}
