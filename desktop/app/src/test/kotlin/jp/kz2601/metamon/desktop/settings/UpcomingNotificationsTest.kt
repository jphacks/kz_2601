package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.desktop.Notifier
import jp.kz2601.metamon.desktop.TrayItem
import jp.kz2601.metamon.desktop.TrayMenu
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

// 予約した通知の並べ方・ラベル・取り消し(Android の UpcomingNotificationList と同じ並び)
class UpcomingNotificationsTest {
    private val utc = ZoneId.of("UTC")
    private fun n(id: String, at: String, title: String = id, app: String? = null) = UpcomingNotification(id, title, "", app, Instant.parse(at))

    @After
    fun tearDown() {
        Notifier.Tray.cancelAll()
    }

    @Test
    fun sortsNearestFirstAndTiesById() {
        val l = UpcomingNotifications.of(listOf(n("c", "2026-11-14T09:00:00Z"), n("b", "2026-11-13T09:00:00Z"), n("a", "2026-11-13T09:00:00Z")))
        assertEquals(listOf("a", "b", "c"), l.rows.map { it.id })
        assertEquals(0, l.hidden)
        assertFalse(l.isEmpty)
    }

    @Test
    fun limitsToFiveAndCountsTheRest() {
        val l = UpcomingNotifications.of((1..8).map { n("n$it", "2026-11-0${it}T09:00:00Z") })
        assertEquals(UpcomingNotifications.LIMIT, l.rows.size)
        assertEquals(listOf("n1", "n2", "n3", "n4", "n5"), l.rows.map { it.id })
        assertEquals(3, l.hidden)
    }

    @Test
    fun emptyWhenNothingScheduled() {
        assertTrue(UpcomingNotifications.EMPTY.isEmpty)
        assertTrue(UpcomingNotifications.of(emptyList()).rows.isEmpty())
    }

    @Test
    fun labelHasTimeTitleAndApp() {
        assertEquals("11/13 14:05 薬 (家計簿)", UpcomingNotifications.label(n("x", "2026-11-13T14:05:00Z", "薬", "家計簿"), utc))
        assertEquals("11/13 14:05 薬", UpcomingNotifications.label(n("x", "2026-11-13T14:05:00Z", "薬", " "), utc))
    }

    @Test
    fun armedNotificationShowsUpAndCancelRemovesIt() {
        val info = n("t1", "2099-01-01T00:00:00Z", "あと", "app")
        Notifier.Tray.arm("t1", 60_000, info) {}
        assertEquals(listOf(info), Notifier.Tray.upcoming())
        Notifier.Tray.cancel("t1")
        assertTrue(Notifier.Tray.upcoming().isEmpty())
        assertTrue(Notifier.Tray.pending().isEmpty())
    }

    @Test
    fun cancelAllClearsTheList() {
        Notifier.Tray.arm("a", 60_000, n("a", "2099-01-01T00:00:00Z")) {}
        Notifier.Tray.arm("b", 60_000, n("b", "2099-01-02T00:00:00Z")) {}
        Notifier.Tray.cancelAll()
        assertTrue(Notifier.Tray.upcoming().isEmpty())
    }

    @Test
    fun rearmingSameIdReplacesTheRow() {
        Notifier.Tray.arm("a", 60_000, n("a", "2099-01-01T00:00:00Z", "古い")) {}
        Notifier.Tray.arm("a", 60_000, n("a", "2099-01-02T00:00:00Z", "新しい")) {}
        assertEquals(listOf("新しい"), Notifier.Tray.upcoming().map { it.title })
    }

    @Test
    fun trayMenuListsThemWithCancelRowsInBothLanguages() {
        val up = UpcomingNotifications.of((1..6).map { n("n$it", "2026-11-0${it}T09:00:00Z") })
        for (lang in Lang.entries) Strings.withLang(lang) {
            val items = TrayMenu.items("App", true, emptyList(), true, upcoming = up)
            val sub = items.filterIsInstance<TrayItem.Submenu>().single { it.label == Strings["desktop_tray_upcoming"] }
            assertEquals(5, sub.items.filterIsInstance<TrayItem.CancelNotification>().size)
            assertTrue(sub.items.any { it is TrayItem.Info && it.label == Strings.format("desktop_tray_upcoming_more", 1) })
            assertTrue(sub.items.first() is TrayItem.Info)
        }
        assertEquals("予約した通知", Strings.withLang(Lang.JA) { Strings["desktop_tray_upcoming"] })
        assertEquals("Scheduled notifications", Strings.withLang(Lang.EN) { Strings["desktop_tray_upcoming"] })
    }

    @Test
    fun trayMenuHasNoSubmenuWhenEmpty() {
        val items = TrayMenu.items("App", true, emptyList(), true)
        assertTrue(items.none { it is TrayItem.CancelNotification })
        assertTrue(items.filterIsInstance<TrayItem.Submenu>().none { it.label == Strings["desktop_tray_upcoming"] })
    }
}
