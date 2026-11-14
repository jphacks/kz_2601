package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Zoom
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.map.LatLng
import jp.kz2601.metamon.renderer.components.map.MapCamera
import jp.kz2601.metamon.renderer.components.map.MapRules
import jp.kz2601.metamon.renderer.components.map.MapStyle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Locale

class MapRulesTest {
    private fun rec(id: String, place: String?, name: String = id, at: String? = null) = Record(
        id,
        buildMap {
            place?.let { put("place", JSONValue.Str(it)) }
            put("name", JSONValue.Str(name))
            at?.let { put("at", JSONValue.Str(it)) }
        },
    )

    @Test
    fun pinsOnlyForReadableCoordinates() {
        val records = listOf(rec("a", "36.5613,136.6562"), rec("b", "bad"), rec("c", null), rec("d", "91,0"), rec("e", " 35.0 , 139.0 "))
        val pins = MapRules.pins(records, "place") { it["name"].stringValue.orEmpty() }
        assertEquals(listOf("a", "e"), pins.map { it.id })
        assertEquals(LatLng(36.5613, 136.6562), pins[0].at)
        assertTrue(MapRules.pins(records, null) { "" }.isEmpty())
    }

    @Test
    fun zoomSpansMatchIos() {
        assertEquals(1_000.0, MapRules.spanMeters(Zoom.NEAR), 0.0)
        assertEquals(10_000.0, MapRules.spanMeters(null), 0.0)
        assertEquals(10_000.0, MapRules.spanMeters(Zoom.CITY), 0.0)
        assertEquals(100_000.0, MapRules.spanMeters(Zoom.WIDE), 0.0)
        assertEquals(500.0, MapRules.routeMinSpan(null), 0.0)
        assertEquals(1_000.0, MapRules.routeMinSpan(Zoom.NEAR), 0.0)
    }

    @Test
    fun cameraCentersOnPinsOrTokyo() {
        assertEquals(MapCamera(MapRules.TOKYO, 10_000.0), MapRules.camera(emptyList(), 10_000.0))
        val c = MapRules.camera(listOf(LatLng(36.0, 136.0), LatLng(38.0, 138.0)), 1_000.0)
        assertEquals(37.0, c.center.lat, 1e-9)
        assertEquals(137.0, c.center.lng, 1e-9)
    }

    @Test
    fun fitKeepsMinSpanAndCoversPoints() {
        val near = MapRules.fit(listOf(LatLng(36.0, 136.0), LatLng(36.0001, 136.0001)), 500.0)
        assertEquals(500.0, near.spanMeters, 0.0)
        val far = MapRules.fit(listOf(LatLng(36.0, 136.0), LatLng(36.1, 136.0)), 500.0)
        assertEquals(0.1 * 111_000 * 1.4, far.spanMeters, 1.0)
        assertEquals(36.05, far.center.lat, 1e-9)
    }

    @Test
    fun zoomShowsTheSpanOnTheShortSide() {
        // 赤道で 512 dp に地球一周 → zoom 0
        assertEquals(0.0, MapRules.zoom(MapCamera(LatLng(0.0, 0.0), 40_075_016.686), 512.0), 1e-6)
        // 幅を半分にすると 1 段上がる
        val z1 = MapRules.zoom(MapCamera(LatLng(36.0, 136.0), 10_000.0), 300.0)
        val z2 = MapRules.zoom(MapCamera(LatLng(36.0, 136.0), 5_000.0), 300.0)
        assertEquals(1.0, z2 - z1, 1e-9)
        assertTrue(z1 in 10.0..12.0)
        assertEquals(MapRules.MAX_ZOOM, MapRules.zoom(MapCamera(LatLng(36.0, 136.0), 1.0), 300.0), 0.0)
    }

    @Test
    fun routeSegmentsSplitOnTenMinuteGaps() {
        val dsl = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"w","voiceEntry":"none"},"initialScreen":"s","entities":[{"id":"p","name":"点","fields":[{"id":"place","name":"位置","type":"location"},{"id":"at","name":"時刻","type":"datetime"}]}],
               "screens":[{"id":"s","title":"t","components":[{"type":"mapView","entity":"p","kind":"route","locationField":"place","dateField":"at"}]}]}""",
        )
        val c = dsl.screens.first().components.first()
        assertTrue(MapRules.isRoute(c))
        val records = listOf(
            rec("3", "36.2,136.2", at = "2026-11-12T10:30:00+09:00"),
            rec("1", "36.0,136.0", at = "2026-11-12T10:00:00+09:00"),
            rec("2", "36.1,136.1", at = "2026-11-12T10:05:00+09:00"),
        )
        val segs = MapRules.segments(records, c)
        assertEquals(listOf(2, 1), segs.map { it.size })
        assertEquals(LatLng(36.0, 136.0), segs[0][0])
    }

    @Test
    fun applyWritesCoordinateAndNameOnlyWhenNamed() {
        val values = mutableMapOf<String, JSONValue>("name" to JSONValue.Str("打った名前"))
        MapRules.apply(LatLng(36.5613, 136.6562), null, values, "place", "name")
        assertEquals("36.561300,136.656200", values["place"]?.stringValue)
        assertEquals("ピンで選んだ時は打った名前を消さない", "打った名前", values["name"]?.stringValue)
        MapRules.apply(LatLng(36.0, 136.0), " 兼六園 ", values, "place", "name")
        assertEquals("兼六園", values["name"]?.stringValue)
        MapRules.apply(LatLng(36.0, 136.0), "兼六園", values, "place", "place")
        assertEquals("36.000000,136.000000", values["place"]?.stringValue)
    }

    @Test
    fun valueUsesDotInAnyLocale() {
        val saved = Locale.getDefault()
        try {
            Locale.setDefault(Locale.GERMANY)
            assertEquals("36.561300,136.656200", MapRules.value(LatLng(36.5613, 136.6562)))
        } finally {
            Locale.setDefault(saved)
        }
        assertNull(MapRules.coordinate(JSONValue.Str("36.5;136.6")))
    }

    @Test
    fun tilesComeFromAKeylessProviderWithOsmAttribution() {
        for (url in listOf(MapStyle.LIGHT, MapStyle.DARK)) {
            assertTrue(url, url.startsWith("https://tiles.openfreemap.org/styles/"))
            assertFalse("OSM 財団のタイルは使わない", "tile.openstreetmap.org" in url)
            assertFalse("キーを持たない", "key=" in url)
        }
        assertTrue("© OpenStreetMap contributors" in MapStyle.ATTRIBUTION)
    }
}
