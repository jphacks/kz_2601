package jp.kz2601.metamon.renderer.components.map

import android.graphics.Color
import android.graphics.RectF
import org.maplibre.android.maps.MapLibreMap
import org.maplibre.android.maps.Style
import org.maplibre.android.style.expressions.Expression
import org.maplibre.android.style.layers.CircleLayer
import org.maplibre.android.style.layers.LineLayer
import org.maplibre.android.style.layers.Property
import org.maplibre.android.style.layers.PropertyFactory
import org.maplibre.android.style.layers.SymbolLayer
import org.maplibre.android.style.sources.GeoJsonSource
import org.maplibre.geojson.Feature
import org.maplibre.geojson.FeatureCollection
import org.maplibre.geojson.LineString
import org.maplibre.geojson.Point

/** スタイルの上に重ねるピン・軌跡の層(GeoJSON のソース + 円・字・線の層)。 */
internal object MapLayers {
    private const val PINS = "metamon-pins"
    const val PIN_CIRCLES = "metamon-pins-circle"
    private const val PIN_LABELS = "metamon-pins-label"
    private const val ROUTE_PAST = "metamon-route-past"
    private const val ROUTE_LATEST = "metamon-route-latest"
    private const val ROUTE_START = "metamon-route-start"
    private const val ROUTE_END = "metamon-route-end"

    private fun point(at: LatLng): Point = Point.fromLngLat(at.lng, at.lat)

    private fun Style.source(id: String, features: List<Feature>): Boolean {
        val fc = FeatureCollection.fromFeatures(features)
        val existing = getSourceAs<GeoJsonSource>(id)
        if (existing != null) {
            existing.setGeoJson(fc)
            return false
        }
        addSource(GeoJsonSource(id, fc))
        return true
    }

    /** ピン(記録 1 件 = 1 本)。名前は下に小さく出す。 */
    fun pins(style: Style, pins: List<MapPin>, tint: Int) {
        val features = pins.map { p ->
            Feature.fromGeometry(point(p.at)).apply {
                addStringProperty("id", p.id)
                addStringProperty("title", p.title)
            }
        }
        if (!style.source(PINS, features)) return
        style.addLayer(
            CircleLayer(PIN_CIRCLES, PINS).withProperties(
                PropertyFactory.circleRadius(9f),
                PropertyFactory.circleColor(tint),
                PropertyFactory.circleStrokeColor(Color.WHITE),
                PropertyFactory.circleStrokeWidth(2.5f),
            ),
        )
        style.addLayer(
            SymbolLayer(PIN_LABELS, PINS).withProperties(
                PropertyFactory.textField(Expression.get("title")),
                PropertyFactory.textFont(MapStyle.FONT),
                PropertyFactory.textSize(12f),
                PropertyFactory.textAnchor(Property.TEXT_ANCHOR_TOP),
                PropertyFactory.textOffset(arrayOf(0f, 1.1f)),
                PropertyFactory.textOptional(true),
                PropertyFactory.textColor(Color.rgb(0x20, 0x20, 0x20)),
                PropertyFactory.textHaloColor(Color.WHITE),
                PropertyFactory.textHaloWidth(1.5f),
            ),
        )
    }

    /** 押した点の近く(±[slopPx])のピンの記録 id。 */
    fun pinAt(map: MapLibreMap, x: Float, y: Float, slopPx: Float): String? =
        map.queryRenderedFeatures(RectF(x - slopPx, y - slopPx, x + slopPx, y + slopPx), PIN_CIRCLES)
            .firstNotNullOfOrNull { f -> f.getStringProperty("id") }

    /** 軌跡: 最新の記録は濃く、前の記録は薄く。最新の記録の始点(緑)と終点に印。 */
    fun route(style: Style, segments: List<List<LatLng>>, tint: Int, startTint: Int) {
        val latest = segments.lastOrNull().orEmpty()
        fun line(seg: List<LatLng>) = Feature.fromGeometry(LineString.fromLngLats(seg.map(::point)))
        val added = style.source(ROUTE_PAST, segments.dropLast(1).filter { it.size > 1 }.map(::line))
        style.source(ROUTE_LATEST, listOfNotNull(latest.takeIf { it.size > 1 }?.let(::line)))
        style.source(ROUTE_START, listOfNotNull(latest.firstOrNull()?.let { Feature.fromGeometry(point(it)) }))
        style.source(ROUTE_END, listOfNotNull(latest.takeIf { it.size > 1 }?.lastOrNull()?.let { Feature.fromGeometry(point(it)) }))
        if (!added) return
        for ((id, opacity) in listOf(ROUTE_PAST to 0.4f, ROUTE_LATEST to 1f)) {
            style.addLayer(
                LineLayer(id, id).withProperties(
                    PropertyFactory.lineColor(tint),
                    PropertyFactory.lineOpacity(opacity),
                    PropertyFactory.lineWidth(5f),
                    PropertyFactory.lineCap(Property.LINE_CAP_ROUND),
                    PropertyFactory.lineJoin(Property.LINE_JOIN_ROUND),
                ),
            )
        }
        for ((id, color) in listOf(ROUTE_START to startTint, ROUTE_END to tint)) {
            style.addLayer(
                CircleLayer(id, id).withProperties(
                    PropertyFactory.circleRadius(8f),
                    PropertyFactory.circleColor(color),
                    PropertyFactory.circleStrokeColor(Color.WHITE),
                    PropertyFactory.circleStrokeWidth(2.5f),
                ),
            )
        }
    }
}
