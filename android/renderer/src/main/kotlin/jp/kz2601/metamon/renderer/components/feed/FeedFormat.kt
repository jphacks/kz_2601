package jp.kz2601.metamon.renderer.components.feed

import jp.kz2601.metamon.dsl.feed.FeedItem
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.roundToInt

/** feed の行の文字(iOS の `FeedLeaf` の static 関数)。Android に依らない(JVM で試す)。 */
object FeedFormat {
    private val jst: ZoneId = ZoneId.of("Asia/Tokyo")
    private val ja: Locale = Locale.JAPAN
    private val day = DateTimeFormatter.ofPattern("M/d(E)", ja)
    private val time = DateTimeFormatter.ofPattern("M/d H:mm", ja)

    /** 行が無い時の絵(SF Symbols の名前・[jp.kz2601.metamon.renderer.theme.SymbolIcon] が写す)。 */
    fun icon(provider: String?): String = when (provider) {
        "weather" -> "cloud.sun"
        "holidays" -> "calendar"
        "warnings" -> "exclamationmark.triangle"
        "pressure" -> "gauge.with.dots.needle.33percent"
        "arxiv" -> "doc.text"
        "diet" -> "building.columns"
        else -> "newspaper"
    }

    /** 「最高 / 最低」(片方だけでも出す)。 */
    fun temps(item: FeedItem): String? {
        fun f(v: Double) = "${v.roundToInt()}°"
        val max = item.max
        val min = item.min
        return when {
            max != null && min != null -> "${f(max)} / ${f(min)}"
            max != null -> f(max)
            min != null -> f(min)
            else -> null
        }
    }

    /** 気圧の「1006〜1013 hPa」(片方だけ・同じ値なら「1013 hPa」)。 */
    fun pressureRange(item: FeedItem): String? {
        fun f(v: Double) = "${v.roundToInt()}"
        val min = item.min
        val max = item.max
        return when {
            min != null && max != null -> if (f(min) == f(max)) "${f(max)} hPa" else "${f(min)}〜${f(max)} hPa"
            min != null -> "${f(min)} hPa"
            max != null -> "${f(max)} hPa"
            else -> null
        }
    }

    /** weather / holidays / pressure / diet は「11/7(土)」、rss / warnings / arxiv は「11/5 18:30」(日本時間・iOS と同じく日本語の曜日)。 */
    fun dateText(raw: String?, provider: String): String? {
        if (raw.isNullOrEmpty()) return null
        return if (provider == "rss" || provider == "warnings" || provider == "arxiv") {
            runCatching { OffsetDateTime.parse(raw).atZoneSameInstant(jst).format(time) }.getOrNull()
        } else {
            runCatching { LocalDate.parse(raw.take(10)).format(day) }.getOrNull()
        }
    }

    /** 小さな折れ線の点(0..1 の x, y・y は上が 0)。行の中の最低〜最高を高さいっぱいに。変化が無ければ真ん中の横線。 */
    fun sparkline(values: List<Double>): List<Pair<Float, Float>> {
        if (values.size < 2) return emptyList()
        val lo = values.min()
        val hi = values.max()
        val last = (values.size - 1).toFloat()
        return values.mapIndexed { i, v ->
            val y = if (hi > lo) (1 - (v - lo) / (hi - lo)).toFloat() else 0.5f
            i / last to y
        }
    }
}
