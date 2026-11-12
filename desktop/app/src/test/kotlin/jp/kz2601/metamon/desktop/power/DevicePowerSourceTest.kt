package jp.kz2601.metamon.desktop.power

import jp.kz2601.metamon.desktop.power.DevicePowerState.Charge
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Path
import java.nio.file.Paths
import kotlin.time.Duration.Companion.seconds

/** 電源の読み取り: sysfs のフィクスチャ(src/test/resources/power/<場面>/ = 偽の /sys)・間隔で読む実体・偽物。 */
@OptIn(ExperimentalCoroutinesApi::class)
class DevicePowerSourceTest {
    private fun fixture(name: String): Path = Paths.get(javaClass.getResource("/power/$name")!!.toURI())

    private fun readFixture(name: String) = LinuxSysfsPowerReader(fixture(name)).read()

    private fun assertLevel(expected: Double, state: DevicePowerState) = assertEquals(expected, state.batteryLevel!!, 1e-9)

    @Test
    fun laptopOnBattery_ignoresPeripheralBatteryAndReadsPlatformProfile() {
        val s = readFixture("laptop-discharging")
        assertLevel(0.57, s)
        // マウスの電池(scope=Device・充電中)には引っ張られない
        assertEquals(Charge.UNPLUGGED, s.charge)
        assertTrue(s.isPowerSaver)
    }

    @Test
    fun twoBatteries_weightedByEnergyFull_chargingWins() {
        val s = readFixture("two-batteries")
        // (100×20 + 40×60) / 80 = 55%
        assertLevel(0.55, s)
        assertEquals(Charge.CHARGING, s.charge)
        assertFalse(s.isPowerSaver)
    }

    @Test
    fun notCharging_isTreatedAsPluggedFull() {
        val s = readFixture("not-charging")
        assertLevel(0.80, s)
        assertEquals(Charge.FULL, s.charge)
        assertTrue(s.isCharging)
    }

    @Test
    fun unknownStatus_fallsBackToMainsOnline() {
        val s = readFixture("status-unknown")
        assertLevel(0.73, s)
        assertEquals(Charge.CHARGING, s.charge)
    }

    @Test
    fun noBattery_absentBattery_missingSysfs_areUnknown() {
        assertEquals(DevicePowerState.Unknown, readFixture("desktop-no-battery"))
        assertEquals(DevicePowerState.Unknown, readFixture("battery-absent"))
        assertEquals(DevicePowerState.Unknown, LinuxSysfsPowerReader(fixture("desktop-no-battery").resolve("nowhere")).read())
    }

    @Test
    fun combine_unweightedAverageWhenEnergyMissing() {
        val s = LinuxSysfsPowerReader.combine(
            listOf(LinuxSysfsPowerReader.Battery(30, "Discharging", null), LinuxSysfsPowerReader.Battery(70, "Full", 1)),
            mainsOnline = emptyList(),
            isPowerSaver = false,
        )
        assertLevel(0.50, s)
        assertEquals(Charge.UNPLUGGED, s.charge)
    }

    @Test
    fun levelIsClamped() {
        assertEquals(1.0, DevicePowerState.of(1.3, Charge.FULL, false).batteryLevel!!, 0.0)
        assertNull(DevicePowerState.of(Double.NaN, Charge.UNKNOWN, false).batteryLevel)
    }

    @Test
    fun polling_emitsFirstValueThenOnlyChanges() = runTest {
        var reads = 0
        val values = ArrayDeque(listOf(0.5, 0.5, 0.5, 0.4, 0.4))
        val source = PollingDevicePowerSource(
            reader = {
                reads++
                DevicePowerState.of(values.removeFirstOrNull() ?: 0.4, Charge.UNPLUGGED, false)
            },
            interval = 60.seconds,
            dispatcher = StandardTestDispatcher(testScheduler),
        )
        val got = mutableListOf<DevicePowerState>()
        val job = launch { source.changes.take(2).toList(got) }
        runCurrent()
        assertEquals(1, got.size)
        advanceTimeBy(121.seconds)
        runCurrent()
        // 3 回読んで 0.5 のまま → 1 つだけ
        assertEquals(1, got.size)
        advanceTimeBy(60.seconds)
        runCurrent()
        job.join()
        assertEquals(listOf(0.5, 0.4), got.map { it.batteryLevel })
        assertEquals(4, reads)
    }

    @Test
    fun polling_readFailureBecomesUnknown() {
        val source = PollingDevicePowerSource({ error("boom") })
        assertEquals(DevicePowerState.Unknown, source.current)
    }

    @Test
    fun fake_fromEnvironmentAndUpdates() = runTest {
        val fake = FakeDevicePowerSource.fromEnvironment(
            mapOf("METAMON_BATTERY_LEVEL" to "0.2", "METAMON_BATTERY_STATE" to "charging", "METAMON_LOW_POWER" to "1"),
        )
        assertEquals(DevicePowerState(0.2, Charge.CHARGING, true), fake.current)
        assertEquals(DevicePowerState.Unknown, FakeDevicePowerSource.fromEnvironment(emptyMap()).current)

        val got = mutableListOf<DevicePowerState>()
        val job = launch { fake.changes.take(2).toList(got) }
        runCurrent()
        fake.state = fake.state.copy(charge = Charge.FULL)
        job.join()
        assertEquals(listOf(Charge.CHARGING, Charge.FULL), got.map { it.charge })
    }

    @Test
    fun make_picksImplementationByOs() {
        assertTrue(DevicePowerSources.make(mapOf("METAMON_MOCK" to "1"), "Linux") is FakeDevicePowerSource)
        assertTrue(DevicePowerSources.make(emptyMap(), "Linux") is PollingDevicePowerSource)
        // Windows は JNA が無いので今は不明(TODO)
        assertEquals(DevicePowerState.Unknown, DevicePowerSources.make(emptyMap(), "Windows 11").current)
        assertEquals(DevicePowerState.Unknown, DevicePowerSources.make(emptyMap(), "Mac OS X").current)
    }
}
