package jp.kz2601.metamon.services

/**
 * 録音 → 文字起こし(iOS の `SpeechTranscriber`)。本物の実装(#359)が入るまではモック([jp.kz2601.metamon.services.mock.MockTranscriber])だけ。
 * 形は iOS に合わせた最小限。#359 で足りない口が出たらここを広げる。
 */
interface SpeechTranscriber {
    val isReady: Boolean

    suspend fun prepare()

    suspend fun startRecording()

    /** 録音を止めて文字にする。 */
    suspend fun stopAndTranscribe(): String

    fun cancelRecording()
}
