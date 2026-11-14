package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.route.RouteGuide
import jp.kz2601.metamon.renderer.components.route.RouteMode
import jp.kz2601.metamon.renderer.components.route.RoutePlace
import jp.kz2601.metamon.renderer.components.route.RouteTarget
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** routeGuide(#868): 目的地の決め方・地図アプリに渡す URI・距離の表示。 */
class RouteGuideTest {
    private val kanazawa = RoutePlace("r1", "金沢駅", 36.578268, 136.648035)

    @Test
    fun registered() {
        assertTrue(ComponentType.ROUTE_GUIDE.wire in ComponentRegistry.standard.registeredTypes)
    }

    @Test
    fun modeDefaultsToCar() {
        assertEquals(RouteMode.AUTOMOBILE, RouteMode.of(null))
        assertEquals(RouteMode.AUTOMOBILE, RouteMode.of(ComponentMode.UNKNOWN))
        assertEquals(RouteMode.WALKING, RouteMode.of(ComponentMode.WALKING))
    }

    @Test
    fun destinationSources() {
        val address = Component(type = ComponentType.ROUTE_GUIDE, address = "  兼六園 ")
        assertEquals("兼六園", RouteGuide.address(address))
        assertFalse(RouteGuide.isMisconfigured(address, formEntity = null, hasForm = false))

        val pick = Component(type = ComponentType.ROUTE_GUIDE, entity = "spot", locationField = "place")
        assertTrue(RouteGuide.choosesRecord(pick, formEntity = null))
        assertFalse(RouteGuide.isMisconfigured(pick, formEntity = null, hasForm = false))
        // 画面と同じ entity はフォームの今のレコード
        assertFalse(RouteGuide.choosesRecord(pick, formEntity = "spot"))

        val formOnly = Component(type = ComponentType.ROUTE_GUIDE, locationField = "place")
        assertTrue(RouteGuide.isMisconfigured(formOnly, formEntity = null, hasForm = false))
        assertFalse(RouteGuide.isMisconfigured(formOnly, formEntity = "spot", hasForm = true))

        assertTrue(RouteGuide.isMisconfigured(Component(type = ComponentType.ROUTE_GUIDE, address = " "), null, false))
    }

    @Test
    fun placesSkipRecordsWithoutLocation() {
        val records = listOf(
            Record(id = "a", data = mapOf("place" to JSONValue.Str("36.5,136.6"), "name" to JSONValue.Str(" 家 "))),
            Record(id = "b", data = mapOf("name" to JSONValue.Str("場所なし"))),
            Record(id = "c", data = mapOf("place" to JSONValue.Str("999,0"))),
        )
        assertEquals(listOf(RoutePlace("a", "家", 36.5, 136.6)), RouteGuide.places(records, "place", "name"))
        assertNull(RouteGuide.place("x", emptyMap(), "place", null))
    }

    @Test
    fun coordinateUrisTryNavigationThenGeoThenBrowser() {
        assertEquals(
            listOf(
                "google.navigation:q=36.578268,136.648035&mode=d",
                "geo:36.578268,136.648035?q=36.578268,136.648035(%E9%87%91%E6%B2%A2%E9%A7%85)",
                "https://www.google.com/maps/dir/?api=1&destination=36.578268,136.648035&travelmode=driving",
            ),
            RouteGuide.uris(RouteTarget.Coord(kanazawa), RouteMode.AUTOMOBILE),
        )
        val walk = RouteGuide.uris(RouteTarget.Coord(RoutePlace("p", "", 36.5, -0.25)), RouteMode.WALKING)
        assertEquals("google.navigation:q=36.5,-0.25&mode=w", walk[0])
        assertEquals("geo:36.5,-0.25?q=36.5,-0.25", walk[1])
        assertTrue(walk[2].endsWith("&travelmode=walking"))
    }

    @Test
    fun addressUrisAreEncoded() {
        assertEquals(
            listOf(
                "google.navigation:q=Kenroku%20en%20%26%20Castle&mode=w",
                "geo:0,0?q=Kenroku%20en%20%26%20Castle",
                "https://www.google.com/maps/dir/?api=1&destination=Kenroku%20en%20%26%20Castle&travelmode=walking",
            ),
            RouteGuide.uris(RouteTarget.Query("Kenroku en & Castle"), RouteMode.WALKING),
        )
    }

    @Test
    fun distanceFormatMatchesIos() {
        assertEquals("10 m", RouteGuide.formatDistance(0.0))
        assertEquals("40 m", RouteGuide.formatDistance(37.0))
        assertEquals("350 m", RouteGuide.formatDistance(340.0))
        assertEquals("1.0 km", RouteGuide.formatDistance(990.0))
        assertEquals("1.2 km", RouteGuide.formatDistance(1_234.0))
        // 金沢駅 → 兼六園 はおよそ 2 km
        val d = RouteGuide.distanceMeters(36.578268, 136.648035, 36.562128, 136.662586)
        assertTrue(d in 2_000.0..2_400.0)
    }
}
