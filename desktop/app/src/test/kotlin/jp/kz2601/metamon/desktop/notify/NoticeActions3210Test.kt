package jp.kz2601.metamon.desktop.notify

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import jp.kz2601.metamon.desktop.Notifier as Scheduler

/** 通知のボタン(開く・あとで)の受け口(#3210)。作り物の口で、押された印が受け口に届くか。 */
class NoticeActions3210Test {
    /** ボタンを出せる作り物。[press] で OS が押されたことにする(Windows は引数・Linux は ActionInvoked の道を通す)。 */
    private class FakeActionNotifier(override val supportsActions: Boolean = true) : Notifier {
        override val available = true
        val shown = mutableListOf<DesktopNotice>()
        val freedesktop = FreedesktopActions()

        override fun show(notice: DesktopNotice): Boolean {
            shown += notice
            freedesktop.shown(shown.size.toLong(), notice)
            return true
        }

        /** Windows: トーストの arguments で起動し直した → 引数を読んで受け口へ。 */
        fun pressOnWindows(notice: DesktopNotice, action: NoticeAction) {
            val arg = NoticeArgs.encode(NoticePress(notice.id!!, action))
            NoticeArgs.parse(listOf(arg))?.let(NoticeActions::deliver)
        }

        /** Linux: ActionInvoked(番号, 鍵)。 */
        fun pressOnLinux(dbusId: Long, key: String) {
            freedesktop.invoked(dbusId, key)?.let(NoticeActions::deliver)
        }
    }

    private val presses = mutableListOf<NoticePress>()

    @Before
    fun setUp() {
        NoticeActions.reset()
    }

    @After
    fun tearDown() {
        NoticeActions.reset()
        Scheduler.Tray.display = TrayNotifier
    }

    @Test
    fun pressReachesHandlerAndNoticeCallback() {
        val fake = FakeActionNotifier()
        Scheduler.Tray.display = fake
        NoticeActions.onPress { presses += it }
        val got = mutableListOf<NoticeAction>()
        assertTrue(Scheduler.Tray.notifyWithActions("n1", "題", "本文") { got += it })
        val notice = fake.shown.single()
        assertEquals(listOf(NoticeAction.OPEN, NoticeAction.LATER), notice.actions)

        fake.pressOnWindows(notice, NoticeAction.LATER)
        fake.pressOnLinux(1, "open")
        assertEquals(listOf(NoticePress("n1", NoticeAction.LATER), NoticePress("n1", NoticeAction.OPEN)), presses)
        assertEquals(listOf(NoticeAction.LATER, NoticeAction.OPEN), got)
    }

    @Test
    fun pressBeforeHandlerIsQueued() {
        // ボタンで起動した直後(受け手を置く前)に届いた分
        NoticeActions.deliver(NoticePress("early", NoticeAction.OPEN))
        NoticeActions.onPress { presses += it }
        assertEquals(listOf(NoticePress("early", NoticeAction.OPEN)), presses)
    }

    @Test
    fun portWithoutActionsShowsPlainNotice() {
        val fake = FakeActionNotifier(supportsActions = false)
        Scheduler.Tray.display = fake
        assertTrue(Scheduler.Tray.notifyWithActions("n2", "題", "本文"))
        assertTrue(fake.shown.single().actions.isEmpty())
        assertNull(fake.shown.single().id)
        assertFalse(TrayNotifier.supportsActions)
    }

    @Test
    fun argsRoundTripAndRejectJunk() {
        val p = NoticePress("timer-a b/&", NoticeAction.OPEN)
        assertEquals(p, NoticeArgs.parse(listOf("shopping", NoticeArgs.encode(p))))
        assertEquals(listOf("shopping"), NoticeArgs.strip(listOf("shopping", NoticeArgs.encode(p))))
        assertEquals(NoticePress("x", NoticeAction.LATER), NoticeArgs.parse(listOf("tailorit-notice://later?id=x")))
        assertNull(NoticeArgs.parse(listOf("tailorit-notice:record?id=x")))
        assertNull(NoticeArgs.parse(listOf("tailorit-notice:open")))
        assertNull(NoticeArgs.parse(listOf("tailorit-notice:open?id=")))
        assertNull(NoticeArgs.parse(listOf("shopping")))
    }

    @Test
    fun toastXmlHasEscapedActions() {
        val n = DesktopNotice("A & <B>", "\"本文\"", id = "n1", actions = listOf(NoticeAction.OPEN, NoticeAction.LATER))
        val xml = WindowsToastXml.build(n) { if (it == NoticeAction.OPEN) "開く" else "あとで" }
        assertTrue(xml, xml.contains("<text>A &amp; &lt;B&gt;</text>"))
        assertTrue(xml, xml.contains("<text>&quot;本文&quot;</text>"))
        assertTrue(xml, xml.contains("<action content=\"開く\" arguments=\"tailorit-notice:open?id=n1\" activationType=\"protocol\"/>"))
        assertTrue(xml, xml.contains("<action content=\"あとで\" arguments=\"tailorit-notice:later?id=n1\" activationType=\"protocol\"/>"))
        assertFalse("ボタンが無ければ actions を出さない", WindowsToastXml.build(DesktopNotice("t", "b")) { it.wire }.contains("<actions>"))
    }

    @Test
    fun freedesktopActionsAndInvoked() {
        val fd = FreedesktopActions()
        val n = DesktopNotice("t", "b", id = "n9", actions = listOf(NoticeAction.OPEN, NoticeAction.LATER))
        assertEquals(listOf("open", "O", "later", "L"), fd.actions(n) { if (it == NoticeAction.OPEN) "O" else "L" })
        fd.shown(42, n)
        assertEquals(NoticePress("n9", NoticeAction.LATER), fd.invoked(42, "later"))
        assertEquals("本体のクリックは開く", NoticePress("n9", NoticeAction.OPEN), fd.invoked(42, "default"))
        assertNull("ほかのアプリの通知", fd.invoked(7, "open"))
        assertNull(fd.invoked(42, "record"))
        fd.closed(42)
        assertNull(fd.invoked(42, "open"))
    }

    @Test
    fun labelsComeFromStrings() {
        assertEquals("開く", NoticeAction.OPEN.label)
        assertEquals("あとで", NoticeAction.LATER.label)
    }
}
