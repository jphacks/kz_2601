package jp.kz2601.metamon.feedback

import kotlin.math.sqrt

/**
 * 端末を振ったかの判定(iOS の `motionShake` に当たる)。加速度(m/s²・重力込み)を 1 つずつ渡す。
 * 強い揺れ([thresholdG] 倍の重力を超える)が [windowMillis] の中に [hits] 回あれば 1 回の「振った」。
 * 判定の後 [cooldownMillis] は次を数えない(1 回振って 2 回開かない)。端末を置く・歩く程度では超えない強さにしてある。
 */
class ShakeDetector(
    private val thresholdG: Float = 2.5f,
    private val hits: Int = 3,
    private val windowMillis: Long = 700,
    private val cooldownMillis: Long = 1_500,
) {
    private val peaks = ArrayDeque<Long>()
    private var lastPeak = Long.MIN_VALUE
    private var lastShake = Long.MIN_VALUE

    /** 1 回分の加速度。振ったと判定した時だけ true。 */
    fun onSample(x: Float, y: Float, z: Float, timeMillis: Long): Boolean {
        if (lastShake != Long.MIN_VALUE && timeMillis - lastShake < cooldownMillis) return false
        val g = sqrt(x * x + y * y + z * z) / GRAVITY
        if (g < thresholdG) return false
        // 同じ山の続きのサンプル(50ms 以内)は 1 回と数える
        if (lastPeak != Long.MIN_VALUE && timeMillis - lastPeak < PEAK_GAP_MILLIS) return false
        lastPeak = timeMillis
        peaks.addLast(timeMillis)
        while (peaks.isNotEmpty() && timeMillis - peaks.first() > windowMillis) peaks.removeFirst()
        if (peaks.size < hits) return false
        peaks.clear()
        lastShake = timeMillis
        return true
    }

    fun reset() {
        peaks.clear()
        lastPeak = Long.MIN_VALUE
        lastShake = Long.MIN_VALUE
    }

    private companion object {
        const val GRAVITY = 9.80665f
        const val PEAK_GAP_MILLIS = 50L
    }
}
