package jp.kz2601.metamon.whisper

/** whisper に渡す音: 16kHz・モノラル・float(-1..1)。 */
object WhisperAudio {
    const val SAMPLE_RATE = 16_000

    /** 1 回に文字にする長さ(秒)。Whisper の窓と同じ 30 秒(iOS の `AudioFileChunking.blockSeconds`)。 */
    const val BLOCK_SECONDS = 30

    const val BLOCK_SAMPLES = SAMPLE_RATE * BLOCK_SECONDS

    /** これ未満の音は文字にしない(Whisper が幻聴を出しやすい・iOS の `minimumSeconds`) */
    const val MINIMUM_SECONDS = 0.5

    /** ピークがこれ未満(約 -60dBFS)なら無音(iOS の `silencePeak`) */
    const val SILENCE_PEAK = 0.001f

    /** 16bit の PCM(チャンネルが交互に並ぶ)→ モノラルの float。 */
    fun monoFromPcm16(pcm: ShortArray, channels: Int): FloatArray {
        val ch = channels.coerceAtLeast(1)
        val frames = pcm.size / ch
        return FloatArray(frames) { f ->
            var sum = 0f
            for (c in 0 until ch) sum += pcm[f * ch + c] / 32768f
            sum / ch
        }
    }

    /** float の PCM(チャンネルが交互)→ モノラル。 */
    fun monoFromFloat(pcm: FloatArray, channels: Int): FloatArray {
        val ch = channels.coerceAtLeast(1)
        if (ch == 1) return pcm.copyOf()
        val frames = pcm.size / ch
        return FloatArray(frames) { f ->
            var sum = 0f
            for (c in 0 until ch) sum += pcm[f * ch + c]
            sum / ch
        }
    }

    fun peak(samples: FloatArray): Float = samples.maxOfOrNull { kotlin.math.abs(it) } ?: 0f

    /** 文字にする価値のある音か(短すぎ・無音は飛ばす)。 */
    fun hasVoice(samples: FloatArray): Boolean =
        samples.size >= SAMPLE_RATE * MINIMUM_SECONDS && peak(samples) >= SILENCE_PEAK

    /** 区切りごとの文字をつなぐ(空は飛ばす・iOS と同じく改行で)。 */
    fun join(parts: List<String>): String = parts.map { it.trim() }.filter { it.isNotEmpty() }.joinToString("\n")
}

/**
 * 任意の速さ → 16kHz の線形補間。区切って渡しても前の区切りの続きとしてつなぐ(ファイル全体をメモリに載せない)。
 */
class StreamingResampler(private val fromRate: Int, private val toRate: Int = WhisperAudio.SAMPLE_RATE) {
    init {
        require(fromRate > 0 && toRate > 0)
    }

    private val step = fromRate.toDouble() / toRate

    /** 次に出す点の位置([prev] があればそれを 0 とした座標)。 */
    private var position = 0.0

    /** 前の区切りの最後の 1 点(区切りをまたぐ補間に使う)。 */
    private var prev: Float? = null

    fun process(input: FloatArray): FloatArray {
        if (fromRate == toRate) return input.copyOf()
        if (input.isEmpty()) return FloatArray(0)
        val p = prev
        val buf = if (p != null) floatArrayOf(p) + input else input
        val out = FloatArray(((buf.size - position) / step).toInt() + 2)
        var n = 0
        while (true) {
            val i = position.toInt()
            // 右の点が次の区切りに入る時は、次の区切りで出す
            if (i + 1 >= buf.size) break
            val frac = (position - i).toFloat()
            out[n++] = buf[i] + (buf[i + 1] - buf[i]) * frac
            position += step
        }
        position -= (buf.size - 1)
        prev = buf.last()
        return out.copyOf(n)
    }
}

/** 16kHz の float を [WhisperAudio.BLOCK_SAMPLES] ごとの区切りにまとめる。 */
class BlockAccumulator(private val blockSamples: Int = WhisperAudio.BLOCK_SAMPLES) {
    private var buffer = FloatArray(blockSamples)
    private var size = 0

    /** [samples] を足し、満ちた区切りを返す(0 個以上)。 */
    fun add(samples: FloatArray): List<FloatArray> {
        val full = mutableListOf<FloatArray>()
        var offset = 0
        while (offset < samples.size) {
            val n = minOf(blockSamples - size, samples.size - offset)
            System.arraycopy(samples, offset, buffer, size, n)
            size += n
            offset += n
            if (size == blockSamples) {
                full += buffer
                buffer = FloatArray(blockSamples)
                size = 0
            }
        }
        return full
    }

    /** 残り(満ちていない最後の区切り)。無ければ null。 */
    fun finish(): FloatArray? = if (size == 0) null else buffer.copyOf(size).also { size = 0 }
}
