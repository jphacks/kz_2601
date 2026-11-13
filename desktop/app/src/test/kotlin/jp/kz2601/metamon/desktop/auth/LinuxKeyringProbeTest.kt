package jp.kz2601.metamon.desktop.auth

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** Secret Service があるかの判定(作り物のバス): ある / 無い / 調べるのに失敗、とバスが無い時。 */
class LinuxKeyringProbeTest {
    private class FakeBus(override val present: Boolean = true, val names: () -> Set<String>) : SessionBus {
        var calls = 0
        override fun names(): Set<String> {
            calls++
            return names.invoke()
        }
    }

    private val real = object : TokenVault {
        override fun save(token: String) = Unit
        override fun read(): String? = "t"
        override fun delete() = Unit
    }

    @Test
    fun availableWhenTheSecretServiceIsOnTheBus() {
        val probe = LinuxSecretProbe(FakeBus { setOf("org.freedesktop.DBus", LinuxSecretProbe.SECRET_SERVICE) })
        assertEquals(LinuxSecretProbe.Result.Available, probe.probe())
        assertSame(real, probe.vault { real })
    }

    @Test
    fun missingSecretServiceFallsBackToNotSaving() = Strings.withLang(Lang.JA) {
        val probe = LinuxSecretProbe(FakeBus { setOf("org.freedesktop.DBus", "org.freedesktop.Notifications") })
        val r = probe.probe() as LinuxSecretProbe.Result.Unavailable
        assertEquals(LinuxSecretProbe.Reason.NO_SECRET_SERVICE, r.reason)
        assertTrue(r.message, r.message.contains("サインインを保存しません"))
        val v = probe.vault { error("使えない時は作らない") }
        assertTrue(v is NoTokenVault)
        assertEquals("desktop_keyring_no_service", (v as NoTokenVault).reasonKey)
        v.save("t")
        assertNull("保存しない", v.read())
    }

    @Test
    fun probeFailureFallsBackWithItsOwnReason() = Strings.withLang(Lang.EN) {
        val probe = LinuxSecretProbe(FakeBus { throw IOException("dbus-send: not found") })
        val r = probe.probe() as LinuxSecretProbe.Result.Unavailable
        assertEquals(LinuxSecretProbe.Reason.PROBE_FAILED, r.reason)
        assertTrue(r.message, r.message.startsWith("Couldn't check the keyring"))
        assertTrue(probe.vault { real } is NoTokenVault)
    }

    @Test
    fun noSessionBusDoesNotAskTheBus() {
        val bus = FakeBus(present = false) { error("聞かない") }
        val r = LinuxSecretProbe(bus).probe() as LinuxSecretProbe.Result.Unavailable
        assertEquals(LinuxSecretProbe.Reason.NO_SESSION_BUS, r.reason)
        assertEquals(0, bus.calls)
    }

    @Test
    fun everyReasonHasTextInBothLanguages() {
        LinuxSecretProbe.Reason.entries.forEach { reason ->
            Lang.entries.forEach { l -> assertTrue("${reason.messageKey} ($l)", Strings.table.hasString(l, reason.messageKey)) }
        }
    }

    @Test
    fun dbusSendReplyIsParsedAndPresenceFollowsTheEnv() {
        val reply = """
            method return time=1 sender=org.freedesktop.DBus -> destination=:1.9 serial=3 reply_serial=2
               array [
                  string "org.freedesktop.DBus"
                  string ":1.9"
                  string "org.freedesktop.secrets"
               ]
        """.trimIndent()
        assertEquals(listOf("org.freedesktop.DBus", ":1.9", "org.freedesktop.secrets"), DbusSendSessionBus.parseNames(reply))
        assertTrue(DbusSendSessionBus(env = { "unix:path=/run/user/1000/bus" }).present)
        assertFalse(DbusSendSessionBus(env = { null }).present)
        assertFalse(DbusSendSessionBus(env = { " " }).present)
    }
}
