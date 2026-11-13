package jp.kz2601.metamon.desktop.debug

import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.desktop.power.DevPowerOverride
import jp.kz2601.metamon.desktop.power.DevPowerPreset
import jp.kz2601.metamon.desktop.power.DevicePowerSources
import jp.kz2601.metamon.desktop.power.DevicePowerState
import jp.kz2601.metamon.desktop.power.DevicePowerState.Charge
import jp.kz2601.metamon.desktop.power.FakeDevicePowerSource
import jp.kz2601.metamon.desktop.power.SwitchableDevicePowerSource
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #3212: 開発用メニューで電池の状態を作り物にする。 */
class DevPower3212Test {
    private val real = DevicePowerState(0.8, Charge.UNPLUGGED, false)

    @After
    fun reset() = DevPowerOverride.select(DevPowerPreset.OFF)

    @Test
    fun switchingChangesTheFlowingValueAndOffRestoresTheRealOne() = runTest {
        val override = MutableStateFlow<DevicePowerState?>(null)
        val source = SwitchableDevicePowerSource(FakeDevicePowerSource(real), override)
        val got = mutableListOf<DevicePowerState>()
        val job = launch { source.changes.take(5).toList(got) }
        runCurrent()
        override.value = DevPowerPreset.LOW_POWER_SAVER.state
        runCurrent()
        override.value = DevPowerPreset.CHARGING.state
        runCurrent()
        override.value = DevPowerPreset.UNKNOWN.state
        runCurrent()
        override.value = DevPowerPreset.OFF.state
        job.join()
        assertEquals(
            listOf(real, DevicePowerState(0.15, Charge.UNPLUGGED, true), DevicePowerState(0.5, Charge.CHARGING, false), DevicePowerState.Unknown, real),
            got,
        )
        assertEquals(real, source.current)
    }

    @Test
    fun paletteItemsSelectThePreset() {
        val host = object : DevMenuHost {
            override val config = DesktopApiConfig.fromEnvironment(mapOf("TAILORIT_API" to "stg"))
            override val pendingCount: Int? = null
            override val canWipe = false
            val messages = mutableListOf<String>()
            override fun showInfo(title: String, lines: List<String>) = Unit
            override fun askWipe() = Unit
            override fun message(text: String) { messages += text }
        }
        val source = SwitchableDevicePowerSource(FakeDevicePowerSource(real))
        val commands = DevMenu.commands(host, enabled = true)
        assertEquals(
            listOf("dev:power:low_power_saver", "dev:power:charging", "dev:power:unknown", "dev:power:off"),
            commands.map { it.id }.filter { it.startsWith("dev:power:") },
        )
        commands.single { it.id == "dev:power:low_power_saver" }.run()
        assertEquals(DevPowerPreset.LOW_POWER_SAVER, DevPowerOverride.selected)
        assertTrue(source.current.isPowerSaver)
        assertEquals(1, host.messages.size)
        commands.single { it.id == "dev:power:off" }.run()
        assertEquals(real, source.current)
        assertTrue(DevMenu.commands(host, enabled = false).isEmpty())
    }

    @Test
    fun makeWrapsOnlyWhenTheDevMenuIsShown() {
        assertTrue(DevicePowerSources.make(mapOf("METAMON_MOCK" to "1"), "Linux", devMenu = true) is SwitchableDevicePowerSource)
        assertTrue(DevicePowerSources.make(mapOf("METAMON_MOCK" to "1"), "Linux", devMenu = false) is FakeDevicePowerSource)
        assertFalse(DevicePowerSources.make(emptyMap(), "Linux") is SwitchableDevicePowerSource)
    }
}
