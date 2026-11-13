package jp.kz2601.metamon.desktop.hotkey

import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.ByteOrder

/** #2203: Wayland の GlobalShortcuts のポータル(D-Bus は作り物)・有無の判定・理由の文言の選び方・D-Bus の読み書き。 */
class PortalGlobalHotkey2203Test {
    private fun chord(raw: String) = (HotkeySpec.parse(raw) as HotkeyParse.Ok).chord

    private class FakePortal(
        val version: Int? = 1,
        val created: PortalResponse = PortalResponse(0, mapOf("session_handle" to "/org/freedesktop/portal/desktop/session/1_5/s")),
        val bound: PortalResponse = PortalResponse(0),
    ) : ShortcutsPortal {
        val binds = mutableListOf<List<String?>>()
        val closedSessions = mutableListOf<String>()
        var closed = false
        var listener: ((String, String) -> Unit)? = null

        override fun version() = version
        override fun createSession() = created
        override fun bind(session: String, id: String, description: String, trigger: String?): PortalResponse {
            binds += listOf(session, id, description, trigger)
            return bound
        }

        override fun onActivated(listener: (session: String, id: String) -> Unit) {
            this.listener = listener
        }

        override fun closeSession(session: String) {
            closedSessions += session
        }

        override fun close() {
            closed = true
        }
    }

    // MARK: - 登録の流れ

    @Test
    fun bindsAndFiresOnlyForOwnSessionAndId() {
        val p = FakePortal()
        var pressed = 0
        val h = PortalGlobalHotkey({ p }, { "desc" })
        assertEquals(HotkeyRegistration.Registered(chord("ctrl+shift+space")), h.register(chord("ctrl+shift+space")) { pressed++ })
        val session = "/org/freedesktop/portal/desktop/session/1_5/s"
        assertEquals(listOf(session, PortalGlobalHotkey.SHORTCUT_ID, "desc", "CTRL+SHIFT+space"), p.binds.single())
        p.listener!!(session, PortalGlobalHotkey.SHORTCUT_ID)
        p.listener!!(session, "other")
        p.listener!!("/other/session", PortalGlobalHotkey.SHORTCUT_ID)
        assertEquals(1, pressed)
        h.unregister()
        assertEquals(listOf(session), p.closedSessions)
        assertTrue(p.closed)
    }

