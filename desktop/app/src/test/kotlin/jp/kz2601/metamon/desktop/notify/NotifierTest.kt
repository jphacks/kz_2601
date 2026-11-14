package jp.kz2601.metamon.desktop.notify

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Notification
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import jp.kz2601.metamon.desktop.Notifier as Scheduler

/** 予約(Notifier.Tray)が通知を出す口([Notifier])を通って鳴るか。作り物の口で確かめる。 */
class NotifierTest {
    private class Fake(override val available: Boolean = true) : Notifier {
        val shown = mutableListOf<DesktopNotice>()
        val latch = CountDownLatch(1)

        override fun show(notice: DesktopNotice): Boolean {
            synchronized(shown) { shown += notice }
            latch.countDown()
            return true
        }
    }

    @After
    fun tearDown() {
        Scheduler.Tray.cancelAll()
        Scheduler.Tray.display = TrayNotifier
    }

    @Test
    fun defaultIsTray() {
        assertTrue(Scheduler.Tray.display === TrayNotifier)
    }

    @Test
    fun scheduledNotificationGoesThroughThePort() {
        val fake = Fake()
        Scheduler.Tray.display = fake
        Scheduler.Tray.schedule(Notification(id = "port-test", title = "題", body = "本文"))
        assertTrue(fake.latch.await(5, TimeUnit.SECONDS))
        assertEquals(listOf("題" to "本文"), synchronized(fake.shown) { fake.shown.map { it.title to it.body } })
    }

    @Test
    fun notifyNowGoesThroughThePort() {
        val fake = Fake()
        Scheduler.Tray.display = fake
        Scheduler.Tray.notify("今", "すぐ")
        assertEquals(listOf("今" to "すぐ"), fake.shown.map { it.title to it.body })
    }

    @Test
    fun unavailablePortRefusesToSchedule() {
        val fake = Fake(available = false)
        Scheduler.Tray.display = fake
        assertEquals(Strings["desktop_tray_unavailable"], Scheduler.Tray.schedule(Notification(id = "off", title = "t", body = "b")))
        assertEquals(0, Scheduler.Tray.pendingCount())
        assertTrue(fake.shown.isEmpty())
    }
}
