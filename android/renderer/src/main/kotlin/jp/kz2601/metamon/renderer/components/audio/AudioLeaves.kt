package jp.kz2601.metamon.renderer.components.audio

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/**
 * 音の部品(#864)。1 行 1 部品・名前順。録音は端末内だけ(サーバーへ上げない)、話して入力の声も端末の外へ出さない。
 * 音の取り合いは [AudioArbiter]、マイクの権限は [LocalMicrophoneAccess]、話して入力は [DictationHost](:app が差す)。
 */
fun ComponentRegistry.Builder.registerAudioLeaves() {
    register(ComponentType.AUDIO_RECORDER) { c, ctx, m -> AudioRecorderLeaf(c, ctx, m) }
    register(ComponentType.SPEAK_BUTTON) { c, ctx, m -> SpeakButtonLeaf(c, ctx, m) }
    register(ComponentType.TRANSCRIBE_FIELD) { c, ctx, m -> TranscribeFieldLeaf(c, ctx, m) }
}
