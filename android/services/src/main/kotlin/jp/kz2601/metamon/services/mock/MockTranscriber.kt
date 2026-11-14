package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.SpeechTranscriber
import kotlinx.coroutines.delay

/** 決まった文を返す文字起こし(iOS の `MockTranscriber`)。マイクは使わない。 */
class MockTranscriber(
    var cannedText: String = DEFAULT_TEXT,
    private val delayMillis: Long = 800,
) : SpeechTranscriber {
    override val isReady: Boolean = true

    /** 録音中か(画面を閉じたら録音が止まるかをテストで見る) */
    @Volatile
    var isRecording: Boolean = false
        private set

    override suspend fun prepare() = Unit

    override suspend fun startRecording() {
        isRecording = true
    }

    override suspend fun stopAndTranscribe(): String {
        isRecording = false
        delay(delayMillis)
        return cannedText
    }

    override fun cancelRecording() {
        isRecording = false
    }

    companion object {
        const val DEFAULT_TEXT = "今月の支出を記録できる家計簿アプリが欲しい"
    }
}
