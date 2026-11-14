package jp.kz2601.metamon.language

import org.junit.Assert.assertEquals
import org.junit.Test
import java.util.Locale

class AppLanguageTest {
    @Test
    fun explicitSettingWinsOverDevice() {
        assertEquals("en", AppLanguage.EN.resolve(listOf(Locale.JAPAN)))
        assertEquals("ja", AppLanguage.JA.resolve(listOf(Locale.US)))
    }

    @Test
    fun systemPicksFirstSupportedDeviceLanguageElseJapanese() {
        assertEquals("en", AppLanguage.SYSTEM.resolve(listOf(Locale.FRANCE, Locale.UK, Locale.JAPAN)))
        assertEquals("ja", AppLanguage.SYSTEM.resolve(listOf(Locale.JAPAN, Locale.US)))
        assertEquals("ja", AppLanguage.SYSTEM.resolve(listOf(Locale.KOREA)))
        assertEquals("ja", AppLanguage.SYSTEM.resolve(emptyList()))
    }

    @Test
    fun fromTagIgnoresRegionAndFallsBackToSystem() {
        assertEquals(AppLanguage.EN, AppLanguage.fromTag("en-GB"))
        assertEquals(AppLanguage.JA, AppLanguage.fromTag("ja-JP"))
        assertEquals(AppLanguage.JA, AppLanguage.fromTag("ja"))
        assertEquals(AppLanguage.SYSTEM, AppLanguage.fromTag("fr"))
        assertEquals(AppLanguage.SYSTEM, AppLanguage.fromTag(null))
        assertEquals(AppLanguage.SYSTEM, AppLanguage.fromTag(""))
    }
}