    @Test
    fun missingPortalOrInterfaceIsUnsupported() {
        assertEquals(
            HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED, PortalGlobalHotkey.NO_PORTAL),
            PortalGlobalHotkey({ null }).register(chord("ctrl+k")) {},
        )
        val old = FakePortal(version = null)
        assertEquals(
            HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED, PortalGlobalHotkey.NO_GLOBAL_SHORTCUTS),
            PortalGlobalHotkey({ old }).register(chord("ctrl+k")) {},
        )
        assertTrue("使わない接続は閉じる", old.closed)
        assertTrue(old.binds.isEmpty())
        assertEquals(
            "つなぐ時の例外も UNSUPPORTED",
            HotkeyFailure.UNSUPPORTED,
            (PortalGlobalHotkey({ error("boom") }).register(chord("ctrl+k")) {} as HotkeyRegistration.Failed).reason,
        )
    }

    @Test
    fun cancelledAndFailedResponses() {
        val cancelled = FakePortal(bound = PortalResponse(PortalResponse.CANCELLED))
        assertEquals(
            HotkeyRegistration.Failed(HotkeyFailure.ERROR, PortalGlobalHotkey.CANCELLED),
            PortalGlobalHotkey({ cancelled }).register(chord("ctrl+k")) {},
        )
        assertEquals("作ったセッションは閉じる", 1, cancelled.closedSessions.size)
        assertTrue(cancelled.closed)
        val noSession = FakePortal(created = PortalResponse(PortalResponse.OTHER))
        val r = PortalGlobalHotkey({ noSession }).register(chord("ctrl+k")) {} as HotkeyRegistration.Failed
        assertEquals(HotkeyFailure.ERROR, r.reason)
        assertTrue(noSession.closedSessions.isEmpty())
        assertTrue(noSession.closed)
        assertEquals("o で来ても読む", "/s", PortalGlobalHotkey.sessionHandle(PortalResponse(0, mapOf("session_handle" to DBusObjectPath("/s")))))
    }

    @Test
    fun triggers() {
        assertEquals("CTRL+SHIFT+space", PortalTrigger.of(chord("ctrl+shift+space")))
        assertEquals("ALT+LOGO+k", PortalTrigger.of(chord("alt+super+K")))
        assertEquals("F9", PortalTrigger.of(chord("f9")))
        assertEquals("CTRL+Return", PortalTrigger.of(chord("ctrl+enter")))
        assertEquals("CTRL+7", PortalTrigger.of(chord("ctrl+7")))
        assertNull(PortalTrigger.of(HotkeyChord(ctrl = true, vk = 0xFFFF, keyName = "?")))
    }

    // MARK: - 理由の文言

    @Test
    fun reasonSelection() {
        val unsupported = HotkeyRegistration.Failed(HotkeyFailure.UNSUPPORTED)
        val wayland = mapOf("WAYLAND_DISPLAY" to "wayland-0")::get
        assertEquals(HotkeyUnavailable.WAYLAND, HotkeyAvailability.reason(unsupported, "Linux", wayland))
        assertEquals(HotkeyUnavailable.NO_DISPLAY, HotkeyAvailability.reason(unsupported, "Linux", mapOf<String, String>()::get))
        assertEquals("libX11 が無い X11", HotkeyUnavailable.OS, HotkeyAvailability.reason(unsupported, "Linux", mapOf("DISPLAY" to ":0")::get))
        assertEquals(HotkeyUnavailable.OS, HotkeyAvailability.reason(unsupported, "Mac OS X", wayland))
        assertEquals(
            HotkeyUnavailable.CANCELLED,
            HotkeyAvailability.reason(HotkeyRegistration.Failed(HotkeyFailure.ERROR, PortalGlobalHotkey.CANCELLED), "Linux", wayland),
        )
        assertNull("重なるのは知らせの方", HotkeyAvailability.reason(HotkeyRegistration.Failed(HotkeyFailure.IN_USE), "Linux", wayland))
        assertNull(HotkeyAvailability.reason(HotkeyRegistration.Failed(HotkeyFailure.ERROR, "x"), "Linux", wayland))
        assertNull(HotkeyAvailability.reason(HotkeyRegistration.Registered(chord("ctrl+k")), "Linux", wayland))
        assertNull(HotkeyAvailability.reason(null, "Linux", wayland))
        assertEquals(Strings["desktop_hotkey_unavailable_wayland"], HotkeyUnavailable.WAYLAND.text)
        assertTrue("Wayland の制限と書く", HotkeyUnavailable.WAYLAND.text.contains("Wayland"))
    }

    @Test
    fun startFromSettingsRecordsStatus() {
        val status = HotkeyStatus()
        GlobalHotkeys.startFromSettings(
            raw = "ctrl+k", hotkey = { PortalGlobalHotkey({ null }) }, voiceEnabled = { true }, bringToFront = {}, notify = { _, _ -> },
            post = { it.run() }, log = {}, status = status,
        )
        assertEquals(HotkeyFailure.UNSUPPORTED, (status.last as HotkeyRegistration.Failed).reason)
    }

    // MARK: - D-Bus

    @Test
    fun signatureSplit() {
        assertEquals(listOf("o", "a(sa{sv})", "s", "a{sv}"), DBusSignature.split("oa(sa{sv})sa{sv}"))
        assertEquals(listOf("u", "a{sv}"), DBusSignature.split("ua{sv}"))
    }

    @Test
    fun messageRoundTrip() {
        val body = listOf(
            DBusObjectPath("/org/freedesktop/portal/desktop/session/1_42/t"),
            listOf(listOf("open-voice", mapOf("description" to DBusVariant("s", "d"), "preferred_trigger" to DBusVariant("s", "CTRL+space")))),
            "",
            mapOf("handle_token" to DBusVariant("s", "tailorit_1")),
        )
        val m = DBusMessage(
            DBusMessage.METHOD_CALL, 7, "/org/freedesktop/portal/desktop", "org.freedesktop.portal.GlobalShortcuts", "BindShortcuts",
            destination = "org.freedesktop.portal.Desktop", signature = "oa(sa{sv})sa{sv}", body = body,
        )
        val bytes = m.encode()
        assertEquals(bytes.size, DBusMessage.totalLength(bytes.copyOf(16)))
        assertEquals(m, DBusMessage.decode(bytes))
    }

    @Test
    fun readsResponseSignalAndVersion() {
        val signal = DBusMessage(
            DBusMessage.SIGNAL, 3, "/org/freedesktop/portal/desktop/request/1_42/tailorit_1", "org.freedesktop.portal.Request", "Response",
            replySerial = null, sender = ":1.9", signature = "ua{sv}",
            body = listOf(0L, mapOf("session_handle" to DBusVariant("s", "/s/1"))),
        )
        val back = DBusMessage.decode(signal.encode())
        assertEquals(PortalResponse(0, mapOf("session_handle" to "/s/1")), DBusShortcutsPortal.response(back.body))
        val reply = DBusMessage.decode(DBusMessage(DBusMessage.METHOD_RETURN, 4, replySerial = 2, signature = "v", body = listOf(DBusVariant("u", 1L))).encode())
        assertEquals(2L, reply.replySerial)
        assertEquals(DBusVariant("u", 1L), reply.body.single())
    }

    @Test
    fun readsBigEndian() {
        // 'B' の u32 = 1 と 文字列 "ab"
        val data = byteArrayOf(0, 0, 0, 1, 0, 0, 0, 2, 'a'.code.toByte(), 'b'.code.toByte(), 0)
        val r = DBusReader(data, ByteOrder.BIG_ENDIAN)
        assertEquals(1L, r.read("u"))
        assertEquals("ab", r.read("s"))
    }

    @Test
    fun emptyArrayStillPadsToElementAlignment() {
        val w = DBusWriter()
        w.write("a{sv}", emptyMap<String, Any?>())
        assertEquals("長さ 4 + 8 への揃え", 8, w.size)
        val r = DBusReader(w.bytes(), ByteOrder.LITTLE_ENDIAN)
        assertEquals(emptyMap<Any?, Any?>(), r.read("a{sv}"))
    }

    @Test
    fun addressAndAuth() {
        assertEquals("/run/user/1000/bus", DBusAddress.unixPath("unix:path=/run/user/1000/bus,guid=abc"))
        assertEquals("/tmp/a b", DBusAddress.unixPath("unix:abstract=/tmp/x;unix:path=/tmp/a%20b"))
        assertNull("abstract だけは開けない", DBusAddress.unixPath("unix:abstract=/tmp/dbus-x,guid=1"))
        assertNull(DBusAddress.unixPath("tcp:host=localhost,port=1"))
        assertNull(DBusAddress.unixPath(null))
        assertEquals("31303030", DBusAddress.externalAuth(1000))
        assertEquals("/org/freedesktop/portal/desktop/request/1_42/t", DBusShortcutsPortal.requestPath(":1.42", "t"))
        assertNull("Linux 以外はつながない", DBusShortcutsPortal.connect("Windows 11") { null })
        assertNull("バスの場所が無ければ null", DBusShortcutsPortal.connect("Linux") { null })
    }
}
