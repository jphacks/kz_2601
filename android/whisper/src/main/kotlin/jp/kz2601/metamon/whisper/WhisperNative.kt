package jp.kz2601.metamon.whisper

/** whisper.cpp の JNI の口(src/main/cpp/whisper_jni.cpp)。直接は使わず [WhisperEngine] を通す。 */
internal object WhisperNative {
    /** .so を読めたか(読めない端末・JVM のテストでは false)。 */
    val loaded: Boolean by lazy {
        try {
            System.loadLibrary("metamon_whisper")
            true
        } catch (_: UnsatisfiedLinkError) {
            false
        } catch (_: SecurityException) {
            false
        }
    }

    external fun initContext(modelPath: String): Long

    external fun freeContext(ctx: Long)

    external fun newAbort(): Long

    external fun requestAbort(abort: Long)

    external fun freeAbort(abort: Long)

    /** 16kHz モノラルの float → UTF-8 のバイト列。失敗・やめた時は null。 */
    external fun transcribe(ctx: Long, abort: Long, samples: FloatArray, language: String, threads: Int): ByteArray?
}
