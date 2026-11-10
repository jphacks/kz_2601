package jp.kz2601.metamon.services.speech

import kotlinx.coroutines.flow.StateFlow

/** 声を文字にする仕組みの状態(画面の表示用)。iOS の `TranscriberStatus` に当たる。 */
sealed interface TranscriberStatus {
    /** 端末内で使えるが、その言語のモデルがまだ無い(API 33+ で分かる) */
    data object NotDownloaded : TranscriberStatus

    /** モデルを取得中 */
    data object Downloading : TranscriberStatus

    /** 録音できる */
    data object Ready : TranscriberStatus

    /** 聞いている・文字にしている */
    data object Recognizing : TranscriberStatus

    /** この端末・言語では端末内の認識が使えない。文字の入力に落とす */
    data class Unavailable(val reason: String) : TranscriberStatus
}

/**
 * 音声入力(iOS の `SpeechTranscriber`)。実機は [AndroidSpeechTranscriber]、テストはモック。
 * 声は端末の外へ送らない(#354 §3)。端末内の認識が無い端末は [TranscriberStatus.Unavailable] になり、文字入力に落とす。
 */
interface SpeechTranscriber {
    val status: StateFlow<TranscriberStatus>

    /** 録音中の途中の聞き取り。録音の始めに "" に戻る */
    val partial: StateFlow<String>

    /** 聞いている間 true。認識器が無音で自分から止まった時も false になる(画面はそれを見て [stopAndTranscribe] を呼ぶ) */
    val listening: StateFlow<Boolean>

    val isReady: Boolean get() = status.value == TranscriberStatus.Ready

    /** 声で入力できる見込みがあるか(false なら最初から文字入力を勧める) */
    val isAvailable: Boolean get() = status.value !is TranscriberStatus.Unavailable

    /** 言語のモデルの有無を確かめる(API 33+)。失敗しても投げない */
    suspend fun prepare() {}

    /** 言語のモデルを取りに行く(API 33+)。モバイル回線の確認は呼ぶ側(android-050 の残り) */
    fun downloadModel() {}

    /** 録音を始める。権限が無い・端末内で使えない時は [TranscriberFailure] を投げる */
    suspend fun startRecording()

    /**
     * 録音を止めて文字を返す。短すぎ / 無音 / 聞き取れない / マイクが使えない は [TranscriberFailure] を投げる。
     * 返す文字は空にならない(空は [TranscriberFailure.NotHeard])。
     */
    suspend fun stopAndTranscribe(): String

    /** 録音を捨てて止める(画面が閉じられた時など)。録音していなければ何もしない */
    fun cancelRecording()
}

/** 録音 / 認識が失敗した理由。画面の文言は :app の `VoiceErrorCopy`(iOS の `ErrorCopy.transcriber*`)で決める。 */
sealed class TranscriberFailure(message: String) : Exception(message) {
    /** 言語のモデルの準備中 */
    data object NotReady : TranscriberFailure("notReady")

    /** マイクの権限が無い */
    data object MicrophoneDenied : TranscriberFailure("microphoneDenied")

    /** マイクを使えない(他のアプリが使用中・認識器が塞がっている) */
    data object NoInputAvailable : TranscriberFailure("noInputAvailable")

    data class TooShort(val seconds: Double) : TranscriberFailure("tooShort($seconds)")

    /** 声が届かなかった(無音のまま時間切れ) */
    data class Silent(val seconds: Double) : TranscriberFailure("silent($seconds)")

    /** 声はあったが文字にならなかった(iOS は "" を返して「うまく聞き取れませんでした」) */
    data object NotHeard : TranscriberFailure("notHeard")

    /** この端末・言語では端末内の認識が使えない(サーバーの認識には落とさない) */
    data object OnDeviceRecognitionUnavailable : TranscriberFailure("onDeviceUnavailable")

    /** 分類できない `SpeechRecognizer.ERROR_*` */
    data class Other(val code: Int) : TranscriberFailure("error($code)")
}

/** UI の言語("ja" | "en")→ 認識の言語タグ。iOS の `localeID(for:)` と同じ。 */
fun speechLanguageTag(language: String): String = if (language == "en") "en-US" else "ja-JP"
