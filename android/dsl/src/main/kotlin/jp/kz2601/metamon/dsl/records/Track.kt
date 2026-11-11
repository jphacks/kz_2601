package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import java.time.Duration
import java.time.Instant
import java.util.Locale
import kotlin.math.asin
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.sin
import kotlin.math.sqrt

/**
 * 位置の記録(`locationTracker`・#168)の点列と、stat の `distance` / `duration` の集計(iOS の Records/Track.swift の集計の部分)。
 * 1 点 = 1 レコード(location field "lat,lng" + 日時 field)。時刻順に並べ、10 分以上空いた所は別の記録として繋がない。
 * 記録中の間引き・自動停止は locationTracker を入れる時に足す。
 */
object Track {
    data class Point(val lat: Double, val lng: Double, val date: Instant)

    /** この秒数以上空いたら別の記録(10 分)。 */
    const val SESSION_GAP_SECONDS = 600.0

    /** stat の aggregate のうち軌跡を集計するもの。distance = 距離(m)、duration = 記録していた時間(秒)。 */
    fun isTrackAggregate(op: String?): Boolean = op == "distance" || op == "duration"

    /** "lat,lng" → 緯度経度(範囲外・壊れた値は null)。 */
    fun coordinate(v: JSONValue): Pair<Double, Double>? {
        val s = v.stringValue ?: return null
        val parts = s.split(',').filter { it.isNotEmpty() }.mapNotNull { SwiftNumbers.parseDouble(it.trim { c -> c == ' ' || c == '\t' }) }
        if (parts.size != 2 || parts[0] !in -90.0..90.0 || parts[1] !in -180.0..180.0) return null
        return parts[0] to parts[1]
    }

    /** レコード → 時刻順の点列。`by`(日時 field)が無い・読めない時は createdAt。どちらも無い点は捨てる。 */
    fun points(records: List<Record>, locationField: String, by: String?): List<Point> =
        records.mapNotNull { r ->
            val (lat, lng) = coordinate(r[locationField]) ?: return@mapNotNull null
            val d = by?.let { FieldCoding.date(r[it]) } ?: r.createdAt?.let { FieldCoding.date(JSONValue.Str(it)) } ?: return@mapNotNull null
            Point(lat, lng, d)
        }.sortedBy { it.date }

    /** 時刻順の点列を [gapSeconds] 以上空いた所で切る。 */
    fun segments(points: List<Point>, gapSeconds: Double = SESSION_GAP_SECONDS): List<List<Point>> {
        val out = mutableListOf<MutableList<Point>>()
        for (p in points) {
            val last = out.lastOrNull()?.lastOrNull()
            if (last != null && seconds(last.date, p.date) < gapSeconds) out.last().add(p) else out.add(mutableListOf(p))
        }
        return out
    }

    /** 2 点間の距離(m・haversine)。 */
    fun distance(a: Point, b: Point): Double {
        val r = 6_371_000.0
        val dLat = (b.lat - a.lat) * Math.PI / 180
        val dLng = (b.lng - a.lng) * Math.PI / 180
        val h = sin(dLat / 2) * sin(dLat / 2) + cos(a.lat * Math.PI / 180) * cos(b.lat * Math.PI / 180) * sin(dLng / 2) * sin(dLng / 2)
        return 2 * r * asin(min(1.0, sqrt(h)))
    }

    fun totalDistance(segments: List<List<Point>>): Double =
        segments.sumOf { seg -> seg.zipWithNext().sumOf { (a, b) -> distance(a, b) } }

    fun totalDuration(segments: List<List<Point>>): Double =
        segments.sumOf { seg -> if (seg.isEmpty()) 0.0 else seconds(seg.first().date, seg.last().date) }

    /** stat の数値(distance = m / duration = 秒)。 */
    fun aggregate(op: String?, records: List<Record>, locationField: String?, by: String?): Double {
        val segs = segments(points(records, locationField ?: "", by))
        return if (op == "duration") totalDuration(segs) else totalDistance(segs)
    }

    /** 850 m / 1.23 km */
    fun formatDistance(meters: Double): String =
        if (meters < 1000) Aggregator.cFormat(meters, 0) + " m" else Aggregator.cFormat(meters / 1000, 2) + " km"

    /** 1 時間未満は 分:秒(0:19 / 12:34)、以上は 時:分:秒(1:05:00)。言語に依存しない。 */
    fun formatDuration(seconds: Double): String {
        val s = maxOf(0.0, seconds).toLong()
        return if (s >= 3600) {
            String.format(Locale.ROOT, "%d:%02d:%02d", s / 3600, s / 60 % 60, s % 60)
        } else {
            String.format(Locale.ROOT, "%d:%02d", s / 60, s % 60)
        }
    }

    fun format(op: String?, value: Double): String = if (op == "duration") formatDuration(value) else formatDistance(value)

    private fun seconds(from: Instant, to: Instant): Double = Duration.between(from, to).toNanos() / 1e9
}
