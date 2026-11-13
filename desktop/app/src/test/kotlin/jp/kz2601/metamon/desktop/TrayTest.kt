package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.entityModels
import java.time.Instant
import java.time.LocalDate
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

    private fun labels(items: List<TrayItem>): List<String> = items.map {
        when (it) {
            is TrayItem.Command -> "${it.label}${if (it.enabled) "" else " [off]"}"
            is TrayItem.Toggle -> "${it.label} [${if (it.checked) "x" else " "}]"
            is TrayItem.Info -> "(${it.label})"
            TrayItem.Separator -> "---"
            is TrayItem.Submenu -> "${it.label} > ${labels(it.items).joinToString(" | ")}"
            is TrayItem.OpenApp -> "${it.label} <${it.appId}>"
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
    fun voiceRowStartsRecordingInTheFrontWindowWhenVoiceIsEnabled() = Strings.withLang(Lang.JA) {
        val items = TrayMenu.items("家計簿", canQuickRecord = true, timers = emptyList(), closeToTray = true, voiceEnabled = true)
        assertEquals("音声で話す(録音を始める / 止める)", labels(items)[2])
        assertEquals(TrayCommand.VOICE, (items[2] as TrayItem.Command).command)

        val c = TrayController(Examples.load("kakeibo"), true, MemoryBoolSetting(true), MemoryBoolSetting(true), voiceEnabled = true)
        assertTrue((c.items(emptyList(), emptyList())[2] as TrayItem.Command).enabled)
        c.onCloseRequest { _, _ -> }
        c.voice()
        assertTrue("結果を出すウィンドウを前に出す", c.windowVisible)
        assertEquals(1, c.voiceRequests)
        // 後ろのウィンドウは受け取らず、前のウィンドウが 1 回だけ受け取る
        assertFalse(c.takeVoice { false })
        assertTrue(c.takeVoice { true })
        assertFalse(c.takeVoice { true })
        // もう一度押すと止める側(同じ openVoice をもう 1 回)
        c.voice()
        assertEquals(2, c.voiceRequests)
        assertTrue(c.takeVoice { true })
    }

    @Test
    fun voiceRowStaysDisabledWhenVoiceIsDisabled() = Strings.withLang(Lang.JA) {
        val c = TrayController(Examples.load("kakeibo"), true, MemoryBoolSetting(true), MemoryBoolSetting(true), voiceEnabled = false)
        assertEquals("音声で話す(デスクトップ版ではまだ使えません) [off]", labels(c.items(emptyList(), emptyList()))[2])
        c.onCloseRequest { _, _ -> }
        c.voice()
        assertFalse("押せない行からは何もしない", c.windowVisible)
        assertEquals(0, c.voiceRequests)
        assertFalse(c.takeVoice { true })
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
    fun statRowsComeFirstForAppsWithStatsAndAreAbsentOtherwise() = Strings.withLang(Lang.JA) {
        val today = LocalDate.now().toString()
        val store = MockStore()
            .save("expense", null, mapOf("amount" to "1200", "category" to "食費", "date" to today))
            .save("expense", null, mapOf("amount" to "300", "category" to "交通", "date" to today))
        val kakeibo = Examples.load("kakeibo")
        val c = TrayController(kakeibo, true, MemoryBoolSetting(true), MemoryBoolSetting(true))
        // 記録を渡す前・読み込み中は「—」
        assertEquals(listOf(TrayStat("今月の支出", "—"), TrayStat("今月の件数", "—")), c.stats())
        val records = SharedRecords(loading = true)
        c.switchApp(kakeibo, records)
        assertEquals(listOf("—", "—"), c.stats().map { it.value })
        records.loaded(store)
        // 値はアプリ内の stat と同じ式
        val data = LeafData.of(kakeibo, store)
        val inApp = TrayStats.components(kakeibo).map { StatValue.resolve(it, data).first }
        assertEquals(inApp, c.stats().map { it.value })
        assertEquals("2 件", inApp[1])
        assertEquals(
            listOf("(今月の支出 ${inApp[0]})", "(今月の件数 2 件)", "---", "「家計簿」を開く", "すぐ記録"),
            labels(c.items(emptyList())).take(5),
        )
        // 保存すると次に開いた時に新しい値
        records.update(kakeibo.entityModels(), store.save("expense", null, mapOf("amount" to "1", "date" to today)))
        assertEquals("3 件", c.stats()[1].value)
        // 集計はメニューにだけ出し、ツールチップには出さない
        assertEquals("Tailor It — 家計簿", TrayMenu.tooltip("家計簿", emptyList()))

        // stat の無いアプリは行を出さない(メニューは集計の無い形と同じ)
        val plain = Examples.load("chat")
        c.switchApp(plain, SharedRecords())
        assertEquals(emptyList<TrayStat>(), c.stats())
        assertEquals(labels(TrayMenu.items(plain.app.name, c.quickRecordAction != null, emptyList(), true)), labels(c.items(emptyList())))
    }

    @Test
    fun statsDayFollowsDslClockInFieldZone() {
        try {
            // 2026-11-30T15:30Z は日本時間で 12/1
            DSLClock.fixed = Instant.parse("2026-11-30T15:30:00Z")
            assertEquals(LocalDate.ofInstant(DSLClock.fixed, FieldCoding.zone), TrayStats.today())
            val before = TrayStats.today()
            DSLClock.fixed = Instant.parse("2026-12-01T15:30:00Z")
            assertTrue(TrayStats.today().isAfter(before))
        } finally {
            DSLClock.fixed = null
        }
    }

    @Test
    fun statOrderFollowsInitialScreenThenTabsThenRestUpToThree() {
        fun stat(label: String) = """{"type":"stat","label":"$label","entity":"e","aggregate":"count"}"""
        val dsl = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"順番","tabs":[{"screen":"b","title":"B"},{"screen":"a","title":"A"}]},"initialScreen":"a",
             "entities":[{"id":"e","name":"E","fields":[{"id":"n","name":"N","type":"string"}]}],
             "screens":[
               {"id":"c","title":"C","components":[${stat("c1")}]},
               {"id":"b","title":"B","components":[${stat("b1")},{"type":"card","children":[${stat("b2")}]}]},
               {"id":"a","title":"A","components":[${stat("a1")}]}
             ]}
            """.trimIndent(),
        )
        assertEquals(listOf("a1", "b1", "b2", "c1"), TrayStats.components(dsl).map { it.label })
        assertEquals(listOf("a1", "b1", "b2"), TrayStats.make(dsl, MockStore()).map { it.label })
        assertEquals(listOf("0 件", "0 件", "0 件"), TrayStats.make(dsl, MockStore()).map { it.value })
    }

    @Test
    fun trayStringsExistInBothLanguages() {
        val keys = listOf(
            "desktop_timer_done", "desktop_tray_close_to_tray", "desktop_tray_hidden_hint", "desktop_tray_open", "desktop_tray_quick_record",
            "desktop_tray_quit", "desktop_tray_timer_elapsed", "desktop_tray_timer_left", "desktop_tray_timer_paused", "desktop_tray_voice",
            "desktop_tray_voice_toggle",
        )
        keys.forEach { k -> Lang.entries.forEach { l -> assertTrue("$k ($l)", Strings.table.hasString(l, k)) } }
        assertEquals("Time's up", Strings.withLang(Lang.EN) { Strings["desktop_timer_done"] })
    }

    // MARK: - timerLabel(#2627)

    private fun timer(seconds: Int, countdown: Boolean, running: Boolean = true) = TimerStatus("k", "ラーメン", seconds, countdown, running)

    @Test
    fun timerLabelLeftElapsedHoursAndZero() {
        Strings.withLang(Lang.JA) {
            assertEquals("ラーメン 残り 02:05", TrayMenu.timerLabel(timer(125, countdown = true)))
            assertEquals("ラーメン 02:05", TrayMenu.timerLabel(timer(125, countdown = false)))
            assertEquals("1 時間を超えたら時も出す", "ラーメン 残り 1:00:00", TrayMenu.timerLabel(timer(3600, countdown = true)))
            assertEquals("ラーメン 59:59", TrayMenu.timerLabel(timer(3599, countdown = false)))
            assertEquals("ラーメン 26:01:01", TrayMenu.timerLabel(timer(26 * 3600 + 61, countdown = false)))
            assertEquals("ラーメン 残り 00:00", TrayMenu.timerLabel(timer(0, countdown = true)))
            assertEquals("ラーメン 00:00", TrayMenu.timerLabel(timer(0, countdown = false)))
            assertEquals("ラーメン 残り 00:30(一時停止)", TrayMenu.timerLabel(timer(30, countdown = true, running = false)))
        }
        Strings.withLang(Lang.EN) {
            assertEquals("ラーメン 02:05 left", TrayMenu.timerLabel(timer(125, countdown = true)))
            assertEquals("ラーメン 00:00 (paused)", TrayMenu.timerLabel(timer(0, countdown = false, running = false)))
        }
    }
}
