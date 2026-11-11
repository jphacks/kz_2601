package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.ActionType
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** トレイ常駐(#1761): メニューの形・閉じてもトレイに残す設定・すぐ記録の行き先・タイマーの表示。画面は出さない。 */
class TrayTest {
    @After
    fun tearDown() = DesktopTimers.clear()

    private fun labels(items: List<TrayItem>) = items.map {
        when (it) {
            is TrayItem.Command -> "${it.label}${if (it.enabled) "" else " [off]"}"
            is TrayItem.Toggle -> "${it.label} [${if (it.checked) "x" else " "}]"
            is TrayItem.Info -> "(${it.label})"
            TrayItem.Separator -> "---"
        }
    }

    @Test
    fun menuWithoutTimers() = Strings.withLang(Lang.JA) {
        val items = TrayMenu.items("家計簿", canQuickRecord = true, timers = emptyList(), closeToTray = true)
        assertEquals(
            listOf("「家計簿」を開く", "すぐ記録", "音声で話す(デスクトップ版ではまだ使えません) [off]", "---", "ウィンドウを閉じてもトレイに残す [x]", "終了"),
            labels(items),
        )
        assertEquals(
            listOf(TrayCommand.OPEN, TrayCommand.QUICK_RECORD, TrayCommand.VOICE, null, TrayCommand.CLOSE_TO_TRAY, TrayCommand.QUIT),
            items.map { (it as? TrayItem.Command)?.command ?: (it as? TrayItem.Toggle)?.command },
        )
    }

    @Test
    fun menuShowsTimersAndDisablesQuickRecordWithoutForm() = Strings.withLang(Lang.EN) {
        val timers = listOf(
            TimerStatus("a/t1", "Ramen", 125, countdown = true, running = true),
            TimerStatus("a/t2", "Study", 3725, countdown = false, running = false),
        )
        val items = TrayMenu.items("Ramen timer", canQuickRecord = false, timers = timers, closeToTray = false)
        assertEquals(
            listOf(
                "Open Ramen timer", "Quick record [off]", "Talk by voice (not available on desktop yet) [off]",
                "---", "(Ramen 02:05 left)", "(Study 1:02:05 (paused))",
                "---", "Keep in tray when window is closed [ ]", "Quit",
            ),
            labels(items),
        )
        assertEquals("Tailor It — Ramen timer\nRamen 02:05 left\nStudy 1:02:05 (paused)", TrayMenu.tooltip("Ramen timer", timers))
        assertEquals("Tailor It — Ramen timer", TrayMenu.tooltip("Ramen timer", emptyList()))
    }

    @Test
    fun closeHidesOnlyWhenEnabledAndTrayExists() {
        assertEquals(CloseDecision.HIDE_TO_TRAY, CloseToTray.decide(enabled = true, traySupported = true))
        assertEquals(CloseDecision.EXIT, CloseToTray.decide(enabled = false, traySupported = true))
        // トレイが無いのに隠すと戻れない
        assertEquals(CloseDecision.EXIT, CloseToTray.decide(enabled = true, traySupported = false))
        assertEquals(CloseDecision.EXIT, CloseToTray.decide(enabled = false, traySupported = false))
    }

    @Test
    fun controllerHidesKeepsSettingAndHintsOnce() = Strings.withLang(Lang.JA) {
        val setting = MemoryBoolSetting(true)
        val hint = MemoryBoolSetting(false)
        val c = TrayController(Examples.load("kakeibo"), traySupported = true, closeSetting = setting, hintSetting = hint)
        val shown = mutableListOf<String>()
        assertTrue(c.windowVisible)
        assertFalse(c.onCloseRequest { _, body -> shown += body })
        assertFalse(c.windowVisible)
        assertEquals(listOf("トレイに残っています。終わるにはトレイのメニューの「終了」を選びます"), shown)

        c.open()
        assertTrue(c.windowVisible)
        assertEquals(1, c.focusRequests)
        assertFalse(c.onCloseRequest { _, body -> shown += body })
        // 2 回目は知らせない(設定に残る)
        assertEquals(1, shown.size)
        assertTrue(hint.get())

        c.updateCloseToTray(false)
        assertFalse(setting.get())
        assertTrue(c.onCloseRequest { _, _ -> error("終わる時は知らせない") })
        // 設定は次の起動にも効く
        assertFalse(TrayController(Examples.load("kakeibo"), true, setting, hint).closeToTray)
    }

    @Test
    fun controllerWithoutTrayAlwaysExits() {
        val c = TrayController(Examples.load("kakeibo"), traySupported = false, closeSetting = MemoryBoolSetting(true), hintSetting = MemoryBoolSetting(false))
        assertTrue(c.onCloseRequest { _, _ -> error("出さない") })
        assertTrue(c.windowVisible)
    }

    @Test
    fun quickRecordOpensWindowAndCountsRequests() {
        val c = TrayController(Examples.load("kakeibo"), true, MemoryBoolSetting(true), MemoryBoolSetting(true))
        c.onCloseRequest { _, _ -> }
        assertFalse(c.windowVisible)
        c.quickRecord()
        assertTrue(c.windowVisible)
        assertEquals(1, c.quickRecordRequests)
        assertEquals("expense_form", c.quickRecordAction?.screen)
    }

    @Test
    fun quickRecordTargetUsesTheDslNavigateToTheFirstRecordableForm() {
        val kakeibo = QuickRecordTarget.find(Examples.load("kakeibo"))
        assertNotNull(kakeibo)
        assertEquals(ActionType.NAVIGATE, kakeibo!!.type)
        assertEquals("expense_form", kakeibo.screen)
        assertNull(kakeibo.prefillRef)
        assertEquals("item_form", QuickRecordTarget.find(Examples.load("shopping"))?.screen)
        // フォーム画面の無いアプリでは押せない
        assertNull(QuickRecordTarget.find(Examples.load("initial")))
    }

    @Test
    fun quickRecordNavigatesTheSession() {
        val dsl = Examples.load("kakeibo")
        val s = DesktopSession(dsl)
        val a = QuickRecordTarget.find(dsl)!!
        s.dispatch(a, ActionContext(s.top, null))
        assertEquals("expense_form", s.top.screen)
    }

    @Test
    fun activeTimersSkipResetAndFinished() {
        val now = 1_000_000L
        DesktopTimers.set("s/run", TimerRun(base = 10, startedAtMillis = now - 5_000), "走る", 60)
        DesktopTimers.set("s/paused", TimerRun(base = 30), "止めた", null)
        DesktopTimers.set("s/reset", TimerRun(), "リセット", 60)
        DesktopTimers.set("s/done", TimerRun(base = 60), "終わった", 60)
        assertEquals(
            listOf(
                TimerStatus("s/paused", "止めた", 30, countdown = false, running = false),
                TimerStatus("s/run", "走る", 45, countdown = true, running = true),
            ),
            DesktopTimers.active(now),
        )
    }

    @Test
    fun trayStringsExistInBothLanguages() {
        val keys = listOf(
            "desktop_timer_done", "desktop_tray_close_to_tray", "desktop_tray_hidden_hint", "desktop_tray_open", "desktop_tray_quick_record",
            "desktop_tray_quit", "desktop_tray_timer_elapsed", "desktop_tray_timer_left", "desktop_tray_timer_paused", "desktop_tray_voice",
        )
        keys.forEach { k -> Lang.entries.forEach { l -> assertTrue("$k ($l)", Strings.table.has(l, k)) } }
        assertEquals("Time's up", Strings.withLang(Lang.EN) { Strings["desktop_timer_done"] })
    }
}
