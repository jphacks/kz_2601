package jp.kz2601.metamon.renderer.components.route

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.Track
import java.net.URLEncoder
import java.util.Locale
import kotlin.math.asin
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.roundToLong
import kotlin.math.sin
import kotlin.math.sqrt

/** 車 / 徒歩(iOS の `RouteGuidance.Mode`)。省略・不明は車。 */
enum class RouteMode {
    AUTOMOBILE, WALKING;

    companion object {
        fun of(mode: ComponentMode?): RouteMode = if (mode == ComponentMode.WALKING) WALKING else AUTOMOBILE
    }
}

/** 行き先 1 件(レコードの location field・iOS の `RouteGuidance.Place`)。 */
data class RoutePlace(val id: String, val name: String, val lat: Double, val lng: Double)

/** 地図アプリに渡す行き先: 緯度経度か、住所・施設名の文字列。 */
sealed interface RouteTarget {
    data class Coord(val place: RoutePlace) : RouteTarget
    data class Query(val text: String) : RouteTarget
}

/**
 * `routeGuide`(#868)の Android の決まり(Compose・Intent に依らない部分。JVM で試す)。
 * Android 版は地図 SDK を持たず、案内は端末の地図アプリ(Google マップなど)に渡す。目的地の決め方は iOS の
 * `RouteGuideLeaf` と同じ: `address` > `entity`(画面の entity と別)+ `locationField` > フォームの `locationField`。
 */
object RouteGuide {
    fun address(c: Component): String? = c.address?.trim()?.takeIf { it.isNotEmpty() }

    /** 部品の中で行き先を選ぶ(entity が画面の entity と別)。 */
    fun choosesRecord(c: Component, formEntity: String?): Boolean {
        val e = c.entity?.takeIf { it.isNotEmpty() } ?: return false
        return e != formEntity
    }

    /** 目的地を決められない置き方(iOS で「目的地が必要です」の枠になる条件)。 */
    fun isMisconfigured(c: Component, formEntity: String?, hasForm: Boolean): Boolean =
        address(c) == null && (c.locationField == null || (!choosesRecord(c, formEntity) && !hasForm))

    /** レコードの値から行き先を作る。場所が無い・壊れていれば null。 */
    fun place(id: String, values: Map<String, JSONValue>, locationField: String, titleField: String?): RoutePlace? {
        val (lat, lng) = Track.coordinate(values[locationField] ?: JSONValue.Null) ?: return null
        val name = titleField?.let { values[it]?.stringValue }?.trim().orEmpty()
        return RoutePlace(id, name, lat, lng)
    }

    /** 場所を持つレコードを候補にする(並びはレコードの順)。 */
    fun places(records: List<Record>, locationField: String, titleField: String?): List<RoutePlace> =
        records.mapNotNull { place(it.id, it.data, locationField, titleField) }

    /**
     * 地図アプリに渡す URI(先頭から試し、開けなければ次)。
     * 1. `google.navigation:` … Google マップのナビをすぐ始める
     * 2. `geo:` … 端末の地図アプリ(Google マップ以外も)で行き先を出す
     * 3. `https://www.google.com/maps/dir/` … 地図アプリが無ければブラウザで経路
     */
    fun uris(target: RouteTarget, mode: RouteMode): List<String> {
        val navMode = if (mode == RouteMode.WALKING) "w" else "d"
        val travel = if (mode == RouteMode.WALKING) "walking" else "driving"
        return when (target) {
            is RouteTarget.Coord -> {
                val ll = "${coord(target.place.lat)},${coord(target.place.lng)}"
                val label = target.place.name.takeIf { it.isNotEmpty() }?.let { "(${encode(it)})" }.orEmpty()
                listOf(
                    "google.navigation:q=$ll&mode=$navMode",
                    "geo:$ll?q=$ll$label",
                    "https://www.google.com/maps/dir/?api=1&destination=$ll&travelmode=$travel",
                )
            }
            is RouteTarget.Query -> {
                val q = encode(target.text)
                listOf(
                    "google.navigation:q=$q&mode=$navMode",
                    "geo:0,0?q=$q",
                    "https://www.google.com/maps/dir/?api=1&destination=$q&travelmode=$travel",
                )
            }
        }
    }

    /** 2 点間の直線距離(m・haversine)。 */
    fun distanceMeters(lat1: Double, lng1: Double, lat2: Double, lng2: Double): Double {
        val r = 6_371_000.0
        val dLat = Math.toRadians(lat2 - lat1)
        val dLng = Math.toRadians(lng2 - lng1)
        val h = sin(dLat / 2) * sin(dLat / 2) + cos(Math.toRadians(lat1)) * cos(Math.toRadians(lat2)) * sin(dLng / 2) * sin(dLng / 2)
        return 2 * r * asin(min(1.0, sqrt(h)))
    }

    /** 丸め(iOS の `RouteGuidance.rounded`): 100 m 未満は 10 m・1 km 未満は 50 m・それ以上は 100 m 単位。 */
    fun rounded(meters: Double): Double {
        val m = maxOf(0.0, meters)
        if (m < 100) return maxOf(10.0, roundHalfUp(m / 10) * 10)
        if (m < 1000) return roundHalfUp(m / 50) * 50
        return roundHalfUp(m / 100) * 100
    }

    /** 「350 m」「1.2 km」(言語に依存しない・iOS の `RouteGuidance.formatDistance`)。 */
    fun formatDistance(meters: Double): String {
        val r = rounded(meters)
        return if (r < 1000) String.format(Locale.ROOT, "%.0f m", r) else String.format(Locale.ROOT, "%.1f km", r / 1000)
    }

    private fun roundHalfUp(x: Double): Double = x.roundToLong().toDouble()

    private fun coord(v: Double): String = String.format(Locale.ROOT, "%.6f", v).trimEnd('0').trimEnd('.')

    private fun encode(s: String): String = URLEncoder.encode(s, Charsets.UTF_8.name()).replace("+", "%20")
}
