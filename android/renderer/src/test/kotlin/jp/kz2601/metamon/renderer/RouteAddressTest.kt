package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.renderer.components.route.AddressLookup
import jp.kz2601.metamon.renderer.components.route.RouteAddressCache
import jp.kz2601.metamon.renderer.components.route.RoutePlace
import jp.kz2601.metamon.renderer.components.route.place
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** routeGuide の住所 → 緯度経度(#1350): 結果の作り方・覚え方・距離に使う行き先。 */
class RouteAddressTest {
    @Test
    fun lookupFromGeocoderAnswer() {
        assertEquals(AddressLookup.Unavailable, AddressLookup.of(null))
        assertEquals(AddressLookup.NotFound, AddressLookup.of(emptyList()))
        assertEquals(AddressLookup.NotFound, AddressLookup.of(listOf(null)))
        assertEquals(AddressLookup.Found(36.56, 136.66), AddressLookup.of(listOf(null, 36.56 to 136.66)))
    }

    @Test
    fun cacheRemembersAnswersPerAddress() = runBlocking {
        val asked = mutableListOf<String>()
        val cache = RouteAddressCache { asked += it; if (it == "兼六園") AddressLookup.Found(36.56, 136.66) else AddressLookup.NotFound }
        assertNull(cache.peek("兼六園"))
        assertEquals(AddressLookup.Found(36.56, 136.66), cache.resolve("兼六園"))
        assertEquals(AddressLookup.Found(36.56, 136.66), cache.resolve("兼六園"))
        assertEquals(AddressLookup.NotFound, cache.resolve("どこでもない"))
        assertEquals(AddressLookup.NotFound, cache.resolve("どこでもない"))
        assertEquals(listOf("兼六園", "どこでもない"), asked)
        assertEquals(AddressLookup.Found(36.56, 136.66), cache.peek("兼六園"))
    }

    @Test
    fun unavailableIsAskedAgain() = runBlocking {
        var calls = 0
        val cache = RouteAddressCache { calls++; AddressLookup.Unavailable }
        assertEquals(AddressLookup.Unavailable, cache.resolve("兼六園"))
        assertEquals(AddressLookup.Unavailable, cache.resolve("兼六園"))
        assertEquals(2, calls)
        assertNull(cache.peek("兼六園"))
    }

    @Test
    fun onlyFoundGivesDistancePlace() {
        assertEquals(RoutePlace("address", "兼六園", 36.56, 136.66), AddressLookup.Found(36.56, 136.66).place("兼六園"))
        assertNull(AddressLookup.NotFound.place("兼六園"))
        assertNull(AddressLookup.Unavailable.place("兼六園"))
        assertNull((null as AddressLookup?).place("兼六園"))
    }
}
