package jp.kz2601.metamon.whisper

/**
 * 端末に落とす音声モデル(ggml 形式)。APK には入れず、初回に [url] から落として [sha256] で確かめる。
 * 取得先は版(コミット)まで固定し、中身が変わったらハッシュで弾く。
 */
data class WhisperModelSpec(
    val fileName: String,
    val url: String,
    /** 小文字の 16 進 */
    val sha256: String,
    val bytes: Long,
) {
    /** 画面に出す大きさ(MB・切り上げ) */
    val megabytes: Int get() = ((bytes + 999_999) / 1_000_000).toInt()
}

object WhisperModels {
    /** Hugging Face の ggerganov/whisper.cpp のコミット(ここで固定する) */
    private const val REVISION = "5359861c739e955e79d9a303bcbc70fb988958b1"

    /**
     * 多言語の base を 5bit に量子化したもの(約 60MB)。日本語と英語の両方を 1 つで読む。
     * tiny は日本語の聞き取りが粗く、base(非量子化)は 148MB と大きいので、この 2 つの間をとる。
     * モデルの重みは OpenAI Whisper(MIT)。
     */
    val BASE_Q5_1 = WhisperModelSpec(
        fileName = "ggml-base-q5_1.bin",
        url = "https://huggingface.co/ggerganov/whisper.cpp/resolve/$REVISION/ggml-base-q5_1.bin",
        sha256 = "422f1ae452ade6f30a004d7e5c6a43195e4433bc370bf23fac9cc591f01a8898",
        bytes = 59_707_625,
    )

    val DEFAULT = BASE_Q5_1
}
