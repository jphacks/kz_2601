package jp.kz2601.metamon.desktop.os

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Windows で通知がオフの時の帯: 通知を使うアプリか・レジストリの読み方・閉じた後。本物のレジストリには触らない。 */
class NotificationsOffTest {
    @After
    fun reset() = NotificationsOff.resetDismissed()

    private fun app(screenJson: String, notifications: String = "[]"): AppDSL = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t","notifications":$notifications},"entities":[],"initialScreen":"home","screens":[{"id":"home","title":"h","components":[$screenJson]}]}""",
    )

    private val plain = app("""{"type":"text","id":"t1","text":"hi"}""")
    private val scheduling = app(
        """{"type":"button","id":"b1","label":"知らせる","action":{"type":"scheduleNotification","notification":{"id":"n1","title":"水","body":"飲む"}}}""",
    )

    @Test
    fun notShownForAppsWithoutNotifications() {
        assertFalse(NotificationsOff.usesNotifications(plain))
        assertFalse(NotificationsOff.isVisible(toastEnabled = false, usesNotifications = NotificationsOff.usesNotifications(plain), dismissed = false))
    }

    @Test
    fun appsThatUseNotifications() {
        assertTrue(NotificationsOff.usesNotifications(scheduling))
        assertTrue(NotificationsOff.usesNotifications(app("""{"type":"timerDisplay","id":"tm","mode":"countdown","seconds":60}""")))
        assertFalse(NotificationsOff.usesNotifications(app("""{"type":"timerDisplay","id":"tm","mode":"stopwatch"}""")))
        assertTrue(NotificationsOff.usesNotifications(app("""{"type":"text","id":"t1","text":"hi"}""", """[{"id":"d","title":"毎日","body":"記録","repeat":"daily","time":"08:00"}]""")))
    }

    @Test
    fun shownOnlyWhenToastsAreOff() {
        assertTrue(NotificationsOff.isVisible(toastEnabled = false, usesNotifications = true, dismissed = false))
        assertFalse(NotificationsOff.isVisible(toastEnabled = true, usesNotifications = true, dismissed = false))
        // 読めない・Windows 以外
        assertFalse(NotificationsOff.isVisible(toastEnabled = null, usesNotifications = true, dismissed = false))
    }

    @Test
    fun notShownAfterDismissForThatApp() {
        assertFalse(NotificationsOff.isDismissed("a"))
        NotificationsOff.dismiss("a")
        assertTrue(NotificationsOff.isDismissed("a"))
        assertFalse(NotificationsOff.isVisible(toastEnabled = false, usesNotifications = true, dismissed = NotificationsOff.isDismissed("a")))
        // ほかのアプリでは出す
        assertTrue(NotificationsOff.isVisible(toastEnabled = false, usesNotifications = true, dismissed = NotificationsOff.isDismissed("b")))
    }

    @Test
    fun parsesRegistry() {
        val off = "\r\nHKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\PushNotifications\r\n    ToastEnabled    REG_DWORD    0x0\r\n"
        assertEquals(false, NotificationsOff.parseToastEnabled(off))
        assertEquals(true, NotificationsOff.parseToastEnabled(off.replace("0x0", "0x1")))
        assertNull(NotificationsOff.parseToastEnabled("ERROR: The system was unable to find the specified registry key or value."))
    }

    @Test
    fun readsNothingOutsideWindows() {
        assertNull(NotificationsOff.read("Linux"))
        assertNull(NotificationsOff.read("Mac OS X"))
    }
}
