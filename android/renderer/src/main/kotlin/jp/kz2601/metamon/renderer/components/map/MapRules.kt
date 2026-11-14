package jp.kz2601.metamon.renderer.components.map

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentKind
import jp.kz2601.metamon.dsl.Zoom
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.Track
import java.util.Locale
import kotlin.math.cos
import kotlin.math.ln
import kotlin.math.max

/** 緯度経度。 */
data class LatLng(val lat: Double, val lng: Double)

/** 地図の 1 本のピン(記録 1 件)。 */
data class MapPin(val id: String, val title: String, val at: LatLng)

/** 見せる範囲: 中心と、縦横どちらにも入れる幅(m)。 */
data class MapCamera(val center: LatLng, val spanMeters: Double)

/**
 * mapView / locationPicker の規則(iOS の `MapViewLeaf` / `PlacePicking`)。地図の SDK に触らないので JVM で試せる。
 */
object MapRules {
    /** ピンが無い時の中心(東京駅・iOS と同じ)。 */
    val TOKYO = LatLng(35.6812, 139.7671)

    /** "lat,lng" → 座標(範囲外・壊れた値は null)。 */
    fun coordinate(v: JSONValue?): LatLng? = v?.let(Track::coordinate)?.let { (lat, lng) -> LatLng(lat, lng) }

    /** 保存する形(locationButton と同じ "lat,lng"・小数 6 桁)。 */
    fun value(at: LatLng): String = String.format(Locale.US, "%.6f,%.6f", at.lat, at.lng)

    fun isRoute(c: Component): Boolean = c.kind == ComponentKind.ROUTE

    /** zoom: near 約 1km / city 約 10km(既定)/ wide 約 100km。 */
    fun spanMeters(zoom: Zoom?): Double = when (zoom) {
        Zoom.NEAR -> 1_000.0
        Zoom.WIDE -> 100_000.0
        else -> 10_000.0
    }

    /** route の最小の広さ: zoom 省略時は約 500 m(散歩の線が見える)。 */
    fun routeMinSpan(zoom: Zoom?): Double = if (zoom == null) 500.0 else spanMeters(zoom)

    /** `locationField` が読めるレコードだけピンにする。[title] は見出しの表示。 */
    fun pins(records: List<Record>, locationField: String?, title: (Record) -> String): List<MapPin> {
        val f = locationField ?: return emptyList()
        return records.mapNotNull { r -> coordinate(r[f])?.let { MapPin(r.id, title(r), it) } }
    }

    /** ピンの平均を中心に [span] の幅(ピンが無ければ東京駅)。 */
    fun camera(points: List<LatLng>, span: Double): MapCamera {
        if (points.isEmpty()) return MapCamera(TOKYO, span)
        return MapCamera(LatLng(points.map { it.lat }.average(), points.map { it.lng }.average()), span)
    }

    /** 点列全体が入る範囲(余白 1.4 倍)。[minSpan] より狭くはしない。 */
    fun fit(points: List<LatLng>, minSpan: Double): MapCamera {
        if (points.isEmpty()) return camera(points, minSpan)
        val minLat = points.minOf { it.lat }
        val maxLat = points.maxOf { it.lat }
        val minLng = points.minOf { it.lng }
        val maxLng = points.maxOf { it.lng }
        val center = LatLng((minLat + maxLat) / 2, (minLng + maxLng) / 2)
        val latMeters = (maxLat - minLat) * 111_000 * 1.4
        val lngMeters = (maxLng - minLng) * 111_000 * cos(Math.toRadians(center.lat)) * 1.4
        return MapCamera(center, max(minSpan, max(latMeters, lngMeters)))
    }

    /** route: 時刻順に並べて 10 分の空きで切った点列。 */
    fun segments(records: List<Record>, c: Component): List<List<LatLng>> =
        Track.segments(Track.points(records, c.locationField.orEmpty(), c.dateField)).map { s -> s.map { LatLng(it.lat, it.lng) } }

    /**
     * [camera] の幅が地図の短い辺([viewDp])に収まる MapLibre の zoom。MapLibre は zoom z で世界一周が 512·2^z dp。
     */
    fun zoom(camera: MapCamera, viewDp: Double): Double {
        val metersPerDpAtZero = EARTH_CIRCUMFERENCE * cos(Math.toRadians(camera.center.lat)) / 512.0
        val z = ln(metersPerDpAtZero * viewDp.coerceAtLeast(1.0) / camera.spanMeters.coerceAtLeast(1.0)) / ln(2.0)
        return z.coerceIn(0.0, MAX_ZOOM)
    }

    /** locationPicker: 選んだ場所を書く。名前があり nameField があればそこにも(名前の無い時は打った名前を消さない)。 */
    fun apply(at: LatLng, name: String?, values: MutableMap<String, JSONValue>, field: String, nameField: String?) {
        values[field] = JSONValue.Str(value(at))
        val n = name?.trim()
        if (!nameField.isNullOrEmpty() && nameField != field && !n.isNullOrEmpty()) values[nameField] = JSONValue.Str(n)
    }

    private const val EARTH_CIRCUMFERENCE = 40_075_016.686
    const val MAX_ZOOM = 19.0
}
