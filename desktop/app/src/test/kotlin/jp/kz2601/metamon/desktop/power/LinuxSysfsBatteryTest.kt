package jp.kz2601.metamon.desktop.power

import jp.kz2601.metamon.desktop.power.DevicePowerState.Charge
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.nio.file.Path

/** Linux の電池: 一時フォルダに作った偽の /sys で、attr・longAttr と容量での重み付けの平均を試す。 */
class LinuxSysfsBatteryTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val sys: File get() = tmp.root

    private fun supply(name: String, vararg attrs: Pair<String, String>): File {
        val dir = File(sys, "class/power_supply/$name").apply { mkdirs() }
        attrs.forEach { (k, v) -> File(dir, k).writeText(v + "\n") }
        return dir
    }

    private fun read() = LinuxSysfsPowerReader(sys.toPath()).read()

    private fun assertLevel(expected: Double, state: DevicePowerState) = assertEquals(expected, state.batteryLevel!!, 1e-9)

    @Test
    fun twoBatteries_chargeOnly_weightedByChargeFull() {
        // 25%(4000) と 75%(1000) → (25×4000 + 75×1000) / 5000 = 35%
        supply("BAT0", "type" to "Battery", "status" to "Discharging", "charge_now" to "1000", "charge_full" to "4000")
        supply("BAT1", "type" to "Battery", "status" to "Discharging", "charge_now" to "750", "charge_full" to "1000")
        val s = read()
        assertLevel(0.35, s)
        assertEquals(Charge.UNPLUGGED, s.charge)
    }

    @Test
    fun twoBatteries_energyOnly_weightedByEnergyFull() {
        // 50%(20000) と 100%(60000) → (50×20000 + 100×60000) / 80000 = 87.5%
        supply("BAT0", "type" to "Battery", "status" to "Full", "energy_now" to "10000", "energy_full" to "20000")
        supply("BAT1", "type" to "Battery", "status" to "Full", "energy_now" to "60000", "energy_full" to "60000")
        assertLevel(0.875, read())
    }

    @Test
    fun zeroEnergyFull_fallsBackToChargeFullWeights() {
        // BAT0 の energy_full=0 は無い扱い → 全部にある charge_full で重みを付ける: (20×100 + 80×300) / 400 = 65%
        supply("BAT0", "type" to "Battery", "capacity" to "20", "energy_full" to "0", "charge_full" to "100")
        supply("BAT1", "type" to "Battery", "capacity" to "80", "energy_full" to "300", "charge_full" to "300")
        assertLevel(0.65, read())
    }

    @Test
    fun nonNumericCapacity_fallsBackToEnergyNow() {
        supply("BAT0", "type" to "Battery", "capacity" to "abc", "energy_now" to "30", "energy_full" to "60")
        assertLevel(0.5, read())
    }

    @Test
    fun zeroOrMissingFull_leavesLevelUnknown() {
        supply("BAT0", "type" to "Battery", "status" to "Discharging", "energy_now" to "10", "energy_full" to "0", "charge_now" to "5")
        val s = read()
        assertNull(s.batteryLevel)
        assertEquals(Charge.UNPLUGGED, s.charge)
    }

    @Test
    fun unreadableCapacity_fallsBackToChargeNow() {
        // capacity が通常のファイルでない(読めない)→ charge_now / charge_full
        val dir = supply("BAT0", "type" to "Battery", "charge_now" to "1", "charge_full" to "4")
        File(dir, "capacity").mkdir()
        assertLevel(0.25, read())
    }

    @Test
    fun batteryWithoutCapacityIsLeftOutOfAverage() {
        // 残量の読めない電池は平均に入れない
        supply("BAT0", "type" to "Battery", "capacity" to "40", "energy_full" to "100")
        supply("BAT1", "type" to "Battery", "capacity" to "x", "energy_full" to "900")
        assertLevel(0.4, read())
    }

    @Test
    fun absentBatteryIsIgnored() {
        supply("BAT0", "type" to "Battery", "capacity" to "40")
        supply("BAT1", "type" to "Battery", "present" to "0", "capacity" to "100")
        assertLevel(0.4, read())
    }

    @Test
    fun attr_trimsAndTreatsEmptyOrMissingAsNull() {
        val dir: Path = supply("BAT0", "a" to "  Full  ", "empty" to "   ").toPath()
        File(dir.toFile(), "sub").mkdir()
        assertEquals("Full", LinuxSysfsPowerReader.attr(dir, "a"))
        assertNull(LinuxSysfsPowerReader.attr(dir, "empty"))
        assertNull(LinuxSysfsPowerReader.attr(dir, "missing"))
        assertNull(LinuxSysfsPowerReader.attr(dir, "sub"))
        assertNull(LinuxSysfsPowerReader.attr(dir.resolve("nowhere"), "a"))
    }

    @Test
    fun longAttr_acceptsOnlyPositiveNumbers() {
        val dir = supply("BAT0", "n" to " 42 ", "zero" to "0", "neg" to "-5", "text" to "abc", "dec" to "1.5").toPath()
        assertEquals(42L, LinuxSysfsPowerReader.longAttr(dir, "n"))
        assertNull(LinuxSysfsPowerReader.longAttr(dir, "zero"))
        assertNull(LinuxSysfsPowerReader.longAttr(dir, "neg"))
        assertNull(LinuxSysfsPowerReader.longAttr(dir, "text"))
        assertNull(LinuxSysfsPowerReader.longAttr(dir, "dec"))
        assertNull(LinuxSysfsPowerReader.longAttr(dir, "missing"))
    }
}
