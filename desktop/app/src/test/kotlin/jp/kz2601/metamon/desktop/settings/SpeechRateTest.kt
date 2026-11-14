package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Test

/** 読み上げの速さ: 3 段から SAPI・spd-say の値への写しと保存(#3002)。 */
class SpeechRateTest {
    @Test
    fun mapsToSapiRate() {
        assertEquals(-3, SpeechRate.SLOW.sapiRate)
        assertEquals(0, SpeechRate.NORMAL.sapiRate)
        assertEquals(3, SpeechRate.FAST.sapiRate)
    }

    @Test
    fun mapsToSpdSayRate() {
        assertEquals(-40, SpeechRate.SLOW.spdSayRate)
        assertEquals(0, SpeechRate.NORMAL.spdSayRate)
        assertEquals(40, SpeechRate.FAST.spdSayRate)
    }

    @Test
    fun valuesStayInEachRange() {
        SpeechRate.entries.forEach {
            assertEquals(true, it.sapiRate in -10..10)
            assertEquals(true, it.spdSayRate in -100..100)
        }
    }

    @Test
    fun parseFallsBackToNormal() {
        assertEquals(SpeechRate.FAST, SpeechRate.parse(" Fast "))
        assertEquals(SpeechRate.SLOW, SpeechRate.parse("slow"))
        assertEquals(SpeechRate.NORMAL, SpeechRate.parse(null))
        assertEquals(SpeechRate.NORMAL, SpeechRate.parse("turbo"))
    }

    @Test
    fun savesAndLoads() {
        val store = JsonKeyValueStore(null)
        assertEquals(SpeechRate.NORMAL, SpeechRate.load(store))
        SpeechRate.save(store, SpeechRate.SLOW)
        assertEquals(SpeechRate.SLOW, SpeechRate.load(store))
    }
}
