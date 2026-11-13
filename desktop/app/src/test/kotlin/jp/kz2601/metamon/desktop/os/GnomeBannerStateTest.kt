package jp.kz2601.metamon.desktop.os

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class GnomeBannerStateTest {
    @Test
    fun parse() {
        assertEquals(true, GnomeBannerState.parse("true\n"))
        assertEquals(false, GnomeBannerState.parse(" false "))
        assertNull(GnomeBannerState.parse(null))
        assertNull(GnomeBannerState.parse(""))
        assertNull(GnomeBannerState.parse("No such schema 'org.gnome.desktop.notifications'"))
    }

    @Test
    fun holdsOnlyWhenBannersAreOff() {
        assertTrue(GnomeBannerState.isQuiet { "false\n" })
        assertFalse(GnomeBannerState.isQuiet { "true\n" })
    }

    @Test
    fun unreadableMeansShow() {
        assertFalse(GnomeBannerState.isQuiet { null })
        assertFalse(GnomeBannerState.isQuiet { "garbage" })
        assertFalse(GnomeBannerState.isQuiet { error("no gsettings") })
    }

    @Test
    fun callsGsettingsShowBanners() {
        var seen: List<String> = emptyList()
        GnomeBannerState.reader { seen = it.toList(); "true" }()
        assertEquals(listOf("gsettings", "get", "org.gnome.desktop.notifications", "show-banners"), seen)
    }
}
