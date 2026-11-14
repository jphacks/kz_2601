package jp.kz2601.metamon.desktop.power

import jp.kz2601.metamon.desktop.power.DevicePowerState.Charge
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

// GetSystemPowerStatus の値 → 状態(Windows でない機械でも動く純粋な部分だけ)
class WindowsPowerReaderTest {
    private fun map(ac: Int, flag: Int, percent: Int, saver: Int = 0) = WindowsPowerReader.map(ac, flag, percent, saver)

    @Test
    fun onBattery() {
        val s = map(ac = 0, flag = 1, percent = 73)
        assertEquals(0.73, s.batteryLevel!!, 1e-9)
        assertEquals(Charge.UNPLUGGED, s.charge)
        assertFalse(s.isPowerSaver)
    }

    @Test
    fun pluggedBelowFull_isCharging_atOrAbove99_isFull() {
        assertEquals(Charge.CHARGING, map(ac = 1, flag = 8, percent = 50).charge)
        assertEquals(Charge.CHARGING, map(ac = 1, flag = 1, percent = 98).charge)
        assertEquals(Charge.FULL, map(ac = 1, flag = 1, percent = 99).charge)
        assertEquals(Charge.FULL, map(ac = 1, flag = 1, percent = 100).charge)
    }

    @Test
    fun unknownPercent_levelUnknown_pluggedStillCharging() {
        val s = map(ac = 1, flag = 8, percent = 255)
        assertNull(s.batteryLevel)
        assertEquals(Charge.CHARGING, s.charge)
    }

    @Test
    fun noBattery_isUnknown_butKeepsPowerSaver() {
        assertEquals(DevicePowerState.Unknown, map(ac = 1, flag = 128, percent = 255))
        assertEquals(DevicePowerState.Unknown.copy(isPowerSaver = true), map(ac = 1, flag = 128, percent = 255, saver = 1))
    }

    @Test
    fun flag255_isNotNoBattery() {
        val s = map(ac = 0, flag = 255, percent = 40)
        assertEquals(0.4, s.batteryLevel!!, 1e-9)
        assertEquals(Charge.UNPLUGGED, s.charge)
    }

    @Test
    fun acUnknown_usesChargingFlagOnly() {
        assertEquals(Charge.CHARGING, map(ac = 255, flag = 8, percent = 60).charge)
        assertEquals(Charge.UNKNOWN, map(ac = 255, flag = 1, percent = 60).charge)
        assertEquals(Charge.UNKNOWN, map(ac = 255, flag = 255, percent = 255).charge)
    }

    @Test
    fun batterySaver() {
        assertTrue(map(ac = 0, flag = 2, percent = 15, saver = 1).isPowerSaver)
        assertFalse(map(ac = 0, flag = 2, percent = 15, saver = 0).isPowerSaver)
    }

    @Test
    fun levelIsClamped() {
        assertEquals(1.0, map(ac = 1, flag = 1, percent = 120).batteryLevel!!, 1e-9)
    }

    @Test
    fun notWindows_readsUnknownWithoutLoadingJna() {
        if (System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)) return
        assertEquals(DevicePowerState.Unknown, WindowsPowerReader.read())
    }
}
