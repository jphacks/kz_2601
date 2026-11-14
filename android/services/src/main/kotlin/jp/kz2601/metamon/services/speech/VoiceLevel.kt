package jp.kz2601.metamon.services.speech

/**
 * 録音中の声の大きさ(#1014)。`RecognitionListener.onRmsChanged` の dB を 0..1 にして、マイクの周りの輪に出す。
 * 端末内の認識器の rmsdB はおおむね -2(無音)〜 10(近くで話す)。
 */
object VoiceLevel {
    const val SILENT_DB = -2f
    const val LOUD_DB = 10f

    /** dB → 0..1(範囲外は端に寄せる・NaN は 0)。 */
    fun fromRmsDb(rmsDb: Float): Float {
        if (rmsDb.isNaN()) return 0f
        return ((rmsDb - SILENT_DB) / (LOUD_DB - SILENT_DB)).coerceIn(0f, 1f)
    }

    /** 上がる時はすぐ、下がる時はゆっくり(輪がちらつかない)。 */
    fun smooth(previous: Float, next: Float): Float =
        if (next >= previous) next else previous + (next - previous) * RELEASE

    private const val RELEASE = 0.3f
}
