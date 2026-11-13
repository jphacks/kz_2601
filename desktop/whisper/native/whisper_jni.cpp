// デスクトップの whisper.cpp の JNI の口(jp.kz2601.metamon.whisper.WhisperNative)。
// android/whisper/src/main/cpp/whisper_jni.cpp と同じ関数・同じ引数(Kotlin の WhisperEngine を共有する)。違いはログの出し先だけ。
#include <jni.h>

#include <atomic>
#include <cstdio>
#include <string>

#include "whisper.h"

namespace {

struct Abort {
    std::atomic<bool> stop{false};
};

bool shouldAbort(void *data) {
    return data != nullptr && static_cast<Abort *>(data)->stop.load();
}

bool abortCallback(void *data) { return shouldAbort(data); }

bool encoderBegin(whisper_context *, whisper_state *, void *data) { return !shouldAbort(data); }

void logCallback(ggml_log_level level, const char *text, void *) {
    if (level >= GGML_LOG_LEVEL_WARN) std::fputs(text, stderr);
}

} // namespace

extern "C" {

JNIEXPORT jlong JNICALL
Java_jp_kz2601_metamon_whisper_WhisperNative_initContext(JNIEnv *env, jobject, jstring modelPath) {
    whisper_log_set(logCallback, nullptr);
    const char *path = env->GetStringUTFChars(modelPath, nullptr);
    whisper_context_params params = whisper_context_default_params();
    params.use_gpu = false;
    whisper_context *ctx = whisper_init_from_file_with_params(path, params);
    env->ReleaseStringUTFChars(modelPath, path);
    return reinterpret_cast<jlong>(ctx);
}

JNIEXPORT void JNICALL
Java_jp_kz2601_metamon_whisper_WhisperNative_freeContext(JNIEnv *, jobject, jlong ctx) {
    if (ctx != 0) whisper_free(reinterpret_cast<whisper_context *>(ctx));
}

JNIEXPORT jlong JNICALL
Java_jp_kz2601_metamon_whisper_WhisperNative_newAbort(JNIEnv *, jobject) {
    return reinterpret_cast<jlong>(new Abort());
}

JNIEXPORT void JNICALL
Java_jp_kz2601_metamon_whisper_WhisperNative_requestAbort(JNIEnv *, jobject, jlong abort) {
    if (abort != 0) reinterpret_cast<Abort *>(abort)->stop.store(true);
}

JNIEXPORT void JNICALL
Java_jp_kz2601_metamon_whisper_WhisperNative_freeAbort(JNIEnv *, jobject, jlong abort) {
    delete reinterpret_cast<Abort *>(abort);
}

// 16kHz モノラルの float を文字にする(UTF-8 のバイト列で返す)。失敗・やめた時は null。
JNIEXPORT jbyteArray JNICALL
Java_jp_kz2601_metamon_whisper_WhisperNative_transcribe(
        JNIEnv *env, jobject, jlong ctxPtr, jlong abortPtr, jfloatArray samples, jstring language, jint threads) {
    auto *ctx = reinterpret_cast<whisper_context *>(ctxPtr);
    auto *abort = reinterpret_cast<Abort *>(abortPtr);
    if (ctx == nullptr || samples == nullptr) return nullptr;

    const char *lang = env->GetStringUTFChars(language, nullptr);
    std::string langCopy(lang);
    env->ReleaseStringUTFChars(language, lang);

    whisper_full_params params = whisper_full_default_params(WHISPER_SAMPLING_GREEDY);
    params.n_threads = threads > 0 ? threads : 1;
    params.language = langCopy.c_str();
    params.translate = false;
    params.no_context = true;
    params.no_timestamps = true;
    params.single_segment = false;
    params.print_progress = false;
    params.print_realtime = false;
    params.print_special = false;
    params.print_timestamps = false;
    params.suppress_blank = true;
    params.abort_callback = abortCallback;
    params.abort_callback_user_data = abort;
    params.encoder_begin_callback = encoderBegin;
    params.encoder_begin_callback_user_data = abort;

    const jsize count = env->GetArrayLength(samples);
    jfloat *data = env->GetFloatArrayElements(samples, nullptr);
    const int rc = whisper_full(ctx, params, data, count);
    env->ReleaseFloatArrayElements(samples, data, JNI_ABORT);
    if (rc != 0 || shouldAbort(abort)) return nullptr;

    std::string text;
    const int segments = whisper_full_n_segments(ctx);
    for (int i = 0; i < segments; ++i) {
        const char *segment = whisper_full_get_segment_text(ctx, i);
        if (segment != nullptr) text += segment;
    }
    jbyteArray out = env->NewByteArray(static_cast<jsize>(text.size()));
    if (out == nullptr) return nullptr;
    env->SetByteArrayRegion(out, 0, static_cast<jsize>(text.size()), reinterpret_cast<const jbyte *>(text.data()));
    return out;
}

} // extern "C"
