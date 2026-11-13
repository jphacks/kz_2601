package jp.kz2601.metamon.desktop.power

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Path
import java.nio.file.Paths

/** #2213: platform_profile の無い Linux で power-profiles-daemon(D-Bus)の ActiveProfile から省電力を読む。 */
class PowerProfiles2213Test {
    private fun fixture(name: String): Path = Paths.get(javaClass.getResource("/power/$name")!!.toURI())

    /** 作り物の D-Bus。[replies] は宛先ごとの Get の応答、[monitorLines] は monitor が流す行。 */
    private class FakeDbus(
        val replies: Map<String, String?>,
        val monitorLines: List<String> = emptyList(),
    ) : DbusClient {
        var gets = 0
        val monitored = mutableListOf<String>()
        var afterMonitorLine: ((Int) -> Unit)? = null

        override fun getProperty(dest: String, path: String, iface: String, name: String): String? {
            gets++
            assertEquals("ActiveProfile", name)
            return replies[dest]
        }

        override fun monitor(dest: String, path: String, onLine: (String) -> Unit): Boolean {
            monitored += dest
            monitorLines.forEachIndexed { i, l ->
                onLine(l)
                afterMonitorLine?.invoke(i)
            }
            return true
        }
    }

    private val newName = "org.freedesktop.UPower.PowerProfiles"
    private val oldName = "net.hadess.PowerProfiles"

    /** monitor を走らせず(スレッドを起こさない)に読む。 */
    private fun source(dbus: DbusClient) = DbusPowerProfileSource(dbus, startThread = {})

    @Test
    fun parsesGdbusGetReply() {
        assertEquals("power-saver", DbusPowerProfileSource.parseGetReply("(<'power-saver'>,)\n"))
        assertEquals("balanced", DbusPowerProfileSource.parseGetReply("(<'balanced'>,)"))
        assertNull(DbusPowerProfileSource.parseGetReply("Error: GDBus.Error:org.freedesktop.DBus.Error.ServiceUnknown"))
        assertNull(DbusPowerProfileSource.parseGetReply(""))
    }

    @Test
    fun parsesMonitorPropertiesChanged() {
        val line = "/org/freedesktop/UPower/PowerProfiles: org.freedesktop.DBus.Properties.PropertiesChanged " +
            "('org.freedesktop.UPower.PowerProfiles', {'ActiveProfile': <'power-saver'>, 'PerformanceDegraded': <''>}, @as [])"
        assertEquals("power-saver", DbusPowerProfileSource.parseMonitorLine(line))
        val other = "/org/freedesktop/UPower/PowerProfiles: org.freedesktop.DBus.Properties.PropertiesChanged " +
            "('org.freedesktop.UPower.PowerProfiles', {'Profiles': <[{'Profile': <'balanced'>}]>}, @as [])"
        assertNull(DbusPowerProfileSource.parseMonitorLine(other))
        assertNull(DbusPowerProfileSource.parseMonitorLine("The name org.freedesktop.UPower.PowerProfiles is owned by :1.5"))
    }

    @Test
    fun noPlatformProfile_daemonPowerSaver_isPowerSaver() {
        val dbus = FakeDbus(mapOf(newName to "(<'power-saver'>,)"))
        val s = LinuxSysfsPowerReader(fixture("desktop-no-battery"), source(dbus)).read()
        assertTrue(s.isPowerSaver)
    }

    @Test
    fun noPlatformProfile_daemonBalanced_isNotPowerSaver() {
        val dbus = FakeDbus(mapOf(newName to "(<'balanced'>,)"))
        assertFalse(LinuxSysfsPowerReader(fixture("desktop-no-battery"), source(dbus)).read().isPowerSaver)
    }

    @Test
    fun oldDaemonName_isUsedWhenNewNameMissing() {
        val dbus = FakeDbus(mapOf(newName to null, oldName to "(<'power-saver'>,)"))
        assertEquals("power-saver", source(dbus).activeProfile())
    }

    @Test
    fun noDbusOrDaemon_isUnknownAndNotPowerSaver() {
        val dbus = FakeDbus(emptyMap())
        val src = source(dbus)
        assertFalse(LinuxSysfsPowerReader(fixture("desktop-no-battery"), src).read().isPowerSaver)
        assertNull(src.activeProfile())
    }

    @Test
    fun platformProfilePresent_daemonIsNotAsked() {
        val dbus = FakeDbus(mapOf(newName to "(<'power-saver'>,)"))
        // two-batteries は platform_profile=balanced。D-Bus が省電力でも platform_profile を優先する
        assertFalse(LinuxSysfsPowerReader(fixture("two-batteries"), source(dbus)).read().isPowerSaver)
        assertEquals(0, dbus.gets)
    }

    @Test
    fun repeatedReads_doNotQueryAgain() {
        val dbus = FakeDbus(mapOf(newName to "(<'balanced'>,)"))
        val reader = LinuxSysfsPowerReader(fixture("desktop-no-battery"), source(dbus))
        repeat(5) { reader.read() }
        assertEquals(1, dbus.gets)
    }

    @Test
    fun monitor_followsChangesAndDaemonGoingAway() {
        val dbus = FakeDbus(
            mapOf(newName to "(<'balanced'>,)"),
            monitorLines = listOf(
                "/org/freedesktop/UPower/PowerProfiles: org.freedesktop.DBus.Properties.PropertiesChanged " +
                    "('org.freedesktop.UPower.PowerProfiles', {'ActiveProfile': <'power-saver'>}, @as [])",
                "The name org.freedesktop.UPower.PowerProfiles does not have an owner",
                "The name org.freedesktop.UPower.PowerProfiles is owned by :1.9",
            ),
        )
        var thread: Runnable? = null
        val src = DbusPowerProfileSource(dbus, startThread = { thread = it })
        assertEquals("balanced", src.activeProfile())
        val seen = mutableListOf<String?>()
        dbus.afterMonitorLine = { seen += src.activeProfile() }
        thread!!.run()
        // 省電力に変わる → デーモンが居なくなる → 戻ってきて読み直す(balanced)
        assertEquals(listOf("power-saver", null, "balanced"), seen)
        assertEquals(listOf(newName), dbus.monitored)
        // monitor が終わったら分からない扱い
        assertNull(src.activeProfile())
        assertEquals(2, dbus.gets)
    }
}
